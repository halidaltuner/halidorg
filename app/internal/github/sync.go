package github

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/settings"
	"halid.org/engine/internal/store"
)

// TypeSync is the job the cron Worker enqueues every 6 hours; the admin
// "run now" button enqueues it too.
const TypeSync = "github.sync"

// pinsSeededKey records that the first sync took the profile's pins; after
// that pins are the admin's.
const pinsSeededKey = "github_pins_seeded"

// Fetcher reads a snapshot (the Client; a fake in tests).
type Fetcher interface {
	Fetch(ctx context.Context) (Snapshot, error)
}

// TokenHeader carries the token on job deliveries (worker/src/jobs.ts).
const TokenHeader = "X-GitHub-Token"

// Sync runs the github.sync job.
type Sync struct {
	db       store.DB
	envToken string
	client   func(token string) Fetcher
	jobs     *jobs.Jobs
}

// NewSync wires the job. The token comes with each delivery from the Worker,
// so a secret set after the container started still works; envToken
// (GITHUB_TOKEN) is the fallback. Without either the job ends at once.
func NewSync(db store.DB, envToken string, j *jobs.Jobs) *Sync {
	return &Sync{db: db, envToken: envToken, jobs: j, client: func(t string) Fetcher { return NewClient(t) }}
}

// WithClient replaces how a token becomes a Fetcher (tests).
func (s *Sync) WithClient(f func(token string) Fetcher) *Sync { s.client = f; return s }

// Register adds the job handler.
func (s *Sync) Register(j *jobs.Jobs) { j.Handle(TypeSync, s.run) }

func (s *Sync) run(ctx context.Context, _ json.RawMessage) error {
	token := jobs.DeliveryHeader(ctx, TokenHeader)
	if token == "" {
		token = s.envToken
	}
	if token == "" {
		return jobs.Permanent{Err: ErrNoToken}
	}
	snap, err := s.client(token).Fetch(ctx)
	if errors.Is(err, ErrNoToken) {
		return jobs.Permanent{Err: err}
	}
	if err != nil && len(snap.Repos) == 0 {
		return err // rate limits and network errors retry with backoff
	}
	if werr := Write(ctx, s.db, snap, content.Clock()); werr != nil {
		return werr
	}
	paths, perr := s.paths(ctx)
	if perr == nil && len(paths) > 0 {
		for len(paths) > 0 {
			n := min(len(paths), 50)
			if e := jobs.EnqueuePages(ctx, s.jobs, paths[:n]...); e != nil {
				slog.Error("github: enqueue pages", "err", e)
			}
			paths = paths[n:]
		}
	}
	// What was written stays; a partial read still reports, so job_runs shows it.
	switch {
	case err != nil:
		return err
	case len(snap.Partial) > 0:
		return fmt.Errorf("github: partial sync: %s", strings.Join(snap.Partial, "; "))
	}
	return perr
}

// paths are the pages that show GitHub data: the projects lists and the
// pages of projects linked to a repo.
func (s *Sync) paths(ctx context.Context) ([]string, error) {
	var out []string
	for _, l := range i18n.Supported {
		out = append(out, "/"+l+"/projects")
	}
	res, err := s.db.Query(ctx, `SELECT e.lang, e.slug FROM entries e JOIN projects p ON p.entry_id = e.id
WHERE p.github_repo IS NOT NULL AND e.status = 'published' AND e.visibility = 'public'`)
	if err != nil {
		return out, err
	}
	rows, err := store.Scan[struct {
		Lang string `json:"lang"`
		Slug string `json:"slug"`
	}](res)
	for _, r := range rows {
		out = append(out, "/"+r.Lang+"/projects/"+r.Slug)
	}
	return out, err
}

func unix(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.Unix()
}

func b(v bool) int {
	if v {
		return 1
	}
	return 0
}

// Write stores a snapshot. Repos are upserted (pins kept); a complete read
// marks the repos it did not see as gone, never deleting them. Languages are
// replaced per repo seen, the calendar when it was read, and activity rows
// are added once each. The first sync takes the profile's pins.
func Write(ctx context.Context, db store.DB, snap Snapshot, now time.Time) error {
	var stmts []store.Stmt
	at := now.Unix()
	for _, r := range snap.Repos {
		topics, _ := json.Marshal(r.Topics)
		if r.Topics == nil {
			topics = []byte("[]")
		}
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO gh_repos (id, full_name, name, description, url, stars, forks, language, topics_json,
  pushed_at, created_at, archived, fork, gone, synced_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)
ON CONFLICT (id) DO UPDATE SET full_name = excluded.full_name, name = excluded.name, description = excluded.description,
  url = excluded.url, stars = excluded.stars, forks = excluded.forks, language = excluded.language, topics_json = excluded.topics_json,
  pushed_at = excluded.pushed_at, created_at = excluded.created_at, archived = excluded.archived, fork = excluded.fork, gone = 0,
  synced_at = excluded.synced_at`,
			Args: []any{r.ID, r.FullName, r.Name, r.Description, r.URL, r.Stars, r.Forks, r.Language, string(topics),
				unix(r.PushedAt), unix(r.CreatedAt), b(r.Archived), b(r.Fork), at}})
		stmts = append(stmts, store.Stmt{SQL: `DELETE FROM gh_languages WHERE repo_id = ?`, Args: []any{r.ID}})
		for lang, bytes := range r.Languages {
			stmts = append(stmts, store.Stmt{SQL: `INSERT INTO gh_languages (repo_id, language, bytes) VALUES (?, ?, ?)`, Args: []any{r.ID, lang, bytes}})
		}
		if !r.Fork && r.CreatedAt != nil {
			stmts = append(stmts, store.Stmt{SQL: `INSERT OR IGNORE INTO gh_activity (id, type, repo, title, note, url, occurred_at) VALUES (?, 'repo', ?, ?, ?, ?, ?)`,
				Args: []any{"repo:" + r.ID, r.FullName, r.Name, r.Description, r.URL, r.CreatedAt.Unix()}})
		}
		for _, rel := range r.Releases {
			title := r.Name + " " + rel.Tag
			stmts = append(stmts, store.Stmt{SQL: `INSERT INTO gh_activity (id, type, repo, title, note, url, occurred_at) VALUES (?, 'release', ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET title = excluded.title, note = excluded.note, url = excluded.url`,
				Args: []any{"release:" + rel.ID, r.FullName, title, rel.Note, rel.URL, rel.PublishedAt.Unix()}})
		}
	}
	if snap.Complete {
		stmts = append(stmts, store.Stmt{SQL: `UPDATE gh_repos SET gone = 1 WHERE synced_at < ?`, Args: []any{at}})
	}
	if snap.Weeks != nil {
		stmts = append(stmts, store.Stmt{SQL: `DELETE FROM gh_weekly`})
		for _, w := range snap.Weeks {
			stmts = append(stmts, store.Stmt{SQL: `INSERT OR REPLACE INTO gh_weekly (week_start, contributions) VALUES (?, ?)`,
				Args: []any{w.Start.Unix(), w.Contributions}})
		}
	}
	// D1 batches are one transaction; keep each one modest.
	for len(stmts) > 0 {
		n := min(len(stmts), 80)
		if _, err := db.Batch(ctx, stmts[:n]...); err != nil {
			return fmt.Errorf("github: write: %w", err)
		}
		stmts = stmts[n:]
	}
	return seedPins(ctx, db, snap.Pinned)
}

func seedPins(ctx context.Context, db store.DB, pinned []string) error {
	var seeded bool
	if err := settings.Get(ctx, db, pinsSeededKey, &seeded); err != nil || seeded {
		return err
	}
	stmts := make([]store.Stmt, 0, len(pinned))
	for i, id := range pinned {
		stmts = append(stmts, store.Stmt{SQL: `UPDATE gh_repos SET pinned = 1, pin_sort = ? WHERE id = ?`, Args: []any{i, id}})
	}
	if len(stmts) > 0 {
		if _, err := db.Batch(ctx, stmts...); err != nil {
			return err
		}
	}
	return settings.Set(ctx, db, pinsSeededKey, true)
}

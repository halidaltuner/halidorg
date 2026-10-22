// Package webmention receives webmentions (Phase 8.1, W3C Webmention): the
// Worker accepts POST /webmention and queues a receive job, the job verifies
// the source and stores what it says, the dashboard moderates replies and
// mentions, and detail pages show the approved ones.
package webmention

import (
	"context"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Types and statuses (migrations/0019_webmentions.sql).
const (
	Reply   = "reply"
	Like    = "like"
	Repost  = "repost"
	Mention = "mention"

	Pending  = "pending"
	Approved = "approved"
	Rejected = "rejected"
)

// Row is a stored webmention.
type Row struct {
	ID          string `json:"id"`
	EntryID     string `json:"target_entry_id"`
	TargetURL   string `json:"target_url"`
	SourceURL   string `json:"source_url"`
	Author      string `json:"author"`
	AuthorURL   string `json:"author_url"`
	Avatar      string `json:"author_avatar"`
	Content     string `json:"content"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	PublishedAt *int64 `json:"published_at"`
	ReceivedAt  int64  `json:"received_at"`
}

// Day is when the mention was written, else when it arrived.
func (r Row) Day() int64 {
	if r.PublishedAt != nil {
		return *r.PublishedAt
	}
	return r.ReceivedAt
}

// target is the entry a webmention points at.
type target struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
	Lang string `json:"lang"`
	Slug string `json:"slug"`
}

// paths are the pages showing the entry's mentions: its own and the same
// slug in the other languages (the untranslated fallback renders it there).
func (t target) paths() []string {
	var out []string
	for _, l := range i18n.Supported {
		if p := jobs.ItemPath(jobs.Ref{Kind: t.Kind, Lang: l, Slug: t.Slug}); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// Repo reads and moderates webmentions.
type Repo struct{ db store.DB }

// NewRepo returns a repo on the main database.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const cols = `w.id, w.target_entry_id, w.target_url, w.source_url, w.author, w.author_url, w.author_avatar,
  w.content, w.type, w.status, w.published_at, w.received_at`

// Approved are the entry's mentions to show, oldest first.
func (r *Repo) Approved(ctx context.Context, entryID string) ([]Row, error) {
	res, err := r.db.Query(ctx, `SELECT `+cols+` FROM webmentions w WHERE w.target_entry_id = ? AND w.status = 'approved'
ORDER BY coalesce(w.published_at, w.received_at), w.id`, entryID)
	if err != nil {
		return nil, err
	}
	return store.Scan[Row](res)
}

// Queued is a pending mention with the title of the entry it points at.
type Queued struct {
	Row
	Title string `json:"title"`
}

// Pending are the mentions waiting for moderation, newest first, and how
// many there are in all.
func (r *Repo) Pending(ctx context.Context, limit int) ([]Queued, int, error) {
	res, err := r.db.Query(ctx, `SELECT `+cols+`, e.title FROM webmentions w JOIN entries e ON e.id = w.target_entry_id
WHERE w.status = 'pending' ORDER BY w.received_at DESC, w.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, 0, err
	}
	rows, err := store.Scan[Queued](res)
	if err != nil {
		return nil, 0, err
	}
	res, err = r.db.Query(ctx, `SELECT COUNT(*) AS n FROM webmentions WHERE status = 'pending'`)
	if err != nil {
		return nil, 0, err
	}
	n, err := store.Scan[struct {
		N int `json:"n"`
	}](res)
	if err != nil || len(n) == 0 {
		return rows, len(rows), err
	}
	return rows, n[0].N, nil
}

// Moderate sets a mention's status and returns the pages to re-render (none
// when what shows did not change). A missing id is content.ErrNotFound.
func (r *Repo) Moderate(ctx context.Context, id, status string) ([]string, error) {
	res, err := r.db.Query(ctx, `SELECT w.status, e.id, e.kind, e.lang, e.slug FROM webmentions w
JOIN entries e ON e.id = w.target_entry_id WHERE w.id = ?`, id)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		Status string `json:"status"`
		target
	}](res)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errNotFound(id)
	}
	if _, err := r.db.Query(ctx, `UPDATE webmentions SET status = ? WHERE id = ?`, status, id); err != nil {
		return nil, err
	}
	if rows[0].Status == status || (rows[0].Status != Approved && status != Approved) {
		return nil, nil
	}
	return rows[0].target.paths(), nil
}

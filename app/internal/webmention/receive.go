package webmention

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/fetch"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// TypeReceive is the job the Worker queues for each POST /webmention
// (worker/src/webmention.ts).
const TypeReceive = "webmention.receive"

// Received is its payload.
type Received struct {
	Source string `json:"source"`
	Target string `json:"target"`
}

// Fetcher gets a URL with the SSRF guard (fetch.Client).
type Fetcher interface {
	Get(ctx context.Context, raw string, max int64) (*fetch.Response, error)
}

// MaxSource is how much of a source page is read.
const MaxSource = 1 << 20

// MaxAvatar bounds an author photo.
const MaxAvatar = 256 << 10

// AvatarPrefix is where avatars are served from R2 by the Worker
// (worker/src/r2public.ts): one object per photo URL.
const AvatarPrefix = "/avatars/"

var avatarMIME = map[string]string{"png": "image/png", "jpg": "image/jpeg", "gif": "image/gif", "webp": "image/webp", "ico": "image/x-icon"}

// Receiver verifies and stores webmentions.
type Receiver struct {
	db     store.DB
	blob   store.Blob
	get    Fetcher
	jobs   *jobs.Jobs
	origin *url.URL // PUBLIC_ORIGIN: targets must be on it
}

// NewReceiver wires the receive job; register it with Register.
func NewReceiver(db store.DB, blob store.Blob, get Fetcher) *Receiver {
	o, _ := url.Parse(strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/"))
	return &Receiver{db: db, blob: blob, get: get, origin: o}
}

// WithOrigin sets the site origin (tests).
func (r *Receiver) WithOrigin(origin string) *Receiver {
	r.origin, _ = url.Parse(origin)
	return r
}

// Register adds the receive handler to j.
func (r *Receiver) Register(j *jobs.Jobs) {
	r.jobs = j
	j.Handle(TypeReceive, r.receive)
}

func (r *Receiver) receive(ctx context.Context, raw json.RawMessage) error {
	var m Received
	if err := json.Unmarshal(raw, &m); err != nil {
		return jobs.Permanent{Err: fmt.Errorf("webmention: bad payload: %w", err)}
	}
	paths, err := r.Receive(ctx, m)
	if err != nil {
		return err
	}
	if len(paths) > 0 && r.jobs != nil {
		return jobs.EnqueuePages(ctx, r.jobs, paths...)
	}
	return nil
}

// Receive verifies one webmention and stores, updates or deletes it. It
// returns the pages whose mentions changed. Errors that a retry cannot fix
// are jobs.Permanent; a source that is down returns a plain error, so the
// queue tries again.
func (r *Receiver) Receive(ctx context.Context, m Received) ([]string, error) {
	t, err := r.target(ctx, m.Target)
	if err != nil {
		return nil, err
	}
	src, err := url.Parse(m.Source)
	if err != nil || fetch.Check(src) != nil {
		return nil, jobs.Permanent{Err: fmt.Errorf("webmention: source %q not allowed", m.Source)}
	}
	if r.origin != nil && strings.EqualFold(src.Hostname(), r.origin.Hostname()) {
		return nil, jobs.Permanent{Err: fmt.Errorf("webmention: source %q is this site", m.Source)}
	}
	res, err := r.get.Get(ctx, m.Source, MaxSource)
	switch {
	case errors.Is(err, fetch.ErrBlocked), errors.Is(err, fetch.ErrTooLarge):
		return nil, jobs.Permanent{Err: fmt.Errorf("webmention: %s: %w", m.Source, err)}
	case err != nil:
		return nil, fmt.Errorf("webmention: %s: %w", m.Source, err)
	}
	switch {
	case res.Status == http.StatusGone || res.Status == http.StatusNotFound:
		return r.remove(ctx, m.Source, t) // the source was deleted
	case res.Status >= 500 || res.Status == http.StatusTooManyRequests:
		return nil, fmt.Errorf("webmention: %s: status %d", m.Source, res.Status)
	case res.Status != http.StatusOK:
		return nil, jobs.Permanent{Err: fmt.Errorf("webmention: %s: status %d", m.Source, res.Status)}
	}
	p := Parse(res.Body, res.ContentType, res.URL, m.Target)
	if !p.Linked {
		return r.remove(ctx, m.Source, t) // never linked, or the link was taken out
	}
	return r.store(ctx, m, t, p)
}

// target finds the published, public entry a target URL names: a page
// with an item page (/{lang}/{section}/{slug}) on this site. The same slug
// in another language is the untranslated fallback of that entry.
func (r *Receiver) target(ctx context.Context, raw string) (target, error) {
	bad := func(why string) (target, error) {
		return target{}, jobs.Permanent{Err: fmt.Errorf("webmention: target %q: %s", raw, why)}
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return bad("not a URL")
	}
	if r.origin == nil || !strings.EqualFold(u.Host, r.origin.Host) {
		return bad("not on this site")
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) != 3 || !i18n.IsSupported(parts[0]) || !content.ValidSlug(parts[2]) {
		return bad("not an entry page")
	}
	kind := jobs.KindOfSegment(parts[1])
	if kind == "" {
		return bad("not an entry page")
	}
	res, err := r.db.Query(ctx, `SELECT id, kind, lang, slug FROM entries
WHERE kind = ? AND slug = ? AND status = 'published' AND visibility = 'public'
ORDER BY lang = ? DESC LIMIT 1`, kind, parts[2], parts[0])
	if err != nil {
		return target{}, err
	}
	rows, err := store.Scan[target](res)
	if err != nil {
		return target{}, err
	}
	if len(rows) == 0 {
		return bad("no such entry")
	}
	return rows[0], nil
}

type existing struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Type    string `json:"type"`
	Author  string `json:"author"`
	Content string `json:"content"`
	Avatar  string `json:"author_avatar"`
}

func (r *Receiver) find(ctx context.Context, source, entryID string) (*existing, error) {
	res, err := r.db.Query(ctx, `SELECT id, status, type, author, content, author_avatar FROM webmentions
WHERE source_url = ? AND target_entry_id = ?`, source, entryID)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[existing](res)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

// remove deletes a mention whose source is gone or no longer links here.
func (r *Receiver) remove(ctx context.Context, source string, t target) ([]string, error) {
	old, err := r.find(ctx, source, t.ID)
	if err != nil || old == nil {
		return nil, err
	}
	if _, err := r.db.Query(ctx, `DELETE FROM webmentions WHERE id = ?`, old.ID); err != nil {
		return nil, err
	}
	if old.Status == Approved {
		return t.paths(), nil
	}
	return nil, nil
}

// store inserts or updates the mention. Likes and reposts show at once;
// replies and mentions wait for approval, and go back to waiting when an
// approved one changes its words. Rejected stays rejected.
func (r *Receiver) store(ctx context.Context, m Received, t target, p Parsed) ([]string, error) {
	old, err := r.find(ctx, m.Source, t.ID)
	if err != nil {
		return nil, err
	}
	quiet := p.Type == Like || p.Type == Repost
	status := Pending
	switch {
	case old != nil && old.Status == Rejected:
		status = Rejected
	case quiet:
		status = Approved
	case old != nil && old.Status == Approved && old.Type == p.Type && old.Content == p.Content && old.Author == p.Author:
		status = Approved
	}
	avatar := ""
	if old != nil {
		avatar = old.Avatar
	}
	avatar = r.avatar(ctx, p.Photo, avatar)
	now := content.Clock().Unix()
	if old == nil {
		_, err = r.db.Query(ctx, `INSERT INTO webmentions (id, target_entry_id, target_url, source_url, author, author_url,
  author_avatar, content, type, status, published_at, received_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (source_url, target_entry_id) DO NOTHING`,
			content.NewID(), t.ID, m.Target, m.Source, p.Author, p.AuthorURL, avatar, p.Content, p.Type, status, p.Published, now, now)
	} else {
		_, err = r.db.Query(ctx, `UPDATE webmentions SET target_url = ?, author = ?, author_url = ?, author_avatar = ?, content = ?,
  type = ?, status = ?, published_at = ?, updated_at = ? WHERE id = ?`,
			m.Target, p.Author, p.AuthorURL, avatar, p.Content, p.Type, status, p.Published, now, old.ID)
	}
	if err != nil {
		return nil, err
	}
	if status == Approved || (old != nil && old.Status == Approved) {
		return t.paths(), nil
	}
	return nil, nil
}

// avatar copies the author's photo into R2, once per photo URL, and
// returns its public path; have is the path stored before. A photo that
// cannot be fetched or is not a raster image leaves the avatar empty (the
// page shows the initial).
func (r *Receiver) avatar(ctx context.Context, photo, have string) string {
	if photo == "" || r.blob == nil {
		return ""
	}
	sum := sha256.Sum256([]byte(photo))
	name := hex.EncodeToString(sum[:12])
	if strings.HasPrefix(have, AvatarPrefix+name+".") {
		return have
	}
	res, err := r.get.Get(ctx, photo, MaxAvatar)
	if err != nil || res.Status != http.StatusOK {
		slog.Info("webmention avatar", "url", photo, "err", err)
		return ""
	}
	ext := fetch.IconExt(res.Body)
	if ext == "" {
		return ""
	}
	key := strings.TrimPrefix(AvatarPrefix, "/") + name + "." + ext
	if err := r.blob.Put(ctx, key, res.Body, avatarMIME[ext]); err != nil {
		slog.Error("webmention avatar", "key", key, "err", err)
		return ""
	}
	return "/" + key
}

func errNotFound(id string) error { return content.NotFound("webmention", id) }

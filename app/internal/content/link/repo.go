package link

import (
	"context"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Repo reads and writes links.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectLink = `SELECT ` + content.EntryColumns + `, l.url, l.domain, l.favicon_url, l.category_id, l.via, l.archived_url, l.read_status
FROM entries e JOIN links l ON l.entry_id = e.id`

type row struct {
	content.Entry
	URL         string  `json:"url"`
	Domain      string  `json:"domain"`
	FaviconURL  *string `json:"favicon_url"`
	CategoryID  *string `json:"category_id"`
	Via         *string `json:"via"`
	ArchivedURL *string `json:"archived_url"`
	ReadStatus  string  `json:"read_status"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (r row) link() Link {
	return Link{Entry: r.Entry, URL: r.URL, Domain: r.Domain, FaviconURL: str(r.FaviconURL), CategoryID: r.CategoryID,
		Via: str(r.Via), ArchivedURL: str(r.ArchivedURL), ReadStatus: r.ReadStatus}
}

func ownRow(id string, l Link) store.Stmt {
	return store.Stmt{
		SQL: `INSERT INTO links (entry_id, url, domain, favicon_url, category_id, via, archived_url, read_status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET url = excluded.url, domain = excluded.domain, favicon_url = excluded.favicon_url,
  category_id = excluded.category_id, via = excluded.via, archived_url = excluded.archived_url, read_status = excluded.read_status`,
		Args: []any{id, l.URL, l.Domain, content.NullIfEmpty(l.FaviconURL), l.CategoryID, content.NullIfEmpty(l.Via),
			content.NullIfEmpty(l.ArchivedURL), l.ReadStatus},
	}
}

func prepare(d *Draft) {
	d.Kind = Kind
	d.Domain = content.Domain(d.URL)
	if d.ReadStatus == "" {
		d.ReadStatus = Unread
	}
}

func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML}, nil
}

func (r *Repo) check(ctx context.Context, l Link) error {
	if err := Validate(l); err != nil {
		return err
	}
	return content.CheckCategory(ctx, r.db, l.CategoryID, Kind)
}

// Create validates and inserts a draft link.
func (r *Repo) Create(ctx context.Context, d Draft) (Link, error) {
	prepare(&d)
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if err := r.check(ctx, d.Link); err != nil {
		return Link{}, err
	}
	b, err := body(d)
	if err != nil {
		return Link{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return []store.Stmt{ownRow(id, d.Link)} })
	return d.Link, err
}

// Save writes the fields and a revision.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Link, error) {
	prepare(&d)
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Link{}, err
	}
	if err := r.check(ctx, d.Link); err != nil {
		return Link{}, err
	}
	b, err := body(d)
	if err != nil {
		return Link{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, []store.Stmt{ownRow(d.ID, d.Link)}); err != nil {
		return Link{}, err
	}
	return r.Get(ctx, d.ID)
}

// SetRead flips read or unread without a revision: it is a reading log, not
// an edit (the links page toggle, Phase 2.3).
func (r *Repo) SetRead(ctx context.Context, id string, read bool) error {
	status := Unread
	if read {
		status = Read
	}
	res, err := r.db.Query(ctx, `UPDATE links SET read_status = ? WHERE entry_id = ?`, status, id)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return content.NotFound(Kind, id)
	}
	return nil
}

// Publish makes the link public.
func (r *Repo) Publish(ctx context.Context, id string) (Link, error) {
	l, err := r.Get(ctx, id)
	if err != nil {
		return Link{}, err
	}
	err = content.PublishEntry(ctx, r.db, l.Entry, func(e content.Entry) error {
		x := l
		x.Entry = e
		return Validate(x)
	})
	if err != nil {
		return Link{}, err
	}
	return r.Get(ctx, id)
}

// Get loads a link by id, any status.
func (r *Repo) Get(ctx context.Context, id string) (Link, error) {
	res, err := r.db.Query(ctx, selectLink+" WHERE e.id = ?", id)
	if err != nil {
		return Link{}, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return Link{}, err
	}
	if len(rows) == 0 {
		return Link{}, content.NotFound(Kind, id)
	}
	return rows[0].link(), nil
}

// ListFilter selects public links for the links page.
type ListFilter struct {
	Lang       string
	ReadStatus string // "" for both
	CategoryID string
}

// List returns published public links, newest first.
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Link, error) {
	q := selectLink + ` WHERE e.lang = ? AND e.status = 'published' AND e.visibility = 'public'`
	args := []any{f.Lang}
	if f.ReadStatus == Read || f.ReadStatus == Unread {
		q += ` AND l.read_status = ?`
		args = append(args, f.ReadStatus)
	}
	if f.CategoryID != "" {
		q += ` AND l.category_id = ?`
		args = append(args, f.CategoryID)
	}
	res, err := r.db.Query(ctx, q+` ORDER BY e.published_at DESC, e.id DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return nil, err
	}
	out := make([]Link, len(rows))
	for i, x := range rows {
		out[i] = x.link()
	}
	return out, nil
}

// Archive takes a link off the site.
func (r *Repo) Archive(ctx context.Context, id string) error {
	return content.ArchiveEntry(ctx, r.db, Kind, id)
}

// Delete removes a link.
func (r *Repo) Delete(ctx context.Context, id string) error {
	return content.DeleteEntry(ctx, r.db, Kind, id)
}

package fav

import (
	"context"
	"slices"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Repo reads and writes favs.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectFav = `SELECT ` + content.EntryColumns + `, f.fav_type, f.author, f.url, f.rating, f.category_id
FROM entries e JOIN favs f ON f.entry_id = e.id`

type row struct {
	content.Entry
	Type       string  `json:"fav_type"`
	Author     *string `json:"author"`
	URL        *string `json:"url"`
	Rating     *int    `json:"rating"`
	CategoryID *string `json:"category_id"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}

func (r row) fav() Fav {
	return Fav{Entry: r.Entry, Type: r.Type, Author: str(r.Author), URL: str(r.URL), Rating: r.Rating, CategoryID: r.CategoryID}
}

func ownRow(id string, f Fav) store.Stmt {
	return store.Stmt{
		SQL: `INSERT INTO favs (entry_id, fav_type, author, url, rating, category_id) VALUES (?, ?, ?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET fav_type = excluded.fav_type, author = excluded.author, url = excluded.url,
  rating = excluded.rating, category_id = excluded.category_id`,
		Args: []any{id, f.Type, content.NullIfEmpty(f.Author), content.NullIfEmpty(f.URL), f.Rating, f.CategoryID},
	}
}

func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML}, nil
}

func (r *Repo) check(ctx context.Context, f Fav) error {
	if err := Validate(f); err != nil {
		return err
	}
	return content.CheckCategory(ctx, r.db, f.CategoryID, Kind)
}

// Create validates and inserts a draft fav.
func (r *Repo) Create(ctx context.Context, d Draft) (Fav, error) {
	d.Kind = Kind
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if err := r.check(ctx, d.Fav); err != nil {
		return Fav{}, err
	}
	b, err := body(d)
	if err != nil {
		return Fav{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return []store.Stmt{ownRow(id, d.Fav)} })
	return d.Fav, err
}

// Save writes the fields and a revision.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Fav, error) {
	d.Kind = Kind
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Fav{}, err
	}
	if err := r.check(ctx, d.Fav); err != nil {
		return Fav{}, err
	}
	b, err := body(d)
	if err != nil {
		return Fav{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, []store.Stmt{ownRow(d.ID, d.Fav)}); err != nil {
		return Fav{}, err
	}
	return r.Get(ctx, d.ID)
}

// Publish makes the fav public.
func (r *Repo) Publish(ctx context.Context, id string) (Fav, error) {
	f, err := r.Get(ctx, id)
	if err != nil {
		return Fav{}, err
	}
	err = content.PublishEntry(ctx, r.db, f.Entry, func(e content.Entry) error {
		x := f
		x.Entry = e
		return Validate(x)
	})
	if err != nil {
		return Fav{}, err
	}
	return r.Get(ctx, id)
}

// Get loads a fav by id, any status.
func (r *Repo) Get(ctx context.Context, id string) (Fav, error) {
	res, err := r.db.Query(ctx, selectFav+" WHERE e.id = ?", id)
	if err != nil {
		return Fav{}, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return Fav{}, err
	}
	if len(rows) == 0 {
		return Fav{}, content.NotFound(Kind, id)
	}
	return rows[0].fav(), nil
}

// List returns published public favs in lang, optionally of one type.
func (r *Repo) List(ctx context.Context, lang, typ string) ([]Fav, error) {
	q := selectFav + ` WHERE ` + content.LangOrFallback("e") + ` AND e.status = 'published' AND e.visibility = 'public'`
	args := []any{lang, lang}
	if slices.Contains(Types, typ) {
		q += ` AND f.fav_type = ?`
		args = append(args, typ)
	}
	res, err := r.db.Query(ctx, q+` ORDER BY e.published_at DESC, e.id DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return nil, err
	}
	out := make([]Fav, len(rows))
	for i, x := range rows {
		out[i] = x.fav()
	}
	return out, nil
}

// Archive takes a fav off the site.
func (r *Repo) Archive(ctx context.Context, id string) error {
	return content.ArchiveEntry(ctx, r.db, Kind, id)
}

// Delete removes a fav.
func (r *Repo) Delete(ctx context.Context, id string) error {
	return content.DeleteEntry(ctx, r.db, Kind, id)
}

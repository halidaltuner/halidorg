package note

import (
	"context"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Repo reads and writes notes.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectNote = `SELECT ` + content.EntryColumns + `, n.source_url, n.til
FROM entries e JOIN notes n ON n.entry_id = e.id`

// selectPublished adds the published body for the public reads.
const selectPublished = `SELECT ` + content.EntryColumns + `, n.source_url, n.til, r.body_html
FROM entries e JOIN notes n ON n.entry_id = e.id JOIN revisions r ON r.id = e.published_revision_id`

type row struct {
	content.Entry
	SourceURL *string `json:"source_url"`
	TIL       int     `json:"til"`
	BodyHTML  *string `json:"body_html"`
}

func (r row) note() Note {
	n := Note{Entry: r.Entry, TIL: r.TIL == 1}
	if r.BodyHTML != nil {
		n.HTML = *r.BodyHTML
	}
	if r.SourceURL != nil {
		n.SourceURL = *r.SourceURL
	}
	return n
}

func ownRow(id string, n Note) store.Stmt {
	return store.Stmt{
		SQL: `INSERT INTO notes (entry_id, source_url, til) VALUES (?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET source_url = excluded.source_url, til = excluded.til`,
		Args: []any{id, content.NullIfEmpty(n.SourceURL), n.TIL},
	}
}

func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML, TOCJSON: out.TOCJSON()}, nil
}

// Create validates, renders and inserts a draft note.
func (r *Repo) Create(ctx context.Context, d Draft) (Note, error) {
	d.Kind = Kind
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if err := Validate(d.Note, d.Markdown); err != nil {
		return Note{}, err
	}
	b, err := body(d)
	if err != nil {
		return Note{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return []store.Stmt{ownRow(id, d.Note)} })
	return d.Note, err
}

// Save writes the fields and a revision (origin autosave or manual).
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Note, error) {
	d.Kind = Kind
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Note{}, err
	}
	if err := Validate(d.Note, d.Markdown); err != nil {
		return Note{}, err
	}
	b, err := body(d)
	if err != nil {
		return Note{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, []store.Stmt{ownRow(d.ID, d.Note)}); err != nil {
		return Note{}, err
	}
	return r.Get(ctx, d.ID)
}

// Publish points the published revision at the current one.
func (r *Repo) Publish(ctx context.Context, id string) (Note, error) {
	n, err := r.Get(ctx, id)
	if err != nil {
		return Note{}, err
	}
	md, err := r.currentMD(ctx, n)
	if err != nil {
		return Note{}, err
	}
	err = content.PublishEntry(ctx, r.db, n.Entry, func(e content.Entry) error {
		x := n
		x.Entry = e
		return Validate(x, md)
	})
	if err != nil {
		return Note{}, err
	}
	return r.Get(ctx, id)
}

func (r *Repo) currentMD(ctx context.Context, n Note) (string, error) {
	if n.CurrentRevisionID == nil {
		return "", nil
	}
	rev, err := content.GetRevision(ctx, r.db, *n.CurrentRevisionID)
	return rev.BodyMD, err
}

// Get loads a note by id, any status.
func (r *Repo) Get(ctx context.Context, id string) (Note, error) {
	return r.one(ctx, "e.id = ?", id)
}

// GetPublished loads a public note by language and slug, with the body of
// its published revision.
func (r *Repo) GetPublished(ctx context.Context, lang, slug string) (Note, error) {
	return r.query(ctx, selectPublished, "e.lang = ? AND e.slug = ? AND e.status = 'published' AND e.visibility <> 'private'", lang, slug)
}

func (r *Repo) one(ctx context.Context, where string, args ...any) (Note, error) {
	return r.query(ctx, selectNote, where, args...)
}

func (r *Repo) query(ctx context.Context, sel, where string, args ...any) (Note, error) {
	res, err := r.db.Query(ctx, sel+" WHERE "+where+" LIMIT 1", args...)
	if err != nil {
		return Note{}, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return Note{}, err
	}
	if len(rows) == 0 {
		return Note{}, content.NotFound(Kind, where)
	}
	return rows[0].note(), nil
}

// List returns published public notes in lang, newest first, with their
// published bodies.
func (r *Repo) List(ctx context.Context, lang string, limit int) ([]Note, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	res, err := r.db.Query(ctx, selectPublished+` WHERE `+content.LangOrFallback("e")+` AND e.status = 'published' AND e.visibility = 'public'
ORDER BY e.published_at DESC, e.id DESC LIMIT ?`, lang, lang, limit)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return nil, err
	}
	out := make([]Note, len(rows))
	for i, x := range rows {
		out[i] = x.note()
	}
	return out, nil
}

// Archive takes a note off the site.
func (r *Repo) Archive(ctx context.Context, id string) error {
	return content.ArchiveEntry(ctx, r.db, Kind, id)
}

// Delete removes a note.
func (r *Repo) Delete(ctx context.Context, id string) error {
	return content.DeleteEntry(ctx, r.db, Kind, id)
}

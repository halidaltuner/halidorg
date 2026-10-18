package page

import (
	"context"
	"encoding/json"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Repo reads and writes pages.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectPage = `SELECT ` + content.EntryColumns + `, p.template FROM entries e JOIN pages p ON p.entry_id = e.id`

func trim(ss []string) []string {
	out := ss[:0:0]
	for _, s := range ss {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// normalize trims the fields, drops empty rows and fixes the slug to the
// template.
func normalize(d *Draft) {
	d.Kind = Kind
	d.Slug = d.Template
	x := &d.Data
	x.Name, x.Place = strings.TrimSpace(x.Name), strings.TrimSpace(x.Place)
	x.Portrait, x.CV = strings.TrimSpace(x.Portrait), strings.TrimSpace(x.CV)
	ctas := x.CTAs[:0:0]
	for _, l := range x.CTAs {
		l.Label, l.URL = strings.TrimSpace(l.Label), strings.TrimSpace(l.URL)
		if l.Label != "" || l.URL != "" {
			ctas = append(ctas, l)
		}
	}
	x.CTAs = ctas
	skills := x.Skills[:0:0]
	for _, s := range x.Skills {
		s.Group, s.Items = strings.TrimSpace(s.Group), trim(s.Items)
		if s.Group != "" || len(s.Items) > 0 {
			skills = append(skills, s)
		}
	}
	x.Skills = skills
	jobs := x.Experience[:0:0]
	for _, j := range x.Experience {
		j.From, j.To, j.Role = strings.TrimSpace(j.From), strings.TrimSpace(j.To), strings.TrimSpace(j.Role)
		j.Org, j.Note = strings.TrimSpace(j.Org), strings.TrimSpace(j.Note)
		if j.From != "" || j.To != "" || j.Role != "" || j.Org != "" || j.Note != "" {
			jobs = append(jobs, j)
		}
	}
	x.Experience = jobs
	builds := x.Building[:0:0]
	for _, b := range x.Building {
		b.Title, b.Note, b.Stage = strings.TrimSpace(b.Title), strings.TrimSpace(b.Note), strings.TrimSpace(b.Stage)
		b.Href, b.Cover = strings.TrimSpace(b.Href), strings.TrimSpace(b.Cover)
		if b.Title != "" || b.Note != "" || b.Stage != "" || b.Href != "" || b.Cover != "" {
			builds = append(builds, b)
		}
	}
	x.Building = builds
	books := x.Reading[:0:0]
	for _, b := range x.Reading {
		b.Title, b.Author, b.Cover = strings.TrimSpace(b.Title), strings.TrimSpace(b.Author), strings.TrimSpace(b.Cover)
		if b.Title != "" || b.Author != "" || b.Cover != "" {
			books = append(books, b)
		}
	}
	x.Reading = books
	learning := x.Learning[:0:0]
	for _, l := range x.Learning {
		l.Title, l.Note = strings.TrimSpace(l.Title), strings.TrimSpace(l.Note)
		if l.Title != "" || l.Note != "" {
			learning = append(learning, l)
		}
	}
	x.Learning = learning
	x.NotDoing = trim(x.NotDoing)
	// Each template keeps only its own sections.
	if d.Template == About {
		x.Place, x.Building, x.Reading, x.Learning, x.NotDoing = "", nil, nil, nil, nil
	} else {
		x.Name, x.Portrait, x.CV, x.CTAs, x.Skills, x.Experience = "", "", "", nil, nil, nil
	}
}

func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	raw, err := json.Marshal(d.Data)
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML, TOCJSON: out.TOCJSON(), DataJSON: string(raw)}, nil
}

// DataOf reads a revision's data_json.
func DataOf(dataJSON string) Data {
	var d Data
	_ = json.Unmarshal([]byte(dataJSON), &d)
	return d
}

// check validates p and the media it names: images with both alt texts
// (portrait, covers) and a PDF for the CV.
func (r *Repo) check(ctx context.Context, p Page) error {
	if err := Validate(p); err != nil {
		return err
	}
	var images []string
	if p.Data.Portrait != "" {
		images = append(images, p.Data.Portrait)
	}
	for _, b := range p.Data.Building {
		if b.Cover != "" {
			images = append(images, b.Cover)
		}
	}
	for _, b := range p.Data.Reading {
		if b.Cover != "" {
			images = append(images, b.Cover)
		}
	}
	if len(images) > 0 {
		args := make([]any, len(images))
		seen := map[string]bool{}
		for i, m := range images {
			args[i] = m
			seen[m] = true
		}
		res, err := r.db.Query(ctx, `SELECT COUNT(*) AS n FROM media WHERE mime LIKE 'image/%' AND trim(alt_tr) <> '' AND trim(alt_en) <> ''
  AND id IN (?`+strings.Repeat(", ?", len(args)-1)+`)`, args...)
		if err != nil {
			return err
		}
		rows, err := store.Scan[struct {
			N int `json:"n"`
		}](res)
		if err != nil {
			return err
		}
		if len(rows) == 0 || rows[0].N != len(seen) {
			return content.ValidationError{"media": "every image (portrait, covers) must exist and have both alt texts"}
		}
	}
	if p.Data.CV != "" {
		res, err := r.db.Query(ctx, `SELECT COUNT(*) AS n FROM media WHERE id = ? AND mime = 'application/pdf'`, p.Data.CV)
		if err != nil {
			return err
		}
		rows, err := store.Scan[struct {
			N int `json:"n"`
		}](res)
		if err != nil {
			return err
		}
		if len(rows) == 0 || rows[0].N != 1 {
			return content.ValidationError{"cv": "must be an uploaded PDF"}
		}
	}
	return nil
}

func ownRows(id string, p Page) []store.Stmt {
	return []store.Stmt{{SQL: `INSERT INTO pages (entry_id, template) VALUES (?, ?)
ON CONFLICT (entry_id) DO UPDATE SET template = excluded.template`, Args: []any{id, p.Template}}}
}

// Create validates and inserts a draft page.
func (r *Repo) Create(ctx context.Context, d Draft) (Page, error) {
	normalize(&d)
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if err := r.check(ctx, d.Page); err != nil {
		return Page{}, err
	}
	b, err := body(d)
	if err != nil {
		return Page{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return ownRows(id, d.Page) })
	if err != nil && strings.Contains(err.Error(), "slug") {
		return Page{}, content.ValidationError{"template": "this language already has this page"}
	}
	return d.Page, err
}

// Save writes the fields and adds a revision. The template never changes.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Page, error) {
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Page{}, err
	}
	d.Template = cur.Template
	normalize(&d)
	if err := r.check(ctx, d.Page); err != nil {
		return Page{}, err
	}
	b, err := body(d)
	if err != nil {
		return Page{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, ownRows(d.ID, d.Page)); err != nil {
		return Page{}, err
	}
	return r.Get(ctx, d.ID)
}

// Publish points the published revision at the current one and records
// the version.
func (r *Repo) Publish(ctx context.Context, id string) (Page, error) {
	p, err := r.Get(ctx, id)
	if err != nil {
		return Page{}, err
	}
	if err := Validate(withStatus(p)); err != nil {
		return Page{}, err
	}
	if p.CurrentRevisionID == nil {
		return Page{}, content.ValidationError{"body": "nothing to publish"}
	}
	now := content.Clock().Unix()
	stmts := content.PublishStmts(p.ID, *p.CurrentRevisionID, now)
	stmts = append(stmts, store.Stmt{SQL: `INSERT INTO page_versions (entry_id, revision_id, published_at) VALUES (?, ?, ?)
ON CONFLICT (entry_id, revision_id) DO UPDATE SET published_at = excluded.published_at`, Args: []any{p.ID, *p.CurrentRevisionID, now}})
	if _, err := r.db.Batch(ctx, stmts...); err != nil {
		return Page{}, err
	}
	return r.Get(ctx, id)
}

func withStatus(p Page) Page {
	p.Status = content.Published
	if p.PublishedAt == nil {
		now := content.Clock().Unix()
		p.PublishedAt = &now
	}
	return p
}

// Get loads a page with its current data, any status.
func (r *Repo) Get(ctx context.Context, id string) (Page, error) {
	p, err := r.one(ctx, "e.id = ?", id)
	if err != nil || p.CurrentRevisionID == nil {
		return p, err
	}
	rev, err := content.GetRevision(ctx, r.db, *p.CurrentRevisionID)
	if err != nil {
		return Page{}, err
	}
	p.Data = DataOf(rev.DataJSON)
	return p, nil
}

// GetPublished loads the public page of a template in lang, with the body
// and data of its published revision.
func (r *Repo) GetPublished(ctx context.Context, lang, template string) (Page, error) {
	p, err := r.one(ctx, "e.lang = ? AND p.template = ? AND e.status = 'published' AND e.visibility = 'public'", lang, template)
	if err != nil || p.PublishedRevisionID == nil {
		return p, err
	}
	rev, err := content.GetRevision(ctx, r.db, *p.PublishedRevisionID)
	if err != nil {
		return Page{}, err
	}
	p.Data, p.HTML = DataOf(rev.DataJSON), rev.BodyHTML
	return p, nil
}

// Versions lists the published versions of a page, newest first.
func (r *Repo) Versions(ctx context.Context, entryID string, limit int) ([]Version, error) {
	res, err := r.db.Query(ctx, `SELECT revision_id, published_at FROM page_versions WHERE entry_id = ?
ORDER BY published_at DESC, revision_id DESC LIMIT ?`, entryID, limit)
	if err != nil {
		return nil, err
	}
	return store.Scan[Version](res)
}

// VersionData reads one version's data.
func (r *Repo) VersionData(ctx context.Context, revisionID string) (Data, error) {
	rev, err := content.GetRevision(ctx, r.db, revisionID)
	if err != nil {
		return Data{}, err
	}
	return DataOf(rev.DataJSON), nil
}

func (r *Repo) one(ctx context.Context, where string, args ...any) (Page, error) {
	res, err := r.db.Query(ctx, selectPage+" WHERE "+where+" LIMIT 1", args...)
	if err != nil {
		return Page{}, err
	}
	rows, err := store.Scan[Page](res)
	if err != nil {
		return Page{}, err
	}
	if len(rows) == 0 {
		return Page{}, content.NotFound(Kind, where)
	}
	return rows[0], nil
}

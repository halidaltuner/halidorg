package project

import (
	"context"
	"encoding/json"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Repo reads and writes projects.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectProject = `SELECT ` + content.EntryColumns + `, p.project_type, p.project_status, p.category_id, p.tagline,
  p.kind_line, p.github_repo, p.started_at, p.ended_at, p.featured, p.cost_total, p.currency, p.time_spent
FROM entries e JOIN projects p ON p.entry_id = e.id`

type row struct {
	content.Entry
	Type          string  `json:"project_type"`
	ProjectStatus string  `json:"project_status"`
	CategoryID    *string `json:"category_id"`
	Tagline       string  `json:"tagline"`
	KindLine      string  `json:"kind_line"`
	GitHubRepo    *string `json:"github_repo"`
	StartedAt     *int64  `json:"started_at"`
	EndedAt       *int64  `json:"ended_at"`
	Featured      int     `json:"featured"`
	CostTotal     *int64  `json:"cost_total"`
	Currency      string  `json:"currency"`
	TimeSpent     string  `json:"time_spent"`
}

func (r row) project() Project {
	p := Project{Entry: r.Entry, Type: r.Type, ProjectStatus: r.ProjectStatus, CategoryID: r.CategoryID, Tagline: r.Tagline,
		KindLine: r.KindLine, StartedAt: r.StartedAt, EndedAt: r.EndedAt, Featured: r.Featured == 1, CostTotal: r.CostTotal,
		Currency: r.Currency, TimeSpent: r.TimeSpent}
	if r.GitHubRepo != nil {
		p.GitHubRepo = *r.GitHubRepo
	}
	return p
}

// normalize trims the rows, drops empty ones, numbers them in order and
// fills defaults: a software project has no category, a part's currency is
// the project's, quantity at least 1.
func normalize(d *Draft) {
	d.Kind = Kind
	d.Tagline = strings.TrimSpace(d.Tagline)
	d.KindLine = strings.TrimSpace(d.KindLine)
	d.GitHubRepo = strings.TrimSpace(d.GitHubRepo)
	d.TimeSpent = strings.TrimSpace(d.TimeSpent)
	d.Currency = strings.ToUpper(strings.TrimSpace(d.Currency))
	if d.Currency == "" {
		d.Currency = "TRY"
	}
	if d.ProjectStatus == "" {
		d.ProjectStatus = "active"
	}
	if d.Type == Software {
		d.CategoryID = nil
	}
	facts := d.Facts[:0:0]
	for _, f := range d.Facts {
		f.Key, f.Value = strings.TrimSpace(f.Key), strings.TrimSpace(f.Value)
		if f.Key == "" && f.Value == "" {
			continue
		}
		f.Sort = len(facts)
		facts = append(facts, f)
	}
	d.Facts = facts
	stack := d.Stack[:0:0]
	for _, s := range d.Stack {
		if s = strings.TrimSpace(s); s != "" {
			stack = append(stack, s)
		}
	}
	d.Stack = stack
	parts := d.Parts[:0:0]
	for _, x := range d.Parts {
		x.Name, x.URL = strings.TrimSpace(x.Name), strings.TrimSpace(x.URL)
		x.Currency = strings.ToUpper(strings.TrimSpace(x.Currency))
		if x.Name == "" && x.Price == nil && x.URL == "" {
			continue
		}
		if x.Currency == "" {
			x.Currency = d.Currency
		}
		if x.Qty == 0 {
			x.Qty = 1
		}
		x.Sort = len(parts)
		parts = append(parts, x)
	}
	d.Parts = parts
	links := d.Links[:0:0]
	for _, l := range d.Links {
		l.Label, l.URL = strings.TrimSpace(l.Label), strings.TrimSpace(l.URL)
		if l.Label == "" && l.URL == "" {
			continue
		}
		l.Sort = len(links)
		links = append(links, l)
	}
	d.Links = links
	seen := map[string]bool{}
	gallery := d.Gallery[:0:0]
	for _, m := range d.Gallery {
		if m = strings.TrimSpace(m); m != "" && !seen[m] {
			seen[m] = true
			gallery = append(gallery, m)
		}
	}
	d.Gallery = gallery
}

func ownRows(id string, p Project) []store.Stmt {
	stmts := []store.Stmt{
		{SQL: `INSERT INTO projects (entry_id, project_type, project_status, category_id, tagline, kind_line, github_repo,
  started_at, ended_at, featured, cost_total, currency, time_spent) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET project_type = excluded.project_type, project_status = excluded.project_status,
  category_id = excluded.category_id, tagline = excluded.tagline, kind_line = excluded.kind_line,
  github_repo = excluded.github_repo, started_at = excluded.started_at, ended_at = excluded.ended_at,
  featured = excluded.featured, cost_total = excluded.cost_total, currency = excluded.currency, time_spent = excluded.time_spent`,
			Args: []any{id, p.Type, p.ProjectStatus, p.CategoryID, p.Tagline, p.KindLine, content.NullIfEmpty(p.GitHubRepo),
				p.StartedAt, p.EndedAt, p.Featured, p.CostTotal, p.Currency, p.TimeSpent}},
	}
	for _, t := range []string{"project_facts", "project_stack", "project_parts", "project_links", "project_gallery"} {
		stmts = append(stmts, store.Stmt{SQL: `DELETE FROM ` + t + ` WHERE project_id = ?`, Args: []any{id}})
	}
	for _, f := range p.Facts {
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO project_facts (project_id, key, value, sort) VALUES (?, ?, ?, ?)`,
			Args: []any{id, f.Key, f.Value, f.Sort}})
	}
	for i, s := range p.Stack {
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO project_stack (project_id, name, sort) VALUES (?, ?, ?)`,
			Args: []any{id, s, i}})
	}
	for _, x := range p.Parts {
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO project_parts (project_id, name, qty, price, currency, url, sort) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{id, x.Name, x.Qty, x.Price, x.Currency, content.NullIfEmpty(x.URL), x.Sort}})
	}
	for _, l := range p.Links {
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO project_links (project_id, label, url, sort) VALUES (?, ?, ?, ?)`,
			Args: []any{id, l.Label, l.URL, l.Sort}})
	}
	for i, m := range p.Gallery {
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO project_gallery (project_id, media_id, sort) VALUES (?, ?, ?)`,
			Args: []any{id, m, i}})
	}
	return stmts
}

// data is what a revision keeps of the rows, so a restore brings them back
// with the text.
type data struct {
	Facts   []Fact   `json:"facts"`
	Stack   []string `json:"stack"`
	Parts   []Part   `json:"parts"`
	Links   []Link   `json:"links"`
	Gallery []string `json:"gallery"`
}

func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	raw, err := json.Marshal(data{Facts: d.Facts, Stack: d.Stack, Parts: d.Parts, Links: d.Links, Gallery: d.Gallery})
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML, TOCJSON: out.TOCJSON(), DataJSON: string(raw)}, nil
}

// RowsOf fills p's rows from a revision's data_json.
func RowsOf(p *Project, dataJSON string) {
	var d data
	_ = json.Unmarshal([]byte(dataJSON), &d)
	p.Facts, p.Stack, p.Parts, p.Links, p.Gallery = d.Facts, d.Stack, d.Parts, d.Links, d.Gallery
}

// check validates p and what it points at: a homelab category, and gallery
// images that exist and have both alt texts (CONTENT_MODEL.md, media).
func (r *Repo) check(ctx context.Context, p Project) error {
	if err := Validate(p); err != nil {
		return err
	}
	if p.Type == Homelab {
		if err := content.CheckCategory(ctx, r.db, p.CategoryID, "homelab"); err != nil {
			return err
		}
	}
	images := append([]string(nil), p.Gallery...)
	if p.CoverMediaID != nil {
		images = append(images, *p.CoverMediaID)
	}
	if len(images) == 0 {
		return nil
	}
	args := make([]any, len(images))
	for i, m := range images {
		args[i] = m
	}
	res, err := r.db.Query(ctx, `SELECT COUNT(*) AS n FROM media WHERE trim(alt_tr) <> '' AND trim(alt_en) <> ''
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
	if want := len(uniq(images)); len(rows) == 0 || rows[0].N != want {
		return content.ValidationError{"gallery": "every image (gallery and cover) must exist and have both alt texts"}
	}
	return nil
}

// Create validates and inserts a draft project with its rows.
func (r *Repo) Create(ctx context.Context, d Draft) (Project, error) {
	normalize(&d)
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if err := r.check(ctx, d.Project); err != nil {
		return Project{}, err
	}
	b, err := body(d)
	if err != nil {
		return Project{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return ownRows(id, d.Project) })
	return d.Project, err
}

// Save writes the fields, replaces the rows and adds a revision.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Project, error) {
	normalize(&d)
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Project{}, err
	}
	if err := r.check(ctx, d.Project); err != nil {
		return Project{}, err
	}
	b, err := body(d)
	if err != nil {
		return Project{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, ownRows(d.ID, d.Project)); err != nil {
		return Project{}, err
	}
	return r.Get(ctx, d.ID)
}

// Publish points the published revision at the current one.
func (r *Repo) Publish(ctx context.Context, id string) (Project, error) {
	p, err := r.Get(ctx, id)
	if err != nil {
		return Project{}, err
	}
	err = content.PublishEntry(ctx, r.db, p.Entry, func(e content.Entry) error {
		x := p
		x.Entry = e
		return Validate(x)
	})
	if err != nil {
		return Project{}, err
	}
	return r.Get(ctx, id)
}

// Get loads a project with its current rows, any status.
func (r *Repo) Get(ctx context.Context, id string) (Project, error) {
	p, err := r.one(ctx, "e.id = ?", id)
	if err != nil {
		return p, err
	}
	return p, r.rows(ctx, &p)
}

// GetPublished loads a public project by language and slug, with the body
// and rows of its published revision (edits since then are not public yet).
func (r *Repo) GetPublished(ctx context.Context, lang, slug string) (Project, error) {
	p, err := r.one(ctx, "e.lang = ? AND e.slug = ? AND e.status = 'published' AND e.visibility <> 'private'", lang, slug)
	if err != nil || p.PublishedRevisionID == nil {
		return p, err
	}
	rev, err := content.GetRevision(ctx, r.db, *p.PublishedRevisionID)
	if err != nil {
		return Project{}, err
	}
	RowsOf(&p, rev.DataJSON)
	p.HTML = rev.BodyHTML
	return p, nil
}

// ListFilter selects projects for List.
type ListFilter struct {
	Lang  string
	Type  string // software, homelab or "" for both
	Admin bool   // every status; otherwise published and public only
}

// List returns projects without their rows, featured first, then newest.
func (r *Repo) List(ctx context.Context, f ListFilter) ([]Project, error) {
	where, args := []string{"1 = 1"}, []any{}
	if f.Lang != "" {
		where, args = append(where, "e.lang = ?"), append(args, f.Lang)
	}
	if f.Type != "" {
		where, args = append(where, "p.project_type = ?"), append(args, f.Type)
	}
	if !f.Admin {
		where = append(where, "e.status = 'published' AND e.visibility = 'public'")
	}
	res, err := r.db.Query(ctx, selectProject+" WHERE "+strings.Join(where, " AND ")+
		" ORDER BY p.featured DESC, coalesce(e.published_at, e.created_at) DESC, e.id DESC LIMIT 200", args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return nil, err
	}
	out := make([]Project, len(rows))
	for i, x := range rows {
		out[i] = x.project()
	}
	return out, nil
}

func (r *Repo) one(ctx context.Context, where string, args ...any) (Project, error) {
	res, err := r.db.Query(ctx, selectProject+" WHERE "+where+" LIMIT 1", args...)
	if err != nil {
		return Project{}, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return Project{}, err
	}
	if len(rows) == 0 {
		return Project{}, content.NotFound(Kind, where)
	}
	return rows[0].project(), nil
}

// rows loads the current rows of p from their tables in one batch.
func (r *Repo) rows(ctx context.Context, p *Project) error {
	res, err := r.db.Batch(ctx,
		store.Stmt{SQL: `SELECT key, value, sort FROM project_facts WHERE project_id = ? ORDER BY sort`, Args: []any{p.ID}},
		store.Stmt{SQL: `SELECT name FROM project_stack WHERE project_id = ? ORDER BY sort`, Args: []any{p.ID}},
		store.Stmt{SQL: `SELECT name, qty, price, currency, coalesce(url, '') AS url, sort FROM project_parts WHERE project_id = ? ORDER BY sort`, Args: []any{p.ID}},
		store.Stmt{SQL: `SELECT label, url, sort FROM project_links WHERE project_id = ? ORDER BY sort`, Args: []any{p.ID}},
		store.Stmt{SQL: `SELECT media_id FROM project_gallery WHERE project_id = ? ORDER BY sort`, Args: []any{p.ID}},
	)
	if err != nil {
		return err
	}
	if p.Facts, err = store.Scan[Fact](res[0]); err != nil {
		return err
	}
	stack, err := store.Scan[struct {
		Name string `json:"name"`
	}](res[1])
	if err != nil {
		return err
	}
	p.Stack = nil
	for _, s := range stack {
		p.Stack = append(p.Stack, s.Name)
	}
	if p.Parts, err = store.Scan[Part](res[2]); err != nil {
		return err
	}
	if p.Links, err = store.Scan[Link](res[3]); err != nil {
		return err
	}
	gallery, err := store.Scan[struct {
		ID string `json:"media_id"`
	}](res[4])
	if err != nil {
		return err
	}
	p.Gallery = nil
	for _, g := range gallery {
		p.Gallery = append(p.Gallery, g.ID)
	}
	return nil
}

// Archive takes a project off the site.
func (r *Repo) Archive(ctx context.Context, id string) error {
	return content.ArchiveEntry(ctx, r.db, Kind, id)
}

// DeleteStmts remove a project with its build log entries; the rows cascade.
// Without the first statement the build logs' own rows would cascade away
// and leave their entries behind.
func DeleteStmts(id string) []store.Stmt {
	return []store.Stmt{
		{SQL: `DELETE FROM entries WHERE kind = 'buildlog' AND id IN (SELECT entry_id FROM buildlogs WHERE project_entry_id = ?)`, Args: []any{id}},
		{SQL: `DELETE FROM entries WHERE id = ? AND kind = 'project'`, Args: []any{id}},
	}
}

// Delete removes a project, its rows and its build logs.
func (r *Repo) Delete(ctx context.Context, id string) error {
	if _, err := r.Get(ctx, id); err != nil {
		return err
	}
	_, err := r.db.Batch(ctx, DeleteStmts(id)...)
	return err
}

func uniq(ss []string) map[string]bool {
	out := map[string]bool{}
	for _, s := range ss {
		out[s] = true
	}
	return out
}

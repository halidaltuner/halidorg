package buildlog

import (
	"context"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/store"
)

// Repo reads and writes build logs.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

const selectLog = `SELECT ` + content.EntryColumns + `, b.project_entry_id, b.log_date, b.hours_spent,
  pe.slug AS project_slug, pe.title AS project_title
FROM entries e JOIN buildlogs b ON b.entry_id = e.id JOIN entries pe ON pe.id = b.project_entry_id`

func ownRow(id string, b Buildlog) store.Stmt {
	return store.Stmt{
		SQL: `INSERT INTO buildlogs (entry_id, project_entry_id, log_date, hours_spent) VALUES (?, ?, ?, ?)
ON CONFLICT (entry_id) DO UPDATE SET project_entry_id = excluded.project_entry_id, log_date = excluded.log_date,
  hours_spent = excluded.hours_spent`,
		Args: []any{id, b.ProjectID, b.LogDate, b.Hours},
	}
}

func body(d Draft) (content.Body, error) {
	out, err := render.Markdown([]byte(d.Markdown), render.Options{Lang: d.Lang})
	if err != nil {
		return content.Body{}, err
	}
	return content.Body{MD: d.Markdown, HTML: out.HTML, TOCJSON: out.TOCJSON()}, nil
}

// check validates b and its project: it must exist and be in b's
// language, since the log lives under the project's URL.
func (r *Repo) check(ctx context.Context, b Buildlog) error {
	if err := Validate(b); err != nil {
		return err
	}
	res, err := r.db.Query(ctx, `SELECT lang FROM entries WHERE id = ? AND kind = 'project'`, b.ProjectID)
	if err != nil {
		return err
	}
	rows, err := store.Scan[struct {
		Lang string `json:"lang"`
	}](res)
	if err != nil {
		return err
	}
	switch {
	case len(rows) == 0:
		return content.ValidationError{"project": "no such project"}
	case rows[0].Lang != b.Lang:
		return content.ValidationError{"project": "the project is in another language"}
	}
	return nil
}

// Create validates and inserts a draft build log.
func (r *Repo) Create(ctx context.Context, d Draft) (Buildlog, error) {
	d.Kind = Kind
	if d.Status == "" || d.Status == content.Published {
		d.Status = content.Draft
	}
	if d.Visibility == "" {
		d.Visibility = content.Public
	}
	if err := r.check(ctx, d.Buildlog); err != nil {
		return Buildlog{}, err
	}
	b, err := body(d)
	if err != nil {
		return Buildlog{}, err
	}
	err = content.CreateEntry(ctx, r.db, &d.Entry, b, d.Tags, func(id string) []store.Stmt { return []store.Stmt{ownRow(id, d.Buildlog)} })
	return d.Buildlog, err
}

// Save writes the fields and a revision.
func (r *Repo) Save(ctx context.Context, d Draft, origin string) (Buildlog, error) {
	d.Kind = Kind
	cur, err := r.Get(ctx, d.ID)
	if err != nil {
		return Buildlog{}, err
	}
	if err := r.check(ctx, d.Buildlog); err != nil {
		return Buildlog{}, err
	}
	b, err := body(d)
	if err != nil {
		return Buildlog{}, err
	}
	if _, err := content.SaveEntry(ctx, r.db, cur.Entry, &d.Entry, b, origin, d.Tags, []store.Stmt{ownRow(d.ID, d.Buildlog)}); err != nil {
		return Buildlog{}, err
	}
	return r.Get(ctx, d.ID)
}

// Publish points the published revision at the current one.
func (r *Repo) Publish(ctx context.Context, id string) (Buildlog, error) {
	b, err := r.Get(ctx, id)
	if err != nil {
		return Buildlog{}, err
	}
	err = content.PublishEntry(ctx, r.db, b.Entry, func(e content.Entry) error {
		x := b
		x.Entry = e
		return Validate(x)
	})
	if err != nil {
		return Buildlog{}, err
	}
	return r.Get(ctx, id)
}

// Get loads a build log by id, any status.
func (r *Repo) Get(ctx context.Context, id string) (Buildlog, error) {
	res, err := r.db.Query(ctx, selectLog+" WHERE e.id = ? LIMIT 1", id)
	if err != nil {
		return Buildlog{}, err
	}
	rows, err := store.Scan[Buildlog](res)
	if err != nil {
		return Buildlog{}, err
	}
	if len(rows) == 0 {
		return Buildlog{}, content.NotFound(Kind, id)
	}
	return rows[0], nil
}

// ForProject lists a project's build logs, newest first: every status for
// the admin, published and public ones otherwise.
func (r *Repo) ForProject(ctx context.Context, projectID string, admin bool) ([]Buildlog, error) {
	where := "b.project_entry_id = ?"
	if !admin {
		where += " AND e.status = 'published' AND e.visibility = 'public'"
	}
	res, err := r.db.Query(ctx, selectLog+" WHERE "+where+" ORDER BY b.log_date DESC, e.id DESC LIMIT 200", projectID)
	if err != nil {
		return nil, err
	}
	return store.Scan[Buildlog](res)
}

// Delete removes a build log.
func (r *Repo) Delete(ctx context.Context, id string) error {
	return content.DeleteEntry(ctx, r.db, Kind, id)
}

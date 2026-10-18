package buildlog

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Base, Text and TagList let the shared editor (shortadmin) edit a build log.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

type options struct {
	Projects    []shortadmin.Option
	Date, Hours string
	ProjectHref string // back to the project's editor
}

// NewEditor is the build log list and editor at /admin/buildlogs (Phase
// 4.1). It sits under projects in the sidebar; a project's editor links to
// a new log with ?project= filled in.
func NewEditor(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "buildlogs", Nav: "projects", Body: true, Fields: "templates/buildlog/admin_fields.html",
		Public: func(d shortadmin.Draft) string {
			b := d.(*Draft)
			if b.ProjectSlug == "" {
				return ""
			}
			return "/" + b.Lang + "/projects/" + b.ProjectSlug + "/log/" + b.Slug
		},
		New: func() shortadmin.Draft {
			return &Draft{Buildlog: Buildlog{Entry: content.Entry{Kind: Kind}, LogDate: content.Clock().UTC().Truncate(24 * 3600e9).Unix()}}
		},
		Prefill: func(req *http.Request, d shortadmin.Draft) {
			b := d.(*Draft)
			b.ProjectID = req.URL.Query().Get("project")
			// A log is in its project's language.
			res, err := db.Query(req.Context(), `SELECT lang FROM entries WHERE id = ? AND kind = 'project'`, b.ProjectID)
			if err != nil {
				return
			}
			if rows, _ := store.Scan[struct {
				Lang string `json:"lang"`
			}](res); len(rows) == 1 {
				b.Lang = rows[0].Lang
			}
		},
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			b, err := r.Get(ctx, id)
			return &Draft{Buildlog: b}, err
		},
		Read: func(req *http.Request, d shortadmin.Draft) {
			b := d.(*Draft)
			b.ProjectID = req.FormValue("project")
			b.LogDate = project.ParseDay(req.FormValue("log_date"))
			b.Hours = nil
			if h, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(req.FormValue("hours")), ",", "."), 64); err == nil {
				b.Hours = &h
			}
		},
		Create: func(ctx context.Context, d shortadmin.Draft) (string, error) {
			b, err := r.Create(ctx, *d.(*Draft))
			return b.ID, err
		},
		Save: func(ctx context.Context, d shortadmin.Draft, origin string) error {
			_, err := r.Save(ctx, *d.(*Draft), origin)
			return err
		},
		Publish: func(ctx context.Context, id string) error {
			_, err := r.Publish(ctx, id)
			return err
		},
		Options: func(ctx context.Context, lang string, d shortadmin.Draft) (any, error) {
			b := d.(*Draft)
			o := options{Date: project.FormatDay(b.LogDate)}
			if b.Hours != nil {
				o.Hours = strconv.FormatFloat(*b.Hours, 'f', -1, 64)
			}
			if b.ProjectID != "" {
				o.ProjectHref = admin.EditorHref("project", b.ProjectID)
			}
			res, err := db.Query(ctx, `SELECT id, title, lang FROM entries WHERE kind = 'project' ORDER BY lang, title`)
			if err != nil {
				return nil, err
			}
			ps, err := store.Scan[struct {
				ID    string `json:"id"`
				Title string `json:"title"`
				Lang  string `json:"lang"`
			}](res)
			if err != nil {
				return nil, err
			}
			for _, p := range ps {
				o.Projects = append(o.Projects, shortadmin.Option{Value: p.ID, Label: p.Title + " · " + p.Lang, On: p.ID == b.ProjectID})
			}
			return o, nil
		},
	}, db, u, a, j)
}

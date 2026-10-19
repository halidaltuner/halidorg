package term

import (
	"context"
	"net/http"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Base, Text and TagList let the shared editor (shortadmin) edit a term.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

// NewEditor is the glossary list and editor at /admin/glossary (derive).
// The title is the term; the body explains it.
func NewEditor(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "glossary", Body: true, Fields: "templates/term/admin_fields.html",
		Public: func(d shortadmin.Draft) string { e := d.Base(); return "/" + e.Lang + "/glossary/" + e.Slug },
		New:    func() shortadmin.Draft { return &Draft{Term: Term{Entry: content.Entry{Kind: Kind}}} },
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			t, err := r.Get(ctx, id)
			return &Draft{Term: t}, err
		},
		Read: func(req *http.Request, d shortadmin.Draft) {
			t := d.(*Draft)
			t.ShortDef = req.FormValue("short_def")
			t.Aliases = strings.Split(req.FormValue("aliases"), ",")
		},
		Create: func(ctx context.Context, d shortadmin.Draft) (string, error) {
			t, err := r.Create(ctx, *d.(*Draft))
			return t.ID, err
		},
		Save: func(ctx context.Context, d shortadmin.Draft, origin string) error {
			_, err := r.Save(ctx, *d.(*Draft), origin)
			return err
		},
		Publish: func(ctx context.Context, id string) error {
			_, err := r.Publish(ctx, id)
			return err
		},
		Options: func(_ context.Context, _ string, d shortadmin.Draft) (any, error) {
			return struct{ Aliases string }{strings.Join(d.(*Draft).Aliases, ", ")}, nil
		},
	}, db, u, a, j)
}

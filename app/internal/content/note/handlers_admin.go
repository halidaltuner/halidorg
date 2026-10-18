package note

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

// Base, Text and TagList let the shared editor (shortadmin) edit a note.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

// NewEditor is the note list and editor at /admin/notes (Phase 2.6).
func NewEditor(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "notes", Body: true, Fields: "templates/note/admin_fields.html",
		Public: func(d shortadmin.Draft) string { e := d.Base(); return "/" + e.Lang + "/notes/" + e.Slug },
		New:    func() shortadmin.Draft { return &Draft{Note: Note{Entry: content.Entry{Kind: Kind}}} },
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			n, err := r.Get(ctx, id)
			return &Draft{Note: n}, err
		},
		Read: func(req *http.Request, d shortadmin.Draft) {
			n := d.(*Draft)
			n.SourceURL = strings.TrimSpace(req.FormValue("source_url"))
			n.TIL = req.FormValue("til") == "1"
		},
		Create: func(ctx context.Context, d shortadmin.Draft) (string, error) {
			n, err := r.Create(ctx, *d.(*Draft))
			return n.ID, err
		},
		Save: func(ctx context.Context, d shortadmin.Draft, origin string) error {
			_, err := r.Save(ctx, *d.(*Draft), origin)
			return err
		},
		Publish: func(ctx context.Context, id string) error {
			_, err := r.Publish(ctx, id)
			return err
		},
	}, db, u, a, j)
}

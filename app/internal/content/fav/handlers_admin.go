package fav

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Base, Text and TagList let the shared editor (shortadmin) edit a fav.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

type options struct {
	Categories, Types, Ratings []shortadmin.Option
}

// NewEditor is the fav list and editor at /admin/favs (Phase 2.6). The
// comment is the summary; favs live on the favs page, so deleting one
// leaves no tombstone.
func NewEditor(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "favs", Fields: "templates/fav/admin_fields.html",
		New: func() shortadmin.Draft { return &Draft{Fav: Fav{Entry: content.Entry{Kind: Kind}, Type: "tool"}} },
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			f, err := r.Get(ctx, id)
			return &Draft{Fav: f}, err
		},
		Read: func(req *http.Request, d shortadmin.Draft) {
			f := d.(*Draft)
			f.Type = req.FormValue("fav_type")
			f.Author = strings.TrimSpace(req.FormValue("author"))
			f.URL = strings.TrimSpace(req.FormValue("url"))
			f.CategoryID = shortadmin.Ptr(req, "category")
			f.Rating = nil
			if n, err := strconv.Atoi(req.FormValue("rating")); err == nil {
				f.Rating = &n
			}
		},
		Create: func(ctx context.Context, d shortadmin.Draft) (string, error) {
			f, err := r.Create(ctx, *d.(*Draft))
			return f.ID, err
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
			f := d.(*Draft)
			cats, err := shortadmin.CategoryOptions(ctx, db, Kind, lang, f.CategoryID)
			if err != nil {
				return nil, err
			}
			o := options{Categories: cats}
			for _, t := range Types {
				o.Types = append(o.Types, shortadmin.Option{Value: t, Label: t, On: f.Type == t})
			}
			o.Ratings = []shortadmin.Option{{Value: "", Label: "–", On: f.Rating == nil}}
			for i := 1; i <= 5; i++ {
				o.Ratings = append(o.Ratings, shortadmin.Option{Value: strconv.Itoa(i), Label: strconv.Itoa(i), On: f.Rating != nil && *f.Rating == i})
			}
			return o, nil
		},
	}, db, u, a, j)
}

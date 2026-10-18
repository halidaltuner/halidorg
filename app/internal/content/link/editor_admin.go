package link

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

// Base, Text and TagList let the shared editor (shortadmin) edit a link.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

type options struct {
	Categories []shortadmin.Option
	Read       []shortadmin.Option
}

// NewEditor is the link list and editor at /admin/links (Phase 2.6). The
// comment is the summary; links have no page of their own, so deleting one
// leaves no tombstone.
func NewEditor(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "links", Fields: "templates/link/admin_fields.html",
		New: func() shortadmin.Draft {
			return &Draft{Link: Link{Entry: content.Entry{Kind: Kind}, ReadStatus: Unread}}
		},
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			l, err := r.Get(ctx, id)
			return &Draft{Link: l}, err
		},
		Read: func(req *http.Request, d shortadmin.Draft) {
			l := d.(*Draft)
			if u := strings.TrimSpace(req.FormValue("url")); u != l.URL {
				l.URL, l.FaviconURL = u, "" // a new address gets its own icon
			}
			l.CategoryID = shortadmin.Ptr(req, "category")
			l.Via = strings.TrimSpace(req.FormValue("via"))
			l.ArchivedURL = strings.TrimSpace(req.FormValue("archived_url"))
			if s := req.FormValue("read_status"); s == Read || s == Unread {
				l.ReadStatus = s
			}
		},
		Create: func(ctx context.Context, d shortadmin.Draft) (string, error) {
			l, err := r.Create(ctx, *d.(*Draft))
			return l.ID, err
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
			l := d.(*Draft)
			cats, err := shortadmin.CategoryOptions(ctx, db, Kind, lang, l.CategoryID)
			if err != nil {
				return nil, err
			}
			read := []shortadmin.Option{{Value: Unread, On: l.ReadStatus != Read}, {Value: Read, On: l.ReadStatus == Read}}
			return options{Categories: cats, Read: read}, nil
		},
	}, db, u, a, j)
}

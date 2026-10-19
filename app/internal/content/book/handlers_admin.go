package book

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Base, Text and TagList let the shared editor (shortadmin) edit a book.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

type options struct {
	States, Ratings, Covers []shortadmin.Option
	Started, Finished       string
}

// NewEditor is the reading list and editor at /admin/reading (derive). The
// summary is the short take shown on the reading page; the body is notes.
// Books live on the reading page, so deleting one leaves no tombstone.
func NewEditor(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "reading", Body: true, Fields: "templates/book/admin_fields.html",
		New: func() shortadmin.Draft { return &Draft{Book: Book{Entry: content.Entry{Kind: Kind}, State: "reading"}} },
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			b, err := r.Get(ctx, id)
			return &Draft{Book: b}, err
		},
		Read: func(req *http.Request, d shortadmin.Draft) {
			b := d.(*Draft)
			b.Author, b.ISBN, b.State = req.FormValue("author"), req.FormValue("isbn"), req.FormValue("book_status")
			b.Progress, _ = strconv.Atoi(strings.TrimSpace(strings.TrimSuffix(req.FormValue("progress_pct"), "%")))
			b.StartedAt, b.FinishedAt = day(req.FormValue("started")), day(req.FormValue("finished"))
			b.Rating = nil
			if n, err := strconv.Atoi(req.FormValue("rating")); err == nil {
				b.Rating = &n
			}
			b.CoverMediaID = shortadmin.Ptr(req, "cover")
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
			o := options{Started: Day(b.StartedAt), Finished: Day(b.FinishedAt)}
			for _, s := range Statuses {
				o.States = append(o.States, shortadmin.Option{Value: s, Label: i18n.T(lang, "reading.status."+s), On: b.State == s})
			}
			o.Ratings = []shortadmin.Option{{Value: "", Label: "–", On: b.Rating == nil}}
			for i := 1; i <= 5; i++ {
				o.Ratings = append(o.Ratings, shortadmin.Option{Value: strconv.Itoa(i), Label: strconv.Itoa(i), On: b.Rating != nil && *b.Rating == i})
			}
			res, err := db.Query(ctx, `SELECT id, name FROM media WHERE mime LIKE 'image/%' ORDER BY created_at DESC LIMIT 200`)
			if err != nil {
				return nil, err
			}
			media, err := store.Scan[struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			}](res)
			if err != nil {
				return nil, err
			}
			o.Covers = []shortadmin.Option{{Value: "", Label: i18n.T(lang, "kind.book.no_cover"), On: b.CoverMediaID == nil}}
			for _, m := range media {
				name := m.Name
				if name == "" {
					name = m.ID
				}
				o.Covers = append(o.Covers, shortadmin.Option{Value: m.ID, Label: name, On: b.CoverMediaID != nil && *b.CoverMediaID == m.ID})
			}
			return o, nil
		},
	}, db, u, a, j)
}

// Day writes unix seconds for a date field.
func Day(v *int64) string {
	if v == nil || *v <= 0 {
		return ""
	}
	return time.Unix(*v, 0).UTC().Format("2006-01-02")
}

// day reads a date field: nil when empty, -1 when not a date (Validate
// reports it).
func day(s string) *int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	v := int64(-1)
	if t, err := time.Parse("2006-01-02", s); err == nil {
		v = t.Unix()
	}
	return &v
}

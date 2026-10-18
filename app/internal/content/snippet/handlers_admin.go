package snippet

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Base, Text and TagList let the shared editor (shortadmin) edit a snippet.
func (d *Draft) Base() *content.Entry { return &d.Entry }
func (d *Draft) Text() *string        { return &d.Markdown }
func (d *Draft) TagList() *[]string   { return &d.Tags }

type options struct {
	Categories []shortadmin.Option
	Files      []File // the files, then one empty row to add a file
}

// NewEditor is the snippet list and editor at /admin/snippets (Phase 2.6).
// Files are rows of name, language, order and code; a row with neither name
// nor code is dropped, so emptying both removes a file and the empty last
// row adds one, without JS.
func NewEditor(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*shortadmin.Admin, error) {
	r := NewRepo(db)
	return shortadmin.New(shortadmin.Kind{
		Name: Kind, Section: "snippets", Body: true, Fields: "templates/snippet/admin_fields.html",
		Public: func(e content.Entry) string { return "/" + e.Lang + "/snippets/" + e.Slug },
		New:    func() shortadmin.Draft { return &Draft{Snippet: Snippet{Entry: content.Entry{Kind: Kind}}} },
		Load: func(ctx context.Context, id string) (shortadmin.Draft, error) {
			s, err := r.Get(ctx, id)
			return &Draft{Snippet: s}, err
		},
		Read: func(req *http.Request, d shortadmin.Draft) {
			s := d.(*Draft)
			s.CategoryID = shortadmin.Ptr(req, "category")
			s.GistURL = strings.TrimSpace(req.FormValue("gist_url"))
			s.Files = readFiles(req)
		},
		Create: func(ctx context.Context, d shortadmin.Draft) (string, error) {
			s, err := r.Create(ctx, *d.(*Draft))
			return s.ID, err
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
			s := d.(*Draft)
			cats, err := shortadmin.CategoryOptions(ctx, db, Kind, lang, s.CategoryID)
			if err != nil {
				return nil, err
			}
			return options{Categories: cats, Files: append(append([]File(nil), s.Files...), File{})}, nil
		},
	}, db, u, a, j)
}

// readFiles reads the file rows in their order field (ties keep the form
// order) and drops the empty ones.
func readFiles(r *http.Request) []File {
	_ = r.ParseForm()
	names, langs, codes, orders := r.PostForm["file_name"], r.PostForm["file_lang"], r.PostForm["file_code"], r.PostForm["file_order"]
	type row struct {
		f     File
		order int
	}
	var rows []row
	for i := range names {
		f := File{Filename: strings.TrimSpace(names[i])}
		if i < len(langs) {
			f.Language = strings.TrimSpace(langs[i])
		}
		if i < len(codes) {
			f.Code = codes[i]
		}
		if f.Filename == "" && strings.TrimSpace(f.Code) == "" {
			continue
		}
		o := i
		if i < len(orders) {
			if n, err := strconv.Atoi(strings.TrimSpace(orders[i])); err == nil {
				o = n
			}
		}
		rows = append(rows, row{f, o})
	}
	sort.SliceStable(rows, func(a, b int) bool { return rows[a].order < rows[b].order })
	out := make([]File, len(rows))
	for i, x := range rows {
		out[i] = x.f
	}
	return out
}

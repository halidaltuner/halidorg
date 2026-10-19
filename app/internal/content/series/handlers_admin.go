package series

import (
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Admin serves /admin/series (derive: dashboard density): the list, and a
// form per series with its parts. Entries join a series in their editor.
type Admin struct {
	db   store.DB
	repo *Repo
	ui   *admin.UI
	auth *auth.Auth
	jobs *jobs.Jobs
	list *template.Template
	edit *template.Template
}

// NewAdmin parses the pages.
func NewAdmin(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*Admin, error) {
	list, err := u.Parse("series", "templates/series/admin_list.html")
	if err != nil {
		return nil, err
	}
	edit, err := u.Parse("series-edit", "templates/series/admin_edit.html")
	if err != nil {
		return nil, err
	}
	return &Admin{db: db, repo: NewRepo(db), ui: u, auth: a, jobs: j, list: list, edit: edit}, nil
}

// Register adds the routes: writes go through admin.Guard inside a session;
// deleting needs a recent sign-in.
func (h *Admin) Register(mux *http.ServeMux) {
	s := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(s(f)) }
	mux.Handle("GET /admin/series", s(h.listPage))
	mux.Handle("GET /admin/series/new", s(h.newPage))
	mux.Handle("POST /admin/series", w(h.save))
	mux.Handle("GET /admin/series/{id}", s(h.editPage))
	mux.Handle("POST /admin/series/{id}", w(h.save))
	mux.Handle("POST /admin/series/{id}/delete", admin.Guard(h.auth.RequireSession(h.auth.RequireRecentAuth(http.HandlerFunc(h.delete)))))
}

type listRow struct {
	Href, Title, Slug, Status, StatusKey string
	Parts                                int
}

type listPage struct {
	admin.Shell
	Rows   []listRow
	Notice string
}

func (h *Admin) listPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	p := listPage{Shell: h.ui.Shell(r, i18n.T(lang, "admin.nav.series"), "series")}
	if r.URL.Query().Get("done") == "deleted" {
		p.Notice = i18n.T(lang, "admin.series.deleted")
	}
	all, err := h.repo.All(ctx)
	if err != nil {
		h.fail(w, err)
		return
	}
	res, err := h.db.Query(ctx, `SELECT series_id, count(*) AS n FROM entries WHERE series_id IS NOT NULL GROUP BY series_id`)
	if err != nil {
		h.fail(w, err)
		return
	}
	counts, err := store.Scan[struct {
		ID string `json:"series_id"`
		N  int    `json:"n"`
	}](res)
	if err != nil {
		h.fail(w, err)
		return
	}
	n := map[string]int{}
	for _, c := range counts {
		n[c.ID] = c.N
	}
	for _, s := range all {
		p.Rows = append(p.Rows, listRow{Href: "/admin/series/" + s.ID, Title: s.Title(lang), Slug: s.Slug, StatusKey: badge[s.Status],
			Status: i18n.T(lang, "series.status."+s.Status), Parts: n[s.ID]})
	}
	h.ui.Render(w, http.StatusOK, h.list, p)
}

// badge is the admin status style of a series status.
var badge = map[string]string{"active": "published", "done": "draft", "archived": "archived"}

type partRow struct{ Href, Title, Lang, Kind, Status, StatusKey, Order string }

type editPage struct {
	admin.Shell
	Series   Series
	New      bool
	Statuses []shortadmin.Option
	Sort     string
	Parts    []partRow
	Errors   []string
	Notice   string
}

func (h *Admin) newPage(w http.ResponseWriter, r *http.Request) {
	h.renderEdit(w, r, http.StatusOK, Series{Status: Statuses[0]}, nil)
}

func (h *Admin) editPage(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.renderEdit(w, r, http.StatusOK, s, nil)
}

func (h *Admin) renderEdit(w http.ResponseWriter, r *http.Request, code int, s Series, errs []string) {
	lang := i18n.FromRequest(r)
	title := s.Title(lang)
	if s.ID == "" {
		title = i18n.T(lang, "admin.series.new")
	}
	p := editPage{Shell: h.ui.Shell(r, title, "series"), Series: s, New: s.ID == "", Errors: errs, Sort: strconv.Itoa(s.Sort)}
	if r.URL.Query().Get("saved") == "1" && errs == nil {
		p.Notice = i18n.T(lang, "entry.saved")
	}
	for _, st := range Statuses {
		p.Statuses = append(p.Statuses, shortadmin.Option{Value: st, Label: i18n.T(lang, "series.status."+st), On: s.Status == st})
	}
	if s.ID != "" {
		parts, err := h.repo.Parts(r.Context(), s.ID, "")
		if err != nil {
			h.fail(w, err)
			return
		}
		for _, x := range parts {
			row := partRow{Href: admin.EditorHref(x.Kind, x.ID), Title: x.Title, Lang: x.Lang, Kind: x.Kind, StatusKey: x.Status,
				Status: i18n.T(lang, "admin.status."+x.Status)}
			if x.Order != nil {
				row.Order = "#" + strconv.FormatInt(*x.Order, 10)
			}
			p.Parts = append(p.Parts, row)
		}
	}
	h.ui.Render(w, code, h.edit, p)
}

func (h *Admin) save(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var s Series
	if id := r.PathValue("id"); id != "" {
		cur, err := h.repo.Get(ctx, id)
		if errors.Is(err, content.ErrNotFound) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			h.fail(w, err)
			return
		}
		s = cur
	}
	old := s.Slug
	s.Slug, s.TitleTR, s.TitleEN = r.FormValue("slug"), r.FormValue("title_tr"), r.FormValue("title_en")
	s.SummaryTR, s.SummaryEN, s.Status = r.FormValue("summary_tr"), r.FormValue("summary_en"), r.FormValue("status")
	s.Sort, _ = strconv.Atoi(strings.TrimSpace(r.FormValue("sort")))
	saved, err := h.repo.Save(ctx, s)
	var v content.ValidationError
	if errors.As(err, &v) {
		h.renderEdit(w, r, http.StatusUnprocessableEntity, s, shortadmin.Errors(i18n.FromRequest(r), v))
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.refresh(r, saved.ID, old, saved.Slug)
	http.Redirect(w, r, "/admin/series/"+saved.ID+"?saved=1", http.StatusSeeOther)
}

func (h *Admin) delete(w http.ResponseWriter, r *http.Request) {
	s, err := h.repo.Get(r.Context(), r.PathValue("id"))
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	paths := h.paths(r, s.ID, s.Slug) // before its parts leave it
	if err := h.repo.Delete(r.Context(), s.ID); err != nil {
		h.fail(w, err)
		return
	}
	h.enqueue(r, paths)
	http.Redirect(w, r, "/admin/series?done=deleted", http.StatusSeeOther)
}

// paths are the pages a series shows on: the index and its page in each
// language, and its parts, whose crumb and nav name it.
func (h *Admin) paths(r *http.Request, id string, slugs ...string) []string {
	var out []string
	for _, l := range i18n.Supported {
		for _, slug := range slugs {
			out = append(out, jobs.SeriesPaths(l, slug)...)
		}
	}
	parts, err := h.repo.Parts(r.Context(), id, "")
	if err != nil {
		slog.Error("series parts", "err", err)
	}
	for _, x := range parts {
		if x.Status == content.Published {
			out = append(out, jobs.ItemPath(jobs.Ref{Kind: x.Kind, Lang: x.Lang, Slug: x.Slug}))
		}
	}
	return out
}

func (h *Admin) refresh(r *http.Request, id, oldSlug, slug string) {
	slugs := []string{slug}
	if oldSlug != "" && oldSlug != slug {
		slugs = append(slugs, oldSlug)
	}
	h.enqueue(r, h.paths(r, id, slugs...))
}

func (h *Admin) enqueue(r *http.Request, paths []string) {
	for len(paths) > 0 {
		n := min(len(paths), 50)
		if err := jobs.EnqueuePages(r.Context(), h.jobs, paths[:n]...); err != nil {
			slog.Error("enqueue pages.changed", "err", err) // the dashboard lists failed jobs
		}
		paths = paths[n:]
	}
}

func (h *Admin) fail(w http.ResponseWriter, err error) {
	slog.Error("series admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

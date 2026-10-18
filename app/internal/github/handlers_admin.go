package github

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Admin serves /admin/github (derive: dashboard density): the last sync and
// "run now", and the repos with pin and project link. Pins and links change
// the projects pages, so they queue their re-render.
type Admin struct {
	db   store.DB
	ui   *admin.UI
	auth *auth.Auth
	jobs *jobs.Jobs
	page *template.Template
}

// NewAdmin parses the page.
func NewAdmin(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*Admin, error) {
	t, err := u.Parse("github", "templates/admin/github.html")
	if err != nil {
		return nil, err
	}
	return &Admin{db: db, ui: u, auth: a, jobs: j, page: t}, nil
}

// Register adds the routes; writes go through admin.Guard inside a session.
func (h *Admin) Register(mux *http.ServeMux) {
	w := func(f http.HandlerFunc) http.Handler { return admin.Guard(h.auth.RequireSession(f)) }
	mux.Handle("GET /admin/github", h.auth.RequireSession(http.HandlerFunc(h.show)))
	mux.Handle("POST /admin/github/sync", w(h.runNow))
	mux.Handle("POST /admin/github/repos/{id}/pin", w(h.pin))
	mux.Handle("POST /admin/github/repos/{id}/link", w(h.link))
}

type repoRow struct {
	ID       string `json:"id"`
	FullName string `json:"full_name"`
	URL      string `json:"url"`
	Stars    int    `json:"stars"`
	Language string `json:"language"`
	Pinned   int    `json:"pinned"`
	Gone     int    `json:"gone"`
	Fork     int    `json:"fork"`
	Project  string `json:"project"` // linked project entry id
}

type projectOpt struct {
	ID, Label, Repo string
}

type page struct {
	admin.Shell
	NoToken   bool // the last sync ended for want of a token
	Last      string
	LastState string
	LastError string
	Repos     []repoRow
	Projects  []projectOpt
	Notice    string
}

func (h *Admin) show(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	lang := i18n.FromRequest(r)
	p := page{Shell: h.ui.Shell(r, "GitHub", "github"), Last: i18n.T(lang, "admin.gh.never")}
	switch r.URL.Query().Get("done") {
	case "sync":
		p.Notice = i18n.T(lang, "admin.gh.queued")
	case "saved":
		p.Notice = i18n.T(lang, "admin.gh.saved")
	}
	res, err := h.db.Query(ctx, `SELECT status, error, created_at, finished_at FROM job_runs WHERE type = ? ORDER BY created_at DESC, id DESC LIMIT 1`, TypeSync)
	if err != nil {
		h.fail(w, err)
		return
	}
	last, err := store.Scan[struct {
		Status   string `json:"status"`
		Error    string `json:"error"`
		Created  int64  `json:"created_at"`
		Finished *int64 `json:"finished_at"`
	}](res)
	if err != nil {
		h.fail(w, err)
		return
	}
	if len(last) == 1 {
		at := last[0].Created
		if last[0].Finished != nil {
			at = *last[0].Finished
		}
		p.Last = h.ui.Time(at, "2006-01-02 15:04")
		p.LastState, p.LastError = last[0].Status, last[0].Error
		p.NoToken = last[0].Error == ErrNoToken.Error()
	}
	res, err = h.db.Query(ctx, `SELECT r.id, r.full_name, r.url, r.stars, r.language, r.pinned, r.gone, r.fork,
  coalesce((SELECT p.entry_id FROM projects p WHERE lower(p.github_repo) = lower(r.full_name) LIMIT 1), '') AS project
FROM gh_repos r ORDER BY r.gone, r.pinned DESC, r.pin_sort, r.pushed_at DESC LIMIT 300`)
	if err != nil {
		h.fail(w, err)
		return
	}
	if p.Repos, err = store.Scan[repoRow](res); err != nil {
		h.fail(w, err)
		return
	}
	res, err = h.db.Query(ctx, `SELECT e.id, e.title, e.lang, coalesce(p.github_repo, '') AS repo FROM entries e JOIN projects p ON p.entry_id = e.id
WHERE p.project_type = 'software' ORDER BY e.lang, e.title`)
	if err != nil {
		h.fail(w, err)
		return
	}
	ps, err := store.Scan[struct {
		ID    string `json:"id"`
		Title string `json:"title"`
		Lang  string `json:"lang"`
		Repo  string `json:"repo"`
	}](res)
	if err != nil {
		h.fail(w, err)
		return
	}
	for _, x := range ps {
		p.Projects = append(p.Projects, projectOpt{ID: x.ID, Label: x.Title + " · " + x.Lang, Repo: x.Repo})
	}
	h.ui.Render(w, http.StatusOK, h.page, p)
}

func (h *Admin) runNow(w http.ResponseWriter, r *http.Request) {
	if _, err := h.jobs.Enqueue(r.Context(), TypeSync, struct{}{}); err != nil {
		h.fail(w, err)
		return
	}
	http.Redirect(w, r, "/admin/github?done=sync", http.StatusSeeOther)
}

func (h *Admin) repo(r *http.Request) (repoRow, bool) {
	res, err := h.db.Query(r.Context(), `SELECT id, full_name, url, stars, language, pinned, gone, fork, '' AS project FROM gh_repos WHERE id = ?`, r.PathValue("id"))
	if err != nil {
		return repoRow{}, false
	}
	rows, _ := store.Scan[repoRow](res)
	if len(rows) == 0 {
		return repoRow{}, false
	}
	return rows[0], true
}

// pin toggles a repo on the projects page; a new pin goes last.
func (h *Admin) pin(w http.ResponseWriter, r *http.Request) {
	rp, ok := h.repo(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	on := r.FormValue("pinned") == "1"
	if _, err := h.db.Query(r.Context(), `UPDATE gh_repos SET pinned = ?, pin_sort = CASE WHEN ? = 1 THEN (SELECT coalesce(max(pin_sort), 0) + 1 FROM gh_repos) ELSE pin_sort END WHERE id = ?`,
		b(on), b(on), rp.ID); err != nil {
		h.fail(w, err)
		return
	}
	h.refresh(r)
	http.Redirect(w, r, "/admin/github?done=saved", http.StatusSeeOther)
}

// link points one project's github_repo at the repo; "" unlinks the repo
// from every project. The project's own pages re-render via entry.changed.
func (h *Admin) link(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rp, ok := h.repo(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	res, err := h.db.Query(ctx, `SELECT entry_id FROM projects WHERE lower(github_repo) = lower(?)`, rp.FullName)
	if err != nil {
		h.fail(w, err)
		return
	}
	before, _ := store.Scan[struct {
		ID string `json:"entry_id"`
	}](res)
	touched := map[string]*jobs.Ref{}
	for _, x := range before {
		touched[x.ID], _ = jobs.RefOf(ctx, h.db, x.ID)
	}
	stmts := []store.Stmt{{SQL: `UPDATE projects SET github_repo = NULL WHERE lower(github_repo) = lower(?)`, Args: []any{rp.FullName}}}
	if id := strings.TrimSpace(r.FormValue("project")); id != "" {
		if _, ok := touched[id]; !ok {
			touched[id], _ = jobs.RefOf(ctx, h.db, id)
		}
		stmts = append(stmts, store.Stmt{SQL: `UPDATE projects SET github_repo = ? WHERE entry_id = ?`, Args: []any{rp.FullName, id}})
	}
	if _, err := h.db.Batch(ctx, stmts...); err != nil {
		h.fail(w, err)
		return
	}
	for id, prev := range touched {
		if err := jobs.Enqueue(ctx, h.jobs, id, prev); err != nil {
			slog.Error("enqueue entry.changed", "entry", id, "err", err)
		}
	}
	http.Redirect(w, r, "/admin/github?done=saved", http.StatusSeeOther)
}

// refresh queues the projects lists, which show the pins.
func (h *Admin) refresh(r *http.Request) {
	var paths []string
	for _, l := range i18n.Supported {
		paths = append(paths, "/"+l+"/projects")
	}
	if err := jobs.EnqueuePages(r.Context(), h.jobs, paths...); err != nil {
		slog.Error("enqueue pages.changed", "err", err)
	}
}

func (h *Admin) fail(w http.ResponseWriter, err error) {
	slog.Error("github admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

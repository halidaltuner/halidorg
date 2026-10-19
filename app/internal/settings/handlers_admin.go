package settings

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Admin serves /admin/settings (derive: dashboard density): the homelab stat
// tiles and the 500px profile of the photos page; a save re-renders the
// pages that show them.
type Admin struct {
	db   store.DB
	ui   *admin.UI
	auth *auth.Auth
	jobs *jobs.Jobs
	page *template.Template
}

// NewAdmin parses the settings page.
func NewAdmin(db store.DB, u *admin.UI, a *auth.Auth, j *jobs.Jobs) (*Admin, error) {
	t, err := u.Parse("settings", "templates/admin/settings.html")
	if err != nil {
		return nil, err
	}
	return &Admin{db: db, ui: u, auth: a, jobs: j, page: t}, nil
}

// Register adds the routes. Saving is a write: admin.Guard inside a session;
// nothing here is destructive, so no recent sign-in.
func (h *Admin) Register(mux *http.ServeMux) {
	mux.Handle("GET /admin/settings", h.auth.RequireSession(http.HandlerFunc(h.show)))
	mux.Handle("POST /admin/settings/homelab", admin.Guard(h.auth.RequireSession(http.HandlerFunc(h.saveHomelab))))
	mux.Handle("POST /admin/settings/photos", admin.Guard(h.auth.RequireSession(http.HandlerFunc(h.savePhotos))))
	mux.Handle("POST /admin/settings/search", admin.Guard(h.auth.RequireSession(http.HandlerFunc(h.rebuildSearch))))
}

// rebuildSearch recreates the search index from the main database in the
// background (jobs.TypeSearchRebuild): the first job empties it, so search
// finds less until the last page is indexed, a minute or so.
func (h *Admin) rebuildSearch(w http.ResponseWriter, r *http.Request) {
	if err := jobs.EnqueueSearchRebuild(r.Context(), h.jobs); err != nil {
		h.fail(w, err)
		return
	}
	http.Redirect(w, r, "/admin/settings?saved=search", http.StatusSeeOther)
}

type page struct {
	admin.Shell
	Homelab Homelab
	Photos  Photos
	Notice  string
	Error   string
}

func (h *Admin) show(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	p := page{Shell: h.ui.Shell(r, i18n.T(lang, "admin.nav.settings"), "settings")}
	if err := Get(r.Context(), h.db, HomelabKey, &p.Homelab); err != nil {
		h.fail(w, err)
		return
	}
	if err := Get(r.Context(), h.db, PhotosKey, &p.Photos); err != nil {
		h.fail(w, err)
		return
	}
	switch r.URL.Query().Get("saved") {
	case "":
	case "search":
		p.Notice = i18n.T(lang, "settings.search_started")
	default:
		p.Notice = i18n.T(lang, "settings.saved")
	}
	h.ui.Render(w, http.StatusOK, h.page, p)
}

func (h *Admin) saveHomelab(w http.ResponseWriter, r *http.Request) {
	v := Homelab{Nodes: strings.TrimSpace(r.FormValue("nodes")), Idle: strings.TrimSpace(r.FormValue("idle")),
		Uptime: strings.TrimSpace(r.FormValue("uptime"))}
	for _, s := range []string{v.Nodes, v.Idle, v.Uptime} {
		if utf8.RuneCountInString(s) > 16 {
			lang := i18n.FromRequest(r)
			h.ui.Render(w, http.StatusUnprocessableEntity, h.page, page{Shell: h.ui.Shell(r, i18n.T(lang, "admin.nav.settings"), "settings"),
				Homelab: v, Error: i18n.T(lang, "settings.too_long")})
			return
		}
	}
	if err := Set(r.Context(), h.db, HomelabKey, v); err != nil {
		h.fail(w, err)
		return
	}
	var paths []string
	for _, l := range i18n.Supported {
		paths = append(paths, "/"+l+"/homelab")
	}
	if err := jobs.EnqueuePages(r.Context(), h.jobs, paths...); err != nil {
		slog.Error("enqueue pages.changed", "err", err) // the dashboard lists failed jobs to retry
	}
	http.Redirect(w, r, "/admin/settings?saved=homelab", http.StatusSeeOther)
}

func (h *Admin) savePhotos(w http.ResponseWriter, r *http.Request) {
	v := Photos{Profile: strings.TrimSpace(r.FormValue("profile_500px"))}
	if v.Profile != "" && !strings.HasPrefix(v.Profile, "https://500px.com/") {
		lang := i18n.FromRequest(r)
		p := page{Shell: h.ui.Shell(r, i18n.T(lang, "admin.nav.settings"), "settings"), Photos: v, Error: i18n.T(lang, "settings.bad_500px")}
		if err := Get(r.Context(), h.db, HomelabKey, &p.Homelab); err != nil {
			h.fail(w, err)
			return
		}
		h.ui.Render(w, http.StatusUnprocessableEntity, h.page, p)
		return
	}
	if err := Set(r.Context(), h.db, PhotosKey, v); err != nil {
		h.fail(w, err)
		return
	}
	var paths []string
	for _, l := range i18n.Supported {
		paths = append(paths, "/"+l+"/photos")
	}
	if err := jobs.EnqueuePages(r.Context(), h.jobs, paths...); err != nil {
		slog.Error("enqueue pages.changed", "err", err)
	}
	http.Redirect(w, r, "/admin/settings?saved=photos", http.StatusSeeOther)
}

func (h *Admin) fail(w http.ResponseWriter, err error) {
	slog.Error("settings admin", "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

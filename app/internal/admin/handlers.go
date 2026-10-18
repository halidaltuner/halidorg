// Package admin serves /admin: sign-in, the shell every admin page shares
// (UI), the dashboard and admin search. Kind editors live with their kind
// (internal/content/<kind>/handlers_admin.go) and build on UI.
package admin

import (
	"bytes"
	"errors"
	"html/template"
	"log/slog"
	"net/http"

	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/ui"
)

// Handlers holds the admin templates, the auth flow and the stores the
// dashboard reads.
type Handlers struct {
	auth     *auth.Auth
	ui       *UI
	db       store.DB
	searchDB store.DB
	jobs     *jobs.Jobs
	env      string // local, staging or prod; prod hides the env badge
	deploy   string // short commit, empty outside CI deploys
	login    *template.Template
	dash     *template.Template
	searchT  *template.Template
}

// New parses the admin templates. APP_ENV and DEPLOY_SHA come from the Worker.
func New(a *auth.Auth, u *UI, db, searchDB store.DB, j *jobs.Jobs) (*Handlers, error) {
	login, err := ui.Parse("login", "templates/admin/base.html", "templates/admin/login.html")
	if err != nil {
		return nil, err
	}
	dash, err := u.Parse("dashboard", "templates/admin/dashboard.html")
	if err != nil {
		return nil, err
	}
	searchT, err := u.Parse("search", "templates/admin/dashboard.html", "templates/admin/search.html")
	if err != nil {
		return nil, err
	}
	return &Handlers{auth: a, ui: u, db: db, searchDB: searchDB, jobs: j, env: ui.Env(), deploy: ui.Deploy(),
		login: login, dash: dash, searchT: searchT}, nil
}

// Register adds the admin routes to mux. Every POST goes through Guard.
func (h *Handlers) Register(mux *http.ServeMux) {
	session := func(f http.HandlerFunc) http.Handler { return h.auth.RequireSession(f) }
	mux.HandleFunc("GET /admin/login", h.loginPage)
	mux.Handle("POST /admin/login", Guard(http.HandlerFunc(h.startLogin)))
	mux.HandleFunc("GET /admin/auth/callback", h.auth.Callback)
	mux.Handle("GET /admin/reauth", session(h.startReauth))
	mux.Handle("POST /admin/logout", Guard(session(h.auth.Logout)))
	mux.Handle("GET /admin", session(h.dashboard))
	mux.Handle("GET /admin/search", session(h.search))
	mux.Handle("POST /admin/jobs/{id}/retry", Guard(session(h.retryJob)))
	mux.Handle("POST /admin/redirects", Guard(session(h.addRedirect)))
}

// cop rejects cross-site form posts (CSRF): browsers send Sec-Fetch-Site,
// and older ones an Origin that must match the host.
var cop = http.NewCrossOriginProtection()

// Guard wraps every state-changing admin route.
func Guard(next http.Handler) http.Handler { return cop.Handler(next) }

type loginPage struct {
	Lang, Env, Deploy, Next string
	Dark                    bool
	Flash                   *auth.Flash
	Switch                  []i18n.Option
}

func (h *Handlers) loginPage(w http.ResponseWriter, r *http.Request) {
	h.renderLogin(w, r, http.StatusOK, h.auth.TakeFlash(w, r))
}

func (h *Handlers) renderLogin(w http.ResponseWriter, r *http.Request, status int, flash *auth.Flash) {
	lang := i18n.FromRequest(r)
	h.render(w, status, h.login, loginPage{
		Lang: lang, Env: h.env, Deploy: h.deploy,
		Next: auth.SafeNext(r.URL.Query().Get("next")), Dark: true, Flash: flash,
		Switch: i18n.Switcher(lang, samePage(r)),
	})
}

func (h *Handlers) startLogin(w http.ResponseWriter, r *http.Request) {
	h.start(w, r, r.FormValue("next"), nil)
}

func (h *Handlers) startReauth(w http.ResponseWriter, r *http.Request) {
	h.start(w, r, r.URL.Query().Get("next"), auth.SessionFrom(r.Context()))
}

func (h *Handlers) start(w http.ResponseWriter, r *http.Request, next string, sess *auth.Session) {
	err := h.auth.Start(w, r, next, sess)
	if err == nil {
		return
	}
	if errors.Is(err, auth.ErrNotConfigured) {
		slog.Error("admin sign-in is not configured", "err", err)
	} else {
		slog.Error("start sign-in", "err", err)
	}
	h.renderLogin(w, r, http.StatusServiceUnavailable, &auth.Flash{Kind: "failed"})
}

// samePage keeps admin on the page it is on: admin URLs carry no language.
// A POST (the failed-login render) has no page to repeat, so it lands on login.
func samePage(r *http.Request) func(string) string {
	target := r.URL.RequestURI()
	if r.Method != http.MethodGet {
		target = auth.LoginPath
	}
	return func(string) string { return target }
}

// render buffers so a template error becomes a clean 500, not half a page.
func (h *Handlers) render(w http.ResponseWriter, status int, t *template.Template, data any) {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, "admin-base", data); err != nil {
		slog.Error("render admin", "template", t.Name(), "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

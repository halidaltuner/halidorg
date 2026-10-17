// Package admin serves /admin: sign-in screens now, the editor from Phase 1.
package admin

import (
	"bytes"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"os"

	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/web"
)

// Handlers holds the admin templates and the auth flow.
type Handlers struct {
	auth   *auth.Auth
	env    string // local, staging or prod; prod hides the env badge
	deploy string // short commit, empty outside CI deploys
	login  *template.Template
	home   *template.Template
}

// New parses the admin templates. APP_ENV and DEPLOY_SHA come from the Worker.
func New(a *auth.Auth) (*Handlers, error) {
	funcs := template.FuncMap{"tr": i18n.T}
	parse := func(page string) (*template.Template, error) {
		return template.New(page).Funcs(funcs).ParseFS(web.Templates,
			"templates/admin/base.html", "templates/partials/lang_switch.html", "templates/admin/"+page+".html")
	}
	login, err := parse("login")
	if err != nil {
		return nil, err
	}
	home, err := parse("home")
	if err != nil {
		return nil, err
	}
	deploy := os.Getenv("DEPLOY_SHA")
	if len(deploy) > 7 {
		deploy = deploy[:7]
	}
	env := os.Getenv("APP_ENV")
	if env == "" {
		env = "local"
	}
	return &Handlers{auth: a, env: env, deploy: deploy, login: login, home: home}, nil
}

// Register adds the admin routes to mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /admin/login", h.loginPage)
	mux.HandleFunc("POST /admin/login", h.startLogin)
	mux.HandleFunc("GET /admin/auth/callback", h.auth.Callback)
	mux.Handle("GET /admin/reauth", h.auth.RequireSession(http.HandlerFunc(h.startReauth)))
	mux.Handle("POST /admin/logout", h.auth.RequireSession(http.HandlerFunc(h.auth.Logout)))
	mux.Handle("GET /admin", h.auth.RequireSession(http.HandlerFunc(h.homePage)))
}

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

type homePage struct {
	Lang, Email string
	Dark        bool
	Switch      []i18n.Option
}

func (h *Handlers) homePage(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	h.render(w, http.StatusOK, h.home, homePage{
		Lang:   lang,
		Email:  auth.SessionFrom(r.Context()).Email,
		Switch: i18n.Switcher(lang, samePage(r)),
	})
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

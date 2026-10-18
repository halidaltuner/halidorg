package admin

import (
	"bytes"
	"context"
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"
	_ "time/tzdata" // admin times are shown in ADMIN_TZ; the image has no zoneinfo

	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/ui"
)

// UI is the admin shell (design/Admin Dashboard.dc.html): the 208px sidebar
// and the 52px top bar, shared by every admin page. Content kinds build their
// admin pages on it (internal/content/<kind>/handlers_admin.go).
type UI struct {
	db  store.DB
	env string
	loc *time.Location
}

// NewUI reads APP_ENV, DEPLOY_SHA and ADMIN_TZ (default Europe/Istanbul).
func NewUI(db store.DB) *UI {
	loc, err := time.LoadLocation(envOr("ADMIN_TZ", "Europe/Istanbul"))
	if err != nil {
		slog.Error("ADMIN_TZ", "err", err)
		loc = time.UTC
	}
	return &UI{db: db, env: ui.Env(), loc: loc}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Parse parses an admin page: admin/base.html, the shell, and files.
func (u *UI) Parse(name string, files ...string) (*template.Template, error) {
	all := append([]string{"templates/admin/base.html", "templates/admin/shell.html"}, files...)
	return ui.Parse(name, all...)
}

// ParseBare parses a full-page admin screen without the shell (the editor).
func (u *UI) ParseBare(name string, files ...string) (*template.Template, error) {
	return ui.Parse(name, append([]string{"templates/admin/base.html"}, files...)...)
}

// SideItem is one sidebar row. Off rows are sections that do not exist yet:
// shown greyed, not linked (Phase 1.6 prompt).
type SideItem struct {
	Label, Href, Count string
	Sub, Active, Off   bool
}

// Shell is what shell.html reads.
type Shell struct {
	Lang   string
	Title  string
	Env    string // empty in prod: no badge
	Deploy string
	Email  string
	Side   []SideItem
	Query  string // search box value
	Switch []i18n.Option
	Dark   bool
}

// sections in sidebar order. kind rows are indented under Content.
var sections = []struct {
	key, href, kind string
	sub, on         bool
}{
	{"dashboard", "/admin", "", false, true},
	{"content", "", "", false, true},
	{"posts", "/admin/posts", "post", true, true},
	{"notes", "/admin/notes", "note", true, true},
	{"snippets", "/admin/snippets", "snippet", true, true},
	{"links", "/admin/links", "link", true, true},
	{"favs", "/admin/favs", "fav", true, true},
	{"projects", "/admin/projects", "project", true, true},
	{"pages", "/admin/pages", "page", true, true},
	{"certificates", "/admin/certificates", "", true, true},
	{"categories", "/admin/categories", "", false, true},
	{"tags", "/admin/tags", "", false, true},
	{"media", "/admin/media", "", false, true},
	{"radar", "", "", false, false},
	{"github", "/admin/github", "", false, true},
	{"jobs", "", "", false, false},
	{"redirects", "", "", false, false},
	{"import", "", "", false, false},
	{"settings", "/admin/settings", "", false, true},
}

// Shell builds the shell for r. active is the sidebar key to mark.
func (u *UI) Shell(r *http.Request, title, active string) Shell {
	lang := i18n.FromRequest(r)
	counts := u.counts(r.Context())
	side := make([]SideItem, 0, len(sections))
	for _, s := range sections {
		it := SideItem{Label: i18n.T(lang, "admin.nav."+s.key), Href: s.href, Sub: s.sub, Active: s.key == active, Off: !s.on}
		if s.kind != "" {
			if n, ok := counts[s.kind]; ok {
				it.Count = strconv.Itoa(n)
			}
		}
		if s.key == "jobs" && counts["jobs"] > 0 {
			it.Count = strconv.Itoa(counts["jobs"])
		}
		side = append(side, it)
	}
	env := u.env
	if env == "prod" {
		env = ""
	}
	email := ""
	if s := auth.SessionFrom(r.Context()); s != nil {
		email = s.Email
	}
	return Shell{
		Lang: lang, Title: title, Env: env, Deploy: ui.Deploy(), Email: email, Side: side,
		Switch: i18n.Switcher(lang, samePage(r)),
	}
}

// counts are the sidebar numbers: entries per kind and failed jobs. A
// failing count query leaves the numbers out rather than failing the page.
func (u *UI) counts(ctx context.Context) map[string]int {
	out := map[string]int{}
	res, err := u.db.Query(ctx, `SELECT kind, COUNT(*) AS n FROM entries GROUP BY kind
UNION ALL SELECT 'jobs', COUNT(*) FROM job_runs WHERE status IN ('failed', 'dead')`)
	if err != nil {
		slog.Error("admin counts", "err", err)
		return out
	}
	rows, _ := store.Scan[struct {
		Kind string `json:"kind"`
		N    int    `json:"n"`
	}](res)
	for _, r := range rows {
		out[r.Kind] = r.N
	}
	return out
}

// Time formats unix seconds in the admin time zone.
func (u *UI) Time(sec int64, layout string) string {
	return time.Unix(sec, 0).In(u.loc).Format(layout)
}

// Ago is a short relative time: "now", "5m", "2h", "3d", then the date.
func Ago(lang string, sec, now int64) string {
	d := now - sec
	switch {
	case d < 60:
		return i18n.T(lang, "admin.ago.now")
	case d < 3600:
		return strconv.FormatInt(d/60, 10) + i18n.T(lang, "admin.ago.m")
	case d < 86400:
		return strconv.FormatInt(d/3600, 10) + i18n.T(lang, "admin.ago.h")
	case d < 30*86400:
		return strconv.FormatInt(d/86400, 10) + i18n.T(lang, "admin.ago.d")
	}
	return time.Unix(sec, 0).UTC().Format("2006-01-02")
}

// Render writes a full admin page (template "admin-base").
func (u *UI) Render(w http.ResponseWriter, status int, t *template.Template, data any) {
	u.write(w, status, t, "admin-base", data)
}

// Fragment writes one named template, for htmx swaps.
func (u *UI) Fragment(w http.ResponseWriter, status int, t *template.Template, name string, data any) {
	u.write(w, status, t, name, data)
}

// write buffers so a template error becomes a clean 500, not half a page.
func (u *UI) write(w http.ResponseWriter, status int, t *template.Template, name string, data any) {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		slog.Error("render admin", "template", t.Name(), "block", name, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(buf.Bytes())
}

// IsHTMX reports whether r came from htmx (and wants a fragment back).
func IsHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

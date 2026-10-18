// Package ui builds the view model shared by every page layout: header, nav,
// footer, and the template functions the partials rely on.
package ui

import (
	"errors"
	"html/template"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/web"
)

// NavItem is one entry in the header nav or the more menu.
type NavItem struct {
	Slug, Label, Href string
	Active            bool
}

type section struct {
	slug    string
	enabled bool
}

// nav and more follow design/Header.dc.html. A section is enabled once its
// public page exists; disabled ones are not rendered at all.
var (
	nav = []section{
		{"posts", true}, {"projects", false}, {"homelab", false}, {"presentations", false},
		{"photos", false}, {"radar", false}, {"now", false}, {"about", false},
	}
	more = []section{
		{"notes", false}, {"snippets", false}, {"links", false}, {"favs", false}, {"certificates", false},
	}
)

// Chrome is what header.html and footer.html read.
type Chrome struct {
	Lang   string
	Nav    []NavItem
	More   []NavItem
	Switch []i18n.Option
	Footer Footer
	// Search loads the command palette (palette.js), which shows the ⌘K
	// button; without JS the button stays hidden.
	Search bool
}

// Footer is the dark footer band. Empty fields are left out of the markup.
type Footer struct {
	Tagline string
	Links   []Link
	Deploy  string
}

// Link is a footer link. Muted renders it in the secondary color (admin).
type Link struct {
	Label, Href string
	Muted       bool
}

// NewChrome builds the chrome for a page in lang. active is the nav slug to
// mark; all renders disabled sections too (styleguide only). switchTo maps a
// language to the page the tr/en toggle lands on.
func NewChrome(lang, active string, all bool, switchTo func(string) string) Chrome {
	return Chrome{
		Lang:   lang,
		Nav:    items(nav, lang, active, all),
		More:   items(more, lang, active, all),
		Switch: i18n.Switcher(lang, switchTo),
		Search: true,
		Footer: Footer{
			Links: []Link{
				{Label: "rss", Href: "/" + lang + "/feed.xml"},
				{Label: "json feed", Href: "/" + lang + "/feed.json"},
				{Label: i18n.T(lang, "footer.admin"), Href: "/admin", Muted: true},
			},
			Deploy: Deploy(),
		},
	}
}

func items(list []section, lang, active string, all bool) []NavItem {
	out := make([]NavItem, 0, len(list))
	for _, s := range list {
		if !s.enabled && !all {
			continue
		}
		out = append(out, NavItem{
			Slug:   s.slug,
			Label:  i18n.T(lang, "nav."+s.slug),
			Href:   "/" + lang + "/" + s.slug,
			Active: s.slug == active,
		})
	}
	return out
}

// deploySHA is the commit serving now. It starts from DEPLOY_SHA, which the
// Durable Object sets when the container starts, and follows the Worker's
// X-Deploy-Sha header (DeployHeader), which is current on every request.
var deploySHA atomic.Value

var shaRE = regexp.MustCompile(`^[0-9a-f]{7,40}$`)

func init() { deploySHA.Store("") }

// DeploySHA is the full commit CI deployed, or empty outside CI deploys:
// the Worker's last header, else the env var the container started with.
func DeploySHA() string {
	if sha := deploySHA.Load().(string); sha != "" {
		return sha
	}
	return os.Getenv("DEPLOY_SHA")
}

// Deploy is the short commit for display.
func Deploy() string {
	sha := DeploySHA()
	if len(sha) > 7 {
		sha = sha[:7]
	}
	return sha
}

// DeployHeader wraps the app: the edge Worker sends its DEPLOY_SHA on every
// request it forwards (and overwrites any client value), so pages and KV
// metadata name the deploy that is live, not the one the container started with.
func DeployHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if sha := r.Header.Get("X-Deploy-Sha"); shaRE.MatchString(sha) && sha != DeploySHA() {
			deploySHA.Store(sha)
		}
		next.ServeHTTP(w, r)
	})
}

// Env is local, staging or prod, from the Worker's APP_ENV.
func Env() string {
	if env := os.Getenv("APP_ENV"); env != "" {
		return env
	}
	return "local"
}

// Funcs is the FuncMap every template set parses with.
func Funcs() template.FuncMap {
	return template.FuncMap{
		"tr": i18n.T, "dict": dict, "date": Date, "minutes": Minutes,
		"langs": func() []string { return i18n.Supported },
		"add":   func(a, b int) int { return a + b },
		"sub":   func(a, b int) int { return a - b },
	}
}

// Date formats unix seconds (int64 or *int64) as 2006-01-02, UTC.
func Date(v any) string {
	switch t := v.(type) {
	case int64:
		return time.Unix(t, 0).UTC().Format("2006-01-02")
	case *int64:
		if t != nil {
			return time.Unix(*t, 0).UTC().Format("2006-01-02")
		}
	}
	return ""
}

// MinutesShort is the compact reading time on cards and rows ("12 min").
func MinutesShort(lang string, n int) string {
	return strings.Replace(i18n.T(lang, "post.min_short"), "%d", strconv.Itoa(max(n, 1)), 1)
}

// Minutes is the reading time label ("12 min read", "12 dk okuma").
func Minutes(lang string, n int) string {
	return strings.Replace(i18n.T(lang, "post.minutes"), "%d", strconv.Itoa(max(n, 1)), 1)
}

// dict lets a template pass named arguments to a partial:
// {{template "cover" dict "tone" "cyan" "radius" "8px"}}.
func dict(kv ...any) (map[string]any, error) {
	if len(kv)%2 != 0 {
		return nil, errors.New("dict: odd number of arguments")
	}
	m := make(map[string]any, len(kv)/2)
	for i := 0; i < len(kv); i += 2 {
		k, ok := kv[i].(string)
		if !ok {
			return nil, errors.New("dict: keys must be strings")
		}
		m[k] = kv[i+1]
	}
	return m, nil
}

// Partials are the shared component templates every page set includes.
var Partials = []string{
	"templates/partials/lang_switch.html",
	"templates/partials/header.html",
	"templates/partials/footer.html",
	"templates/partials/cover.html",
	"templates/partials/chips.html",
}

// Parse parses files from web.Templates together with Partials and Funcs.
func Parse(name string, files ...string) (*template.Template, error) {
	return template.New(name).Funcs(Funcs()).ParseFS(web.Templates, append(files, Partials...)...)
}

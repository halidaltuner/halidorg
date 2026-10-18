// Package ui builds the view model shared by every page layout: header, nav,
// footer, and the template functions the partials rely on.
package ui

import (
	"errors"
	"html/template"
	"os"

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
	// Search shows the ⌘K button; off until the command palette ships.
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
		Footer: Footer{
			Links:  []Link{{Label: i18n.T(lang, "footer.admin"), Href: "/admin", Muted: true}},
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
			Href:   "/" + lang + "/" + s.slug + "/",
			Active: s.slug == active,
		})
	}
	return out
}

// Deploy is the short commit CI deployed, or empty outside CI deploys.
func Deploy() string {
	sha := os.Getenv("DEPLOY_SHA")
	if len(sha) > 7 {
		sha = sha[:7]
	}
	return sha
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
	return template.FuncMap{"tr": i18n.T, "dict": dict}
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

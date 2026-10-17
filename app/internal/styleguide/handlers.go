// Package styleguide serves /_styleguide: every token and shared partial on one
// page, for comparing against design/*.dc.html. It is a 404 in prod.
package styleguide

import (
	"bytes"
	"html/template"
	"log/slog"
	"net/http"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/ui"
)

// Handlers renders the styleguide.
type Handlers struct {
	env  string
	page *template.Template
}

// New parses the styleguide template.
func New() (*Handlers, error) {
	page, err := ui.Parse("styleguide", "templates/styleguide/index.html")
	if err != nil {
		return nil, err
	}
	return &Handlers{env: ui.Env(), page: page}, nil
}

// Register adds GET /_styleguide.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /_styleguide", h.show)
}

// Fixture copy shown only here; real pages take these from content and settings.
var (
	tones      = []string{"cyan", "indigo", "amber", "green", "orange", "rose", "violet", "navy"}
	sections   = []string{"posts", "projects", "homelab", "presentations", "photos", "radar", "now", "about"}
	fixtureFtr = ui.Footer{
		Tagline: "styleguide tagline",
		Links: []ui.Link{
			{Label: "rss", Href: "#"}, {Label: "json feed", Href: "#"}, {Label: "github", Href: "#"},
			{Label: "admin", Href: "/admin", Muted: true},
		},
		Deploy: "0000000",
	}
)

type data struct {
	ui.Chrome
	Headers []ui.Chrome
	Tones   []string
	Scale   []int
	Space   []int
	Callout template.HTML
}

func (h *Handlers) show(w http.ResponseWriter, r *http.Request) {
	if h.env == "prod" {
		http.NotFound(w, r)
		return
	}
	lang := i18n.FromRequest(r)
	here := func(string) string { return "/_styleguide" }
	chrome := ui.NewChrome(lang, "posts", true, here)
	chrome.Footer = fixtureFtr
	headers := make([]ui.Chrome, 0, len(sections))
	for _, s := range sections {
		c := ui.NewChrome(lang, s, true, here)
		c.Search = true
		headers = append(headers, c)
	}
	var buf bytes.Buffer
	err := h.page.ExecuteTemplate(&buf, "styleguide", data{
		Chrome:  chrome,
		Headers: headers,
		Tones:   tones,
		Scale:   []int{11, 12, 13, 14, 15, 16, 18, 20, 22, 24, 28, 36, 40, 48, 56},
		Space:   []int{4, 8, 12, 16, 20, 24, 32, 40, 48, 56, 64, 72, 96},
		Callout: template.HTML("Explicit saves always create a new revision."),
	})
	if err != nil {
		slog.Error("render styleguide", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Robots-Tag", "noindex")
	_, _ = w.Write(buf.Bytes())
}

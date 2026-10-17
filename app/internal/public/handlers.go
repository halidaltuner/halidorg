// Package public serves the public site.
package public

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"html/template"
	"log/slog"
	"net/http"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/web"
)

type alternate struct {
	Lang string
	Href string
}

type page struct {
	Lang       string
	Alternates []alternate
}

// Handlers holds parsed templates for public pages.
type Handlers struct {
	hello *template.Template
}

// New parses the public templates. It fails at startup rather than per request.
func New() (*Handlers, error) {
	funcs := template.FuncMap{"tr": i18n.T}
	hello, err := template.New("hello").Funcs(funcs).ParseFS(web.Templates,
		"templates/partials/base.html", "templates/public/hello.html")
	if err != nil {
		return nil, err
	}
	return &Handlers{hello: hello}, nil
}

// Register adds the public routes to mux.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.root)
	mux.HandleFunc("GET /{lang}/{$}", h.home)
}

func (h *Handlers) root(w http.ResponseWriter, r *http.Request) {
	lang := i18n.Negotiate(r.Header.Get("Accept-Language"))
	// Varies per visitor, so the edge must not cache it under the path key.
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Accept-Language")
	http.Redirect(w, r, "/"+lang+"/", http.StatusFound)
}

func (h *Handlers) home(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	if !i18n.IsSupported(lang) {
		http.NotFound(w, r)
		return
	}
	alts := make([]alternate, 0, len(i18n.Supported))
	for _, l := range i18n.Supported {
		alts = append(alts, alternate{Lang: l, Href: "/" + l + "/"})
	}
	var buf bytes.Buffer
	if err := h.hello.ExecuteTemplate(&buf, "base", page{Lang: lang, Alternates: alts}); err != nil {
		slog.Error("render hello", "lang", lang, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeCacheable(w, r, buf.Bytes())
}

// writeCacheable sends HTML the edge Worker may store in KV. `public` is the
// opt-in signal the Worker checks; max-age=0 keeps browsers revalidating so a
// purge is visible on the next request.
func writeCacheable(w http.ResponseWriter, r *http.Request, body []byte) {
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}

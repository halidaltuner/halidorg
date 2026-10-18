// Package public serves the public site: home, posts, post lists, tag
// pages, the language switch and the styled 404/410 pages.
package public

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/fav"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	pages "halid.org/engine/internal/content/page"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/render/og"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/ui"
)

type alternate struct {
	Lang string
	Href string
}

// Head is what partials/base.html puts in <head>.
type Head struct {
	Title       string
	Description string
	Canonical   string
	Alternates  []alternate
	Noindex     bool
	OGType      string
	Image       string // absolute og:image URL (render/og)
	Scripts     []string
	Feeds       []feedLink // the page's own feeds, after the language feed
}

type feedLink struct {
	Title, Href string
}

type page struct {
	ui.Chrome
	Head Head
}

// Handlers holds parsed templates and the post repository.
type Handlers struct {
	db          store.DB
	posts       *post.Repo
	snippetRepo *snippet.Repo
	noteRepo    *note.Repo
	linkRepo    *link.Repo
	favRepo     *fav.Repo
	projectRepo *project.Repo
	logRepo     *buildlog.Repo
	pageRepo    *pages.Repo
	origin      string
	tmpl        map[string]*template.Template
}

// New parses the public templates. It fails at startup rather than per request.
func New(db store.DB) (*Handlers, error) {
	h := &Handlers{db: db, posts: post.NewRepo(db), snippetRepo: snippet.NewRepo(db), noteRepo: note.NewRepo(db),
		linkRepo: link.NewRepo(db), favRepo: fav.NewRepo(db), projectRepo: project.NewRepo(db), logRepo: buildlog.NewRepo(db),
		pageRepo: pages.NewRepo(db), origin: origin(), tmpl: map[string]*template.Template{}}
	for name, file := range map[string]string{
		"home":          "templates/public/home.html",
		"post":          "templates/post/show.html",
		"list":          "templates/post/list.html",
		"error":         "templates/public/error.html",
		"snippets":      "templates/snippet/index.html",
		"notes":         "templates/note/list.html",
		"note":          "templates/note/show.html",
		"links":         "templates/link/index.html",
		"favs":          "templates/fav/index.html",
		"awesome":       "templates/link/awesome.html",
		"awesome-index": "templates/link/awesome_index.html",
		"projects":      "templates/project/index.html",
		"project":       "templates/project/show.html",
		"buildlog":      "templates/buildlog/show.html",
		"homelab":       "templates/project/homelab.html",
		"stream":        "templates/public/stream.html",
		"github":        "templates/github/index.html",
		"about":         "templates/page/about.html",
		"now":           "templates/page/now.html",
		"now-archive":   "templates/page/now_archive.html",
		"certificates":  "templates/certificate/index.html",
		"photos":        "templates/photo/index.html",
	} {
		t, err := ui.Parse(name, "templates/partials/base.html", "templates/post/card.html", "templates/note/card.html", file)
		if err != nil {
			return nil, err
		}
		h.tmpl[name] = t
	}
	return h, nil
}

func origin() string {
	if o := strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/"); o != "" {
		return o
	}
	return "http://localhost:8787"
}

// Register adds the public routes. Language routes are literal per language
// (CLAUDE.md): a /{lang}/posts pattern would overlap /lang/{lang}.
func (h *Handlers) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", h.root)
	mux.HandleFunc("GET /lang/{lang}", setLang)
	for _, l := range i18n.Supported {
		lang := l
		mux.HandleFunc("GET /"+lang+"/{$}", func(w http.ResponseWriter, r *http.Request) { h.home(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/posts", func(w http.ResponseWriter, r *http.Request) { h.list(w, r, lang, "") })
		mux.HandleFunc("GET /"+lang+"/posts/{slug}", func(w http.ResponseWriter, r *http.Request) { h.post(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/tags/{tag}", func(w http.ResponseWriter, r *http.Request) {
			tag := r.PathValue("tag")
			if !content.ValidSlug(tag) {
				h.notFound(w, r)
				return
			}
			h.list(w, r, lang, tag)
		})
		mux.HandleFunc("GET /"+lang+"/snippets", func(w http.ResponseWriter, r *http.Request) { h.snippets(w, r, lang, "") })
		mux.HandleFunc("GET /"+lang+"/snippets/{slug}", func(w http.ResponseWriter, r *http.Request) { h.snippets(w, r, lang, r.PathValue("slug")) })
		mux.HandleFunc("GET /"+lang+"/snippets/{slug}/raw/{file}", func(w http.ResponseWriter, r *http.Request) { h.snippetRaw(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/notes", func(w http.ResponseWriter, r *http.Request) { h.notes(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/notes/{slug}", func(w http.ResponseWriter, r *http.Request) { h.note(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/links", func(w http.ResponseWriter, r *http.Request) { h.links(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/favs", func(w http.ResponseWriter, r *http.Request) { h.favs(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/awesome", func(w http.ResponseWriter, r *http.Request) { h.awesomeIndex(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/awesome/{category}", func(w http.ResponseWriter, r *http.Request) { h.awesome(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/projects", func(w http.ResponseWriter, r *http.Request) { h.projects(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/homelab", func(w http.ResponseWriter, r *http.Request) { h.homelab(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/stream", func(w http.ResponseWriter, r *http.Request) { h.streamPage(w, r, lang, "") })
		mux.HandleFunc("GET /"+lang+"/stream/{month}", func(w http.ResponseWriter, r *http.Request) { h.streamPage(w, r, lang, r.PathValue("month")) })
		mux.HandleFunc("GET /"+lang+"/github", func(w http.ResponseWriter, r *http.Request) { h.github(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/photos", func(w http.ResponseWriter, r *http.Request) { h.photos(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/certificates", func(w http.ResponseWriter, r *http.Request) { h.certificates(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/about", func(w http.ResponseWriter, r *http.Request) { h.about(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/now", func(w http.ResponseWriter, r *http.Request) { h.now(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/now/archive", func(w http.ResponseWriter, r *http.Request) { h.nowArchive(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/now/archive/{day}", func(w http.ResponseWriter, r *http.Request) { h.nowVersion(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/projects/{slug}", func(w http.ResponseWriter, r *http.Request) { h.project(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/projects/{slug}/log/{log}", func(w http.ResponseWriter, r *http.Request) { h.buildlog(w, r, lang) })
	}
	// Everything else: redirects table, then the styled 404.
	mux.HandleFunc("GET /", h.fallback)
}

func (h *Handlers) root(w http.ResponseWriter, r *http.Request) {
	lang := i18n.FromRequest(r)
	// Varies per visitor, so the edge must not cache it under the path key.
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Vary", "Accept-Language, Cookie")
	http.Redirect(w, r, "/"+lang+"/", http.StatusFound)
}

// newPage builds the chrome and head shared by public pages. path is the
// language-free part of the URL ("/posts"); alternates come from it unless
// the page has its own (posts link to their translations).
func (h *Handlers) newPage(lang, active, title, path string, switchTo func(string) string) page {
	if switchTo == nil {
		switchTo = func(l string) string { return "/" + l + path }
	}
	alts := make([]alternate, 0, len(i18n.Supported))
	for _, l := range i18n.Supported {
		alts = append(alts, alternate{Lang: l, Href: h.origin + "/" + l + path})
	}
	return page{
		Chrome: ui.NewChrome(lang, active, false, switchTo),
		Head: Head{Title: title, Canonical: h.origin + "/" + lang + path, Alternates: alts, OGType: "website",
			Image: h.origin + og.URL(og.SiteKey(lang), og.Site(lang))},
	}
}

func (h *Handlers) render(w http.ResponseWriter, r *http.Request, name string, data any) {
	var buf bytes.Buffer
	if err := h.tmpl[name].ExecuteTemplate(&buf, "base", data); err != nil {
		slog.Error("render", "page", name, "path", r.URL.Path, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	writeCacheable(w, r, buf.Bytes())
}

func (h *Handlers) fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, content.ErrNotFound) {
		h.notFound(w, r)
		return
	}
	slog.Error("public", "path", r.URL.Path, "err", err)
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
}

// writeCacheable sends a page the edge Worker may store in KV. `public` is
// the opt-in signal the Worker checks; max-age=0 keeps browsers revalidating
// so a purge is visible on the next request.
func writeCacheable(w http.ResponseWriter, r *http.Request, body []byte) {
	writeCacheableType(w, r, "text/html; charset=utf-8", body)
}

// writeCacheableType is writeCacheable for another content type the Worker
// stores (worker/src/cache.ts CACHEABLE_TYPES).
func writeCacheableType(w http.ResponseWriter, r *http.Request, contentType string, body []byte) {
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}

// setLang stores the switcher's choice and returns to next. Public pages link
// here with next set to their translation, admin pages with their own path, so
// the cached public HTML stays identical for every visitor.
func setLang(w http.ResponseWriter, r *http.Request) {
	lang := r.PathValue("lang")
	if !i18n.IsSupported(lang) {
		http.NotFound(w, r)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: i18n.Cookie, Value: lang, Path: "/", MaxAge: 365 * 24 * 3600,
		Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, localPath(r.URL.Query().Get("next"), "/"+lang+"/"), http.StatusSeeOther)
}

// localPath returns next when it is a path on this site, else fallback, so
// /lang cannot be used as an open redirect.
func localPath(next, fallback string) string {
	u, err := url.Parse(next)
	if err != nil || next == "" || u.Scheme != "" || u.Host != "" ||
		!strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.Contains(next, "\\") {
		return fallback
	}
	return u.RequestURI()
}

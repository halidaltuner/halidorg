// Package feeds serves RSS, Atom and JSON Feed per language, RSS per tag,
// the sitemap and robots.txt. Responses are public so the edge Worker
// caches them; the entry.changed job re-renders them on publish.
package feeds

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/ui"
)

// Handlers serves the feeds.
type Handlers struct {
	db     store.DB
	posts  *post.Repo
	origin string
	env    string
}

// New reads PUBLIC_ORIGIN for absolute URLs.
func New(db store.DB) *Handlers {
	origin := strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/")
	if origin == "" {
		origin = "http://localhost:8787"
	}
	return &Handlers{db: db, posts: post.NewRepo(db), origin: origin, env: ui.Env()}
}

// Register adds the routes. Language routes use literal prefixes (/tr/,
// /en/), not a {lang} wildcard: /{lang}/feed.xml and /lang/{lang} both match
// /lang/feed.xml and ServeMux would panic.
func (h *Handlers) Register(mux *http.ServeMux) {
	for _, l := range i18n.Supported {
		lang := l
		mux.HandleFunc("GET /"+lang+"/feed.xml", func(w http.ResponseWriter, r *http.Request) { h.rss(w, r, lang, "") })
		mux.HandleFunc("GET /"+lang+"/atom.xml", func(w http.ResponseWriter, r *http.Request) { h.atom(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/feed.json", func(w http.ResponseWriter, r *http.Request) { h.jsonFeed(w, r, lang) })
		mux.HandleFunc("GET /"+lang+"/tags/{tag}/feed.xml", func(w http.ResponseWriter, r *http.Request) {
			tag := r.PathValue("tag")
			if !content.ValidSlug(tag) {
				http.NotFound(w, r)
				return
			}
			h.rss(w, r, lang, tag)
		})
	}
	mux.HandleFunc("GET /sitemap.xml", h.sitemap)
	mux.HandleFunc("GET /robots.txt", h.robots)
}

func (h *Handlers) postURL(p post.Post) string {
	return h.origin + "/" + p.Lang + "/posts/" + p.Slug
}

func ts(sec *int64) time.Time {
	if sec == nil {
		return time.Unix(0, 0).UTC()
	}
	return time.Unix(*sec, 0).UTC()
}

func (h *Handlers) load(w http.ResponseWriter, r *http.Request, lang, tag string) ([]post.Published, bool) {
	items, err := h.posts.Feed(r.Context(), lang, tag, 20)
	if err != nil {
		slog.Error("feed", "lang", lang, "tag", tag, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return nil, false
	}
	return items, true
}

// --- RSS 2.0 ----------------------------------------------------------------

type rssDoc struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Content string     `xml:"xmlns:content,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title         string    `xml:"title"`
	Link          string    `xml:"link"`
	Self          atomLink  `xml:"atom:link"`
	Description   string    `xml:"description"`
	Language      string    `xml:"language"`
	LastBuildDate string    `xml:"lastBuildDate,omitempty"`
	Items         []rssItem `xml:"item"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr,omitempty"`
}

type rssItem struct {
	Title       string   `xml:"title"`
	Link        string   `xml:"link"`
	GUID        string   `xml:"guid"`
	PubDate     string   `xml:"pubDate"`
	Description string   `xml:"description,omitempty"`
	Content     cdata    `xml:"content:encoded"`
	Categories  []string `xml:"category"`
}

type cdata struct {
	Text string `xml:",cdata"`
}

func (h *Handlers) rss(w http.ResponseWriter, r *http.Request, lang, tag string) {
	items, ok := h.load(w, r, lang, tag)
	if !ok {
		return
	}
	self := h.origin + "/" + lang + "/feed.xml"
	link := h.origin + "/" + lang + "/"
	title := i18n.T(lang, "feed.posts")
	if tag != "" {
		self = h.origin + "/" + lang + "/tags/" + tag + "/feed.xml"
		link = h.origin + "/" + lang + "/tags/" + tag
		title += " · #" + tag
	}
	ch := rssChannel{Title: title, Link: link, Self: atomLink{Href: self, Rel: "self", Type: "application/rss+xml"},
		Description: title, Language: lang}
	for i, p := range items {
		if i == 0 {
			ch.LastBuildDate = ts(p.PublishedAt).Format(time.RFC1123Z)
		}
		u := h.postURL(p.Post)
		ch.Items = append(ch.Items, rssItem{Title: p.Title, Link: u, GUID: u, PubDate: ts(p.PublishedAt).Format(time.RFC1123Z),
			Description: p.Summary, Content: cdata{abs(p.HTML, h.origin)}, Categories: p.Tags})
	}
	h.writeXML(w, r, "application/rss+xml; charset=utf-8", rssDoc{Version: "2.0", Atom: "http://www.w3.org/2005/Atom",
		Content: "http://purl.org/rss/1.0/modules/content/", Channel: ch})
}

// --- Atom -------------------------------------------------------------------

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Lang    string      `xml:"xml:lang,attr"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Links   []atomLink  `xml:"link"`
	Author  atomAuthor  `xml:"author"`
	Entries []atomEntry `xml:"entry"`
}

type atomAuthor struct {
	Name string `xml:"name"`
}

type atomEntry struct {
	Title     string    `xml:"title"`
	ID        string    `xml:"id"`
	Link      atomLink  `xml:"link"`
	Published string    `xml:"published"`
	Updated   string    `xml:"updated"`
	Summary   string    `xml:"summary,omitempty"`
	Content   atomText  `xml:"content"`
	Category  []atomCat `xml:"category"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type atomCat struct {
	Term string `xml:"term,attr"`
}

func (h *Handlers) atom(w http.ResponseWriter, r *http.Request, lang string) {
	items, ok := h.load(w, r, lang, "")
	if !ok {
		return
	}
	self := h.origin + "/" + lang + "/atom.xml"
	f := atomFeed{Lang: lang, Title: i18n.T(lang, "feed.posts"), ID: self, Updated: time.Unix(0, 0).UTC().Format(time.RFC3339),
		Links:  []atomLink{{Href: self, Rel: "self", Type: "application/atom+xml"}, {Href: h.origin + "/" + lang + "/", Rel: "alternate", Type: "text/html"}},
		Author: atomAuthor{Name: "halid.org"}}
	for i, p := range items {
		upd := time.Unix(p.UpdatedAt, 0).UTC().Format(time.RFC3339)
		if i == 0 {
			f.Updated = upd
		}
		u := h.postURL(p.Post)
		e := atomEntry{Title: p.Title, ID: u, Link: atomLink{Href: u, Rel: "alternate", Type: "text/html"},
			Published: ts(p.PublishedAt).Format(time.RFC3339), Updated: upd, Summary: p.Summary,
			Content: atomText{Type: "html", Text: abs(p.HTML, h.origin)}}
		for _, t := range p.Tags {
			e.Category = append(e.Category, atomCat{Term: t})
		}
		f.Entries = append(f.Entries, e)
	}
	h.writeXML(w, r, "application/atom+xml; charset=utf-8", f)
}

// --- JSON Feed 1.1 ----------------------------------------------------------

type jsonFeedDoc struct {
	Version     string         `json:"version"`
	Title       string         `json:"title"`
	HomePageURL string         `json:"home_page_url"`
	FeedURL     string         `json:"feed_url"`
	Language    string         `json:"language"`
	Items       []jsonFeedItem `json:"items"`
}

type jsonFeedItem struct {
	ID            string   `json:"id"`
	URL           string   `json:"url"`
	Title         string   `json:"title"`
	Summary       string   `json:"summary,omitempty"`
	ContentHTML   string   `json:"content_html"`
	DatePublished string   `json:"date_published"`
	DateModified  string   `json:"date_modified"`
	Tags          []string `json:"tags,omitempty"`
}

func (h *Handlers) jsonFeed(w http.ResponseWriter, r *http.Request, lang string) {
	items, ok := h.load(w, r, lang, "")
	if !ok {
		return
	}
	doc := jsonFeedDoc{Version: "https://jsonfeed.org/version/1.1", Title: i18n.T(lang, "feed.posts"),
		HomePageURL: h.origin + "/" + lang + "/", FeedURL: h.origin + "/" + lang + "/feed.json", Language: lang,
		Items: []jsonFeedItem{}}
	for _, p := range items {
		u := h.postURL(p.Post)
		doc.Items = append(doc.Items, jsonFeedItem{ID: u, URL: u, Title: p.Title, Summary: p.Summary,
			ContentHTML: abs(p.HTML, h.origin), DatePublished: ts(p.PublishedAt).Format(time.RFC3339),
			DateModified: time.Unix(p.UpdatedAt, 0).UTC().Format(time.RFC3339), Tags: p.Tags})
	}
	body, err := json.Marshal(doc)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	write(w, r, "application/feed+json; charset=utf-8", body)
}

// --- Sitemap ----------------------------------------------------------------

type urlset struct {
	XMLName xml.Name `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	XHTML   string   `xml:"xmlns:xhtml,attr"`
	URLs    []smURL  `xml:"url"`
}

type smURL struct {
	Loc     string  `xml:"loc"`
	LastMod string  `xml:"lastmod,omitempty"`
	Alts    []smAlt `xml:"xhtml:link"`
}

type smAlt struct {
	Rel      string `xml:"rel,attr"`
	Hreflang string `xml:"hreflang,attr"`
	Href     string `xml:"href,attr"`
}

type smRow struct {
	Lang      string `json:"lang"`
	Slug      string `json:"slug"`
	Group     string `json:"translation_group_id"`
	UpdatedAt int64  `json:"updated_at"`
}

func (h *Handlers) sitemapRows(ctx context.Context) ([]smRow, error) {
	res, err := h.db.Query(ctx, `SELECT lang, slug, translation_group_id, updated_at FROM entries
WHERE kind = 'post' AND status = 'published' AND visibility = 'public' ORDER BY published_at DESC LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	return store.Scan[smRow](res)
}

func (h *Handlers) sitemap(w http.ResponseWriter, r *http.Request) {
	rows, err := h.sitemapRows(r.Context())
	if err != nil {
		slog.Error("sitemap", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	set := urlset{XHTML: "http://www.w3.org/1999/xhtml"}
	// Home and post list exist in every language: alternate each other.
	for _, path := range []string{"/", "/posts"} {
		var alts []smAlt
		for _, l := range i18n.Supported {
			alts = append(alts, smAlt{Rel: "alternate", Hreflang: l, Href: h.origin + "/" + l + path})
		}
		for _, l := range i18n.Supported {
			set.URLs = append(set.URLs, smURL{Loc: h.origin + "/" + l + path, Alts: alts})
		}
	}
	groups := map[string][]smRow{}
	for _, row := range rows {
		groups[row.Group] = append(groups[row.Group], row)
	}
	for _, row := range rows {
		u := smURL{Loc: h.origin + "/" + row.Lang + "/posts/" + row.Slug, LastMod: time.Unix(row.UpdatedAt, 0).UTC().Format("2006-01-02")}
		if sibs := groups[row.Group]; len(sibs) > 1 {
			for _, s := range sibs {
				u.Alts = append(u.Alts, smAlt{Rel: "alternate", Hreflang: s.Lang, Href: h.origin + "/" + s.Lang + "/posts/" + s.Slug})
			}
		}
		set.URLs = append(set.URLs, u)
	}
	h.writeXML(w, r, "application/xml; charset=utf-8", set)
}

// --- robots.txt -------------------------------------------------------------

func (h *Handlers) robots(w http.ResponseWriter, r *http.Request) {
	body := "User-agent: *\nDisallow: /\n"
	if h.env == "prod" {
		body = "User-agent: *\nDisallow: /admin\nDisallow: /x/\n\nSitemap: " + h.origin + "/sitemap.xml\n"
	}
	write(w, r, "text/plain; charset=utf-8", []byte(body))
}

// --- helpers ----------------------------------------------------------------

func (h *Handlers) writeXML(w http.ResponseWriter, r *http.Request, ct string, v any) {
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(v); err != nil {
		slog.Error("feed encode", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	buf.WriteByte('\n')
	write(w, r, ct, buf.Bytes())
}

// write sends a response the Worker may cache (see public.writeCacheable).
func write(w http.ResponseWriter, r *http.Request, ct string, body []byte) {
	sum := sha256.Sum256(body)
	etag := `"` + hex.EncodeToString(sum[:8]) + `"`
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
	w.Header().Set("ETag", etag)
	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	_, _ = w.Write(body)
}

// abs makes root-relative href and src attributes absolute: feed readers
// resolve them against the feed URL, not the site.
func abs(html, origin string) string {
	// Protocol-relative URLs come first: the replacer tries olds in order.
	r := strings.NewReplacer(`href="//`, `href="//`, `src="//`, `src="//`,
		`href="/`, `href="`+origin+`/`, `src="/`, `src="`+origin+`/`)
	return r.Replace(html)
}

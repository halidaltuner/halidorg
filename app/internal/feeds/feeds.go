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
	"html/template"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/certificate"
	"halid.org/engine/internal/content/photo"
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
		mux.HandleFunc("GET /"+lang+"/feed.xml", func(w http.ResponseWriter, r *http.Request) { h.rss(w, r, lang, "", "") })
		mux.HandleFunc("GET /"+lang+"/atom.xml", func(w http.ResponseWriter, r *http.Request) { h.atom(w, r, lang, "") })
		mux.HandleFunc("GET /"+lang+"/feed.json", func(w http.ResponseWriter, r *http.Request) { h.jsonFeed(w, r, lang, "") })
		mux.HandleFunc("GET /"+lang+"/tags/{tag}/feed.xml", func(w http.ResponseWriter, r *http.Request) {
			tag := r.PathValue("tag")
			if !content.ValidSlug(tag) {
				http.NotFound(w, r)
				return
			}
			h.rss(w, r, lang, "", tag)
		})
		// Per-kind feeds (CONTENT_MODEL.md §5): /{lang}/{notes,snippets,links,favs}/feed.xml.
		for kind, seg := range kindFeeds {
			kind, base := kind, "/"+lang+"/"+seg
			mux.HandleFunc("GET "+base+"/feed.xml", func(w http.ResponseWriter, r *http.Request) { h.rss(w, r, lang, kind, "") })
			mux.HandleFunc("GET "+base+"/atom.xml", func(w http.ResponseWriter, r *http.Request) { h.atom(w, r, lang, kind) })
			mux.HandleFunc("GET "+base+"/feed.json", func(w http.ResponseWriter, r *http.Request) { h.jsonFeed(w, r, lang, kind) })
		}
	}
	mux.HandleFunc("GET /sitemap.xml", h.sitemap)
	mux.HandleFunc("GET /robots.txt", h.robots)
}

// kindFeeds are the kinds with their own feeds and the URL segment they sit
// under. Posts use the language feeds (/{lang}/feed.xml).
var kindFeeds = map[string]string{"note": "notes", "snippet": "snippets", "link": "links", "fav": "favs", "project": "projects",
	"certificate": "certificates", "photo": "photos", "presentation": "presentations"}

// item is one feed entry, whatever its kind.
type item struct {
	URL, Title, Summary, HTML string
	Published                 *int64
	Updated                   int64
	Tags                      []string
}

// channel names a feed: its title, its own URL and the page it belongs to.
type channel struct {
	Title, Self, Link string
}

func (h *Handlers) channel(lang, kind, tag, file string) channel {
	if kind != "" {
		base := h.origin + "/" + lang + "/" + kindFeeds[kind]
		return channel{Title: i18n.T(lang, "feed."+kindFeeds[kind]), Self: base + "/" + file, Link: base}
	}
	c := channel{Title: i18n.T(lang, "feed.posts"), Self: h.origin + "/" + lang + "/" + file, Link: h.origin + "/" + lang + "/"}
	if tag != "" {
		c.Self = h.origin + "/" + lang + "/tags/" + tag + "/feed.xml"
		c.Link = h.origin + "/" + lang + "/tags/" + tag
		c.Title += " · #" + tag
	}
	return c
}

func ts(sec *int64) time.Time {
	if sec == nil {
		return time.Unix(0, 0).UTC()
	}
	return time.Unix(*sec, 0).UTC()
}

func (h *Handlers) load(w http.ResponseWriter, r *http.Request, lang, kind, tag string) ([]item, bool) {
	var items []item
	var err error
	if kind == "" {
		items, err = h.postItems(r.Context(), lang, tag)
	} else if kind == "certificate" {
		items, err = h.certItems(r.Context(), lang)
	} else if kind == "photo" {
		items, err = h.photoItems(r.Context(), lang)
	} else {
		items, err = h.kindItems(r.Context(), lang, kind)
	}
	if err != nil {
		slog.Error("feed", "lang", lang, "kind", kind, "tag", tag, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return nil, false
	}
	return items, true
}

func (h *Handlers) postItems(ctx context.Context, lang, tag string) ([]item, error) {
	posts, err := h.posts.Feed(ctx, lang, tag, 20)
	if err != nil {
		return nil, err
	}
	out := make([]item, len(posts))
	for i, p := range posts {
		out[i] = item{URL: h.origin + "/" + p.Lang + "/posts/" + p.Slug, Title: p.Title, Summary: p.Summary, HTML: p.HTML,
			Published: p.PublishedAt, Updated: p.UpdatedAt, Tags: p.Tags}
	}
	return out, nil
}

// certItems are the published certificates, newest issue first: the same
// list in both languages, each linking to its verification page when it has
// one, else to the certificates page.
func (h *Handlers) certItems(ctx context.Context, lang string) ([]item, error) {
	repo := certificate.NewRepo(h.db)
	certs, err := repo.Published(ctx)
	if err != nil {
		return nil, err
	}
	issuers, err := repo.Issuers(ctx)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for _, i := range issuers {
		names[i.ID] = i.Name
	}
	sort.SliceStable(certs, func(i, j int) bool { return certs[i].IssuedAt > certs[j].IssuedAt })
	out := make([]item, 0, min(len(certs), 20))
	for _, c := range certs[:min(len(certs), 20)] {
		it := item{Title: c.Name, Summary: names[c.IssuerID], URL: h.origin + "/" + lang + "/certificates", Updated: c.UpdatedAt, Tags: c.Skills}
		issued := c.IssuedAt
		it.Published = &issued
		if c.CredlyURL != nil {
			it.URL = *c.CredlyURL
		}
		it.HTML = "<p>" + template.HTMLEscapeString(names[c.IssuerID]+" · "+certificate.Month(&c.IssuedAt)) + "</p>"
		out = append(out, it)
	}
	return out, nil
}

// photoItems are the newest published photos (by when they were added):
// each item shows the image and links to its lightbox on the photos page.
func (h *Handlers) photoItems(ctx context.Context, lang string) ([]item, error) {
	repo := photo.NewRepo(h.db, nil)
	list, err := repo.Published(ctx)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(list, func(i, j int) bool { return list[i].CreatedAt > list[j].CreatedAt })
	list = list[:min(len(list), 20)]
	type img struct{ url, alt string }
	imgs := map[string]img{}
	if len(list) > 0 {
		args := make([]any, len(list))
		for i, x := range list {
			args[i] = x.MediaID
		}
		res, err := h.db.Query(ctx, `SELECT id, r2_key, alt_tr, alt_en FROM media WHERE id IN (?`+strings.Repeat(", ?", len(args)-1)+`)`, args...)
		if err != nil {
			return nil, err
		}
		rows, err := store.Scan[struct {
			ID    string `json:"id"`
			Key   string `json:"r2_key"`
			AltTR string `json:"alt_tr"`
			AltEN string `json:"alt_en"`
		}](res)
		if err != nil {
			return nil, err
		}
		for _, m := range rows {
			alt := m.AltEN
			if lang == "tr" {
				alt = m.AltTR
			}
			imgs[m.ID] = img{h.origin + "/" + m.Key, alt}
		}
	}
	out := make([]item, 0, len(list))
	for _, x := range list {
		created := x.CreatedAt
		meta := strings.Trim(strings.Join([]string{x.Place, x.Camera, x.Exposure}, " · "), " ·")
		body := "<p>" + template.HTMLEscapeString(meta) + "</p>"
		if d := x.Description(lang); d != "" {
			body = "<p>" + strings.ReplaceAll(template.HTMLEscapeString(d), "\n", "<br>") + "</p>" + body
		}
		if im, ok := imgs[x.MediaID]; ok {
			if im.alt == "" {
				im.alt = x.Title(lang)
			}
			body = `<p><img src="` + template.HTMLEscapeString(im.url) + `" alt="` + template.HTMLEscapeString(im.alt) + `"></p>` + body
		}
		out = append(out, item{Title: x.Title(lang), Summary: meta, URL: h.origin + "/" + lang + "/photos#p-" + x.ID,
			Published: &created, Updated: x.UpdatedAt, HTML: body})
	}
	return out, nil
}

// kindItems reads the newest published entries of a short-form kind with the
// HTML of their published revision. Links point at the link itself, favs at
// their URL when they have one; notes and snippets at their page. The
// projects feed carries build logs too, under a published project, titled
// "project: log".
func (h *Handlers) kindItems(ctx context.Context, lang, kind string) ([]item, error) {
	logs := ""
	if kind == "project" {
		logs = "buildlog"
	}
	res, err := h.db.Query(ctx, `SELECT e.slug, e.lang, e.title, e.summary, e.published_at, e.updated_at, r.body_html,
  l.url AS link_url, f.url AS fav_url, pe.slug AS parent, pe.title AS parent_title,
  (SELECT group_concat(t.slug, ' ') FROM entry_tags et JOIN tags t ON t.id = et.tag_id WHERE et.entry_id = e.id) AS tags
FROM entries e JOIN revisions r ON r.id = e.published_revision_id
LEFT JOIN links l ON l.entry_id = e.id LEFT JOIN favs f ON f.entry_id = e.id
LEFT JOIN buildlogs b ON b.entry_id = e.id LEFT JOIN entries pe ON pe.id = b.project_entry_id
WHERE e.kind IN (?, ?) AND e.lang = ? AND e.status = 'published' AND e.visibility = 'public'
  AND (e.kind <> 'buildlog' OR (pe.status = 'published' AND pe.visibility = 'public'))
ORDER BY e.published_at DESC, e.id DESC LIMIT 20`, kind, logs, lang)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		Slug      string  `json:"slug"`
		Lang      string  `json:"lang"`
		Title     string  `json:"title"`
		Summary   string  `json:"summary"`
		Published *int64  `json:"published_at"`
		Updated   int64   `json:"updated_at"`
		HTML      string  `json:"body_html"`
		LinkURL   *string `json:"link_url"`
		FavURL    *string `json:"fav_url"`
		Parent    *string `json:"parent"`
		PTitle    *string `json:"parent_title"`
		Tags      *string `json:"tags"`
	}](res)
	if err != nil {
		return nil, err
	}
	out := make([]item, len(rows))
	for i, x := range rows {
		it := item{Title: x.Title, Summary: x.Summary, HTML: x.HTML, Published: x.Published, Updated: x.Updated}
		switch {
		case kind == "link" && x.LinkURL != nil:
			it.URL = *x.LinkURL
		case kind == "fav" && x.FavURL != nil:
			it.URL = *x.FavURL
		case kind == "fav":
			it.URL = h.origin + "/" + x.Lang + "/favs#" + x.Slug
		case x.Parent != nil:
			it.URL = h.origin + "/" + x.Lang + "/projects/" + *x.Parent + "/log/" + x.Slug
			if x.PTitle != nil && *x.PTitle != "" {
				it.Title = *x.PTitle + ": " + x.Title
			}
		default:
			it.URL = h.origin + "/" + x.Lang + "/" + kindFeeds[kind] + "/" + x.Slug
		}
		if it.Title == "" { // untitled notes
			it.Title = x.Summary
		}
		if it.Title == "" {
			it.Title = x.Slug
		}
		if x.Tags != nil && *x.Tags != "" {
			it.Tags = strings.Fields(*x.Tags)
		}
		// A link's comment is its content when it has no longer note.
		if it.HTML == "" && x.Summary != "" {
			it.HTML = "<p>" + template.HTMLEscapeString(x.Summary) + "</p>"
		}
		out[i] = it
	}
	return out, nil
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

func (h *Handlers) rss(w http.ResponseWriter, r *http.Request, lang, kind, tag string) {
	items, ok := h.load(w, r, lang, kind, tag)
	if !ok {
		return
	}
	c := h.channel(lang, kind, tag, "feed.xml")
	ch := rssChannel{Title: c.Title, Link: c.Link, Self: atomLink{Href: c.Self, Rel: "self", Type: "application/rss+xml"},
		Description: c.Title, Language: lang}
	for i, p := range items {
		if i == 0 {
			ch.LastBuildDate = ts(p.Published).Format(time.RFC1123Z)
		}
		ch.Items = append(ch.Items, rssItem{Title: p.Title, Link: p.URL, GUID: p.URL, PubDate: ts(p.Published).Format(time.RFC1123Z),
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

func (h *Handlers) atom(w http.ResponseWriter, r *http.Request, lang, kind string) {
	items, ok := h.load(w, r, lang, kind, "")
	if !ok {
		return
	}
	c := h.channel(lang, kind, "", "atom.xml")
	f := atomFeed{Lang: lang, Title: c.Title, ID: c.Self, Updated: time.Unix(0, 0).UTC().Format(time.RFC3339),
		Links:  []atomLink{{Href: c.Self, Rel: "self", Type: "application/atom+xml"}, {Href: c.Link, Rel: "alternate", Type: "text/html"}},
		Author: atomAuthor{Name: "halid.org"}}
	for i, p := range items {
		upd := time.Unix(p.Updated, 0).UTC().Format(time.RFC3339)
		if i == 0 {
			f.Updated = upd
		}
		e := atomEntry{Title: p.Title, ID: p.URL, Link: atomLink{Href: p.URL, Rel: "alternate", Type: "text/html"},
			Published: ts(p.Published).Format(time.RFC3339), Updated: upd, Summary: p.Summary,
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

func (h *Handlers) jsonFeed(w http.ResponseWriter, r *http.Request, lang, kind string) {
	items, ok := h.load(w, r, lang, kind, "")
	if !ok {
		return
	}
	c := h.channel(lang, kind, "", "feed.json")
	doc := jsonFeedDoc{Version: "https://jsonfeed.org/version/1.1", Title: c.Title,
		HomePageURL: c.Link, FeedURL: c.Self, Language: lang, Items: []jsonFeedItem{}}
	for _, p := range items {
		doc.Items = append(doc.Items, jsonFeedItem{ID: p.URL, URL: p.URL, Title: p.Title, Summary: p.Summary,
			ContentHTML: abs(p.HTML, h.origin), DatePublished: ts(p.Published).Format(time.RFC3339),
			DateModified: time.Unix(p.Updated, 0).UTC().Format(time.RFC3339), Tags: p.Tags})
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
	Kind      string `json:"kind"`
	Lang      string `json:"lang"`
	Slug      string `json:"slug"`
	Group     string `json:"translation_group_id"`
	UpdatedAt int64  `json:"updated_at"`
}

func (h *Handlers) sitemapRows(ctx context.Context) ([]smRow, error) {
	// A build log lives under its project: its "slug" here is the path
	// below /{lang}/projects/ (project/log/slug).
	res, err := h.db.Query(ctx, `SELECT kind, lang, slug, translation_group_id, updated_at FROM entries
WHERE kind IN ('post', 'note', 'snippet', 'project', 'presentation', 'page') AND status = 'published' AND visibility = 'public'
UNION ALL
SELECT 'project', e.lang, pe.slug || '/log/' || e.slug, e.translation_group_id, e.updated_at
FROM entries e JOIN buildlogs b ON b.entry_id = e.id JOIN entries pe ON pe.id = b.project_entry_id
WHERE e.status = 'published' AND e.visibility = 'public' AND pe.status = 'published' AND pe.visibility = 'public'
LIMIT 5000`)
	if err != nil {
		return nil, err
	}
	return store.Scan[smRow](res)
}

// awesomeRows lists the awesome pages that render: link categories, and
// their parents, with published public links in a language.
func (h *Handlers) awesomeRows(ctx context.Context) ([]smRow, error) {
	res, err := h.db.Query(ctx, `SELECT DISTINCT e.lang, c.slug FROM links l
JOIN entries e ON e.id = l.entry_id JOIN categories c ON c.id = l.category_id
WHERE e.status = 'published' AND e.visibility = 'public'
UNION
SELECT DISTINCT e.lang, p.slug FROM links l
JOIN entries e ON e.id = l.entry_id JOIN categories c ON c.id = l.category_id JOIN categories p ON p.id = c.parent_id
WHERE e.status = 'published' AND e.visibility = 'public'
ORDER BY 1, 2`)
	if err != nil {
		return nil, err
	}
	return store.Scan[smRow](res)
}

// streamMonths lists the /stream archive pages that render: months with
// stream rows per language (public.streamMonths; GitHub rows in both).
func (h *Handlers) streamMonths(ctx context.Context) ([]smRow, error) {
	res, err := h.db.Query(ctx, `SELECT DISTINCT e.lang, strftime('%Y-%m', e.published_at, 'unixepoch') AS slug FROM entries e
LEFT JOIN buildlogs b ON b.entry_id = e.id LEFT JOIN entries pe ON pe.id = b.project_entry_id
WHERE e.status = 'published' AND e.visibility = 'public' AND e.published_at IS NOT NULL
  AND e.kind IN ('post', 'note', 'snippet', 'link', 'fav', 'project', 'buildlog')
  AND (e.kind <> 'buildlog' OR (pe.status = 'published' AND pe.visibility = 'public'))
ORDER BY 1, 2 LIMIT 2000`)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[smRow](res)
	if err != nil {
		return nil, err
	}
	res, err = h.db.Query(ctx, `SELECT DISTINCT strftime('%Y-%m', a.occurred_at, 'unixepoch') AS slug FROM gh_activity a
JOIN gh_repos g ON g.full_name = a.repo WHERE g.gone = 0 ORDER BY 1 LIMIT 1000`)
	if err != nil {
		return nil, err
	}
	gh, err := store.Scan[smRow](res)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, r := range rows {
		seen[r.Lang+r.Slug] = true
	}
	for _, l := range i18n.Supported {
		for _, m := range gh {
			if !seen[l+m.Slug] {
				rows = append(rows, smRow{Lang: l, Slug: m.Slug})
			}
		}
	}
	return rows, nil
}

// itemSegments are the kinds with a page per entry and their URL segment.
var itemSegments = map[string]string{"post": "posts", "note": "notes", "snippet": "snippets", "project": "projects",
	"presentation": "presentations"}

func itemPath(r smRow) string {
	if r.Kind == "page" { // About and Now sit at the language root
		return "/" + r.Lang + "/" + r.Slug
	}
	return "/" + r.Lang + "/" + itemSegments[r.Kind] + "/" + r.Slug
}

func (h *Handlers) sitemap(w http.ResponseWriter, r *http.Request) {
	rows, err := h.sitemapRows(r.Context())
	if err != nil {
		slog.Error("sitemap", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	set := urlset{XHTML: "http://www.w3.org/1999/xhtml"}
	// Home and the list pages exist in every language: alternate each other.
	for _, path := range []string{"/", "/posts", "/notes", "/snippets", "/links", "/favs", "/awesome", "/projects", "/homelab", "/stream", "/github", "/certificates", "/photos", "/presentations"} {
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
		u := smURL{Loc: h.origin + itemPath(row), LastMod: time.Unix(row.UpdatedAt, 0).UTC().Format("2006-01-02")}
		if sibs := groups[row.Group]; len(sibs) > 1 {
			for _, s := range sibs {
				u.Alts = append(u.Alts, smAlt{Rel: "alternate", Hreflang: s.Lang, Href: h.origin + itemPath(s)})
			}
		}
		set.URLs = append(set.URLs, u)
	}
	awesome, err := h.awesomeRows(r.Context())
	if err != nil {
		slog.Error("sitemap", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	for _, a := range awesome {
		set.URLs = append(set.URLs, smURL{Loc: h.origin + "/" + a.Lang + "/awesome/" + a.Slug})
	}
	months, err := h.streamMonths(r.Context())
	if err != nil {
		slog.Error("sitemap", "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	for _, m := range months {
		set.URLs = append(set.URLs, smURL{Loc: h.origin + "/" + m.Lang + "/stream/" + m.Slug})
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

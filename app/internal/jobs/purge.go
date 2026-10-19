package jobs

import (
	"net/url"
	"sort"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
)

// Ref locates an entry's public page: what it is and where it lives.
type Ref struct {
	Kind   string   `json:"kind"`
	Lang   string   `json:"lang"`
	Slug   string   `json:"slug"`
	Tags   []string `json:"tags,omitempty"`
	Listed bool     `json:"listed"` // published and public: appears in lists, feeds, sitemap
	Title  string   `json:"title,omitempty"`
	// Files are a snippet's file names, in tab order: each has a raw URL.
	Files []string `json:"files,omitempty"`
	// Categories are a link's category and its parent, by slug: each has
	// an awesome list page.
	Categories []string `json:"categories,omitempty"`
	// Homelab marks a homelab project: it is also on the homelab page.
	Homelab bool `json:"homelab,omitempty"`
	// Parent is a build log's project slug: the log lives under it and the
	// project page shows its timeline.
	Parent string `json:"parent,omitempty"`
	// Logs are a project's build log slugs: their URLs sit under the
	// project's, so they move (and are purged) with it.
	Logs []string `json:"logs,omitempty"`
	// Month is when it was published (2006-01, UTC): its /stream/{month}
	// archive page lists it.
	Month string `json:"month,omitempty"`
	// Version is a page's latest published version (2006-01-02): the Now
	// archive has a page per day.
	Version string `json:"version,omitempty"`
	// Series is its series' slug; Parts are the series' other published
	// parts in its language ("kind:slug"), whose pages show its title in
	// their series nav.
	Series string   `json:"series,omitempty"`
	Parts  []string `json:"parts,omitempty"`
}

// SeriesPaths are the pages a series shows on: the index and its own page.
func SeriesPaths(lang, slug string) []string {
	if !i18n.IsSupported(lang) || !content.ValidSlug(slug) {
		return nil
	}
	return []string{"/" + lang + "/series", "/" + lang + "/series/" + slug}
}

// StreamMonth is the /stream archive page of a month, or "".
func StreamMonth(lang, month string) string {
	if month == "" || !i18n.IsSupported(lang) {
		return ""
	}
	return "/" + lang + "/stream/" + month
}

// Change is one entry change: its state now (nil when deleted), its state
// before the change (nil when new), and its translations, whose pages link
// to it through hreflang and the language switch.
type Change struct {
	Now      *Ref
	Prev     *Ref
	Siblings []Ref
	Linked   []Ref // entries it links to, before or after: their backlinks change
	// Peers are the other listed snippets in the entry's language, set when
	// the snippet list changes (PeersChanged): every snippet page shows it.
	Peers []Ref
}

// PeersChanged reports whether c changes what the snippet list on every
// snippet page shows: a snippet joins or leaves it, or its title, slug or
// first file (the language in the row) changes.
func PeersChanged(c Change) bool {
	now, prev := c.Now, c.Prev
	if now == nil && prev == nil {
		return false
	}
	r := now
	if r == nil {
		r = prev
	}
	if r.Kind != "snippet" {
		return false
	}
	listed := func(x *Ref) bool { return x != nil && x.Listed }
	if listed(now) != listed(prev) {
		return true
	}
	if !listed(now) {
		return false
	}
	return now.Title != prev.Title || now.Slug != prev.Slug || now.Lang != prev.Lang || first(now.Files) != first(prev.Files)
}

func first(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}

// RawPath is the public path of one snippet file as text/plain.
func RawPath(r Ref, file string) string {
	item := ItemPath(r)
	if item == "" || r.Kind != "snippet" || file == "" {
		return ""
	}
	return item + "/raw/" + url.PathEscape(file)
}

// kindPath is the URL segment of the kinds that have a page per entry
// (CONTENT_MODEL.md §5).
var kindPath = map[string]string{
	"post":         "posts",
	"note":         "notes",
	"snippet":      "snippets",
	"project":      "projects",
	"presentation": "presentations",
	"garden":       "garden",
	"runbook":      "runbooks",
}

// listPath is the URL segment of each kind's list page and per-kind feeds.
// Links and favs have no item pages: their list is where they live.
var listPath = map[string]string{
	"post":         "posts",
	"note":         "notes",
	"snippet":      "snippets",
	"link":         "links",
	"fav":          "favs",
	"project":      "projects",
	"presentation": "presentations",
	"garden":       "garden",
	"runbook":      "runbooks",
}

// ListSegment is the URL segment of a kind's list page, or "".
func ListSegment(kind string) string { return listPath[kind] }

// ItemSegment is the URL segment of a kind's item pages, or "" when the kind
// has none.
func ItemSegment(kind string) string { return kindPath[kind] }

// ItemPath is the public path of one entry, or "" for kinds without pages yet.
// A build log lives under its project: /{lang}/projects/{project}/log/{slug}.
func ItemPath(r Ref) string {
	if r.Kind == "page" { // About and Now: the slug is the template
		if r.Slug == "" || !i18n.IsSupported(r.Lang) {
			return ""
		}
		return "/" + r.Lang + "/" + r.Slug
	}
	if r.Kind == "buildlog" {
		if r.Slug == "" || r.Parent == "" || !i18n.IsSupported(r.Lang) {
			return ""
		}
		return "/" + r.Lang + "/projects/" + r.Parent + "/log/" + r.Slug
	}
	seg, ok := kindPath[r.Kind]
	if !ok || r.Slug == "" || !i18n.IsSupported(r.Lang) {
		return ""
	}
	return "/" + r.Lang + "/" + seg + "/" + r.Slug
}

// Paths returns every public path whose HTML or feed can change with c,
// sorted and without duplicates. The purge job re-renders each one; a path
// that no longer renders is deleted from KV.
//
// For an entry of kind K in lang L: its page (kinds with item pages), the
// same slug in the other languages (the untranslated fallback), K's list
// page and K's feeds, home (the stream), each tag page and tag feed (old and
// new tags), the language feeds (posts), the sitemap, and the pages of its
// translations. A slug change also covers the old URL. Snippets add their
// raw files and, when the list changes, the pages of the other snippets;
// links the awesome pages of their category and its parent, and the
// awesome index.
func Paths(c Change) []string {
	set := map[string]bool{}
	add := func(p string) {
		if p != "" {
			set[p] = true
		}
	}
	for _, r := range []*Ref{c.Now, c.Prev} {
		if r == nil {
			continue
		}
		add(ItemPath(*r))
		for _, s := range r.Logs {
			add(ItemPath(Ref{Kind: "buildlog", Lang: r.Lang, Slug: s, Parent: r.Slug}))
		}
		if r.Kind == "buildlog" && i18n.IsSupported(r.Lang) {
			// the project's timeline, the projects feeds (they carry build
			// logs), and the stream on home and /stream
			add(ItemPath(Ref{Kind: "project", Lang: r.Lang, Slug: r.Parent}))
			for _, f := range []string{"/feed.xml", "/atom.xml", "/feed.json"} {
				add("/" + r.Lang + "/projects" + f)
			}
			add("/" + r.Lang + "/")
			add("/" + r.Lang + "/stream")
			add(StreamMonth(r.Lang, r.Month))
		}
		for _, f := range r.Files {
			add(RawPath(*r, f))
		}
		if r.Series != "" {
			for _, p := range SeriesPaths(r.Lang, r.Series) {
				add(p)
			}
			for _, part := range r.Parts {
				if kind, slug, ok := strings.Cut(part, ":"); ok {
					add(ItemPath(Ref{Kind: kind, Lang: r.Lang, Slug: slug}))
				}
			}
		}
		if r.Kind == "page" && i18n.IsSupported(r.Lang) {
			// Now's archive (the list, the day of each version), About's
			// "now" card, the sitemap.
			if r.Slug == "now" {
				add("/" + r.Lang + "/now/archive")
				if r.Version != "" {
					add("/" + r.Lang + "/now/archive/" + r.Version)
				}
				add("/" + r.Lang + "/about")
			}
			add("/sitemap.xml")
		}
		// The same slug under the other languages serves the "not translated
		// yet" page (public.post), which renders this entry.
		for _, l := range i18n.Supported {
			if l != r.Lang && r.Kind != "buildlog" { // a log is always in its project's language
				o := *r
				o.Lang = l
				add(ItemPath(o))
			}
		}
		if seg, ok := listPath[r.Kind]; ok && i18n.IsSupported(r.Lang) {
			l := "/" + r.Lang
			add(l + "/")
			add(l + "/stream")
			add(StreamMonth(r.Lang, r.Month))
			add(l + "/" + seg)
			if r.Kind == "post" {
				add(l + "/feed.xml")
				add(l + "/atom.xml")
				add(l + "/feed.json")
			} else {
				add(l + "/" + seg + "/feed.xml")
				add(l + "/" + seg + "/atom.xml")
				add(l + "/" + seg + "/feed.json")
			}
			for _, t := range r.Tags {
				if content.ValidSlug(t) {
					add(l + "/tags/" + t)
					add(l + "/tags/" + t + "/feed.xml")
				}
			}
			add("/sitemap.xml")
			if r.Kind == "project" && r.Homelab {
				add(l + "/homelab")
			}
			if r.Kind == "project" || r.Kind == "presentation" {
				add(l + "/about") // its cards count projects, builds and decks
			}
			if r.Kind == "link" && len(r.Categories) > 0 {
				add(l + "/awesome") // the index counts the category's links
				for _, c := range r.Categories {
					if content.ValidSlug(c) {
						add(l + "/awesome/" + c)
					}
				}
			}
		}
	}
	for _, s := range c.Siblings {
		add(ItemPath(s))
	}
	for _, s := range c.Linked {
		add(ItemPath(s))
	}
	for _, s := range c.Peers {
		add(ItemPath(s))
	}
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

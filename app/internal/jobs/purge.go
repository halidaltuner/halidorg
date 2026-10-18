package jobs

import (
	"net/url"
	"sort"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
)

// Ref locates an entry's public page: what it is and where it lives.
type Ref struct {
	Kind   string   `json:"kind"`
	Lang   string   `json:"lang"`
	Slug   string   `json:"slug"`
	Tags   []string `json:"tags,omitempty"`
	Files  []string `json:"files,omitempty"` // snippet filenames: their raw pages
	Listed bool     `json:"listed"`          // published and public: appears in lists, feeds, sitemap
}

// Change is one entry change: its state now (nil when deleted), its state
// before the change (nil when new), and its translations, whose pages link
// to it through hreflang and the language switch.
type Change struct {
	Now      *Ref
	Prev     *Ref
	Siblings []Ref
	Linked   []Ref // entries it links to, before or after: their backlinks change
	// Peers are pages that list it next to their own content: every snippet
	// page shows the snippet list (design/Snippets.dc.html).
	Peers []Ref
}

// kindPath is the URL segment of the kinds that have a page per entry
// (CONTENT_MODEL.md §5).
var kindPath = map[string]string{
	"post":    "posts",
	"note":    "notes",
	"snippet": "snippets",
}

// listPath is the URL segment of each kind's list page and per-kind feeds.
// Links and favs have no item pages: their list is where they live.
var listPath = map[string]string{
	"post":    "posts",
	"note":    "notes",
	"snippet": "snippets",
	"link":    "links",
	"fav":     "favs",
}

// ListSegment is the URL segment of a kind's list page, or "".
func ListSegment(kind string) string { return listPath[kind] }

// ItemSegment is the URL segment of a kind's item pages, or "" when the kind
// has none.
func ItemSegment(kind string) string { return kindPath[kind] }

// ItemPath is the public path of one entry, or "" for kinds without pages yet.
func ItemPath(r Ref) string {
	seg, ok := kindPath[r.Kind]
	if !ok || r.Slug == "" || !i18n.IsSupported(r.Lang) {
		return ""
	}
	return "/" + r.Lang + "/" + seg + "/" + r.Slug
}

// RawPath is the public path of one snippet file as plain text (escaped).
func RawPath(lang, slug, filename string) string {
	return "/" + lang + "/snippets/" + slug + "/raw/" + url.PathEscape(filename)
}

// Paths returns every public path whose HTML or feed can change with c,
// sorted and without duplicates. The purge job re-renders each one; a path
// that no longer renders is deleted from KV.
//
// For an entry of kind K in lang L: its page (kinds with item pages), the
// same slug in the other languages (the untranslated fallback), K's list
// page and K's feeds, a snippet's raw files, home (the stream), each tag page and tag feed (old and
// new tags), the language feeds (posts), the sitemap, and the pages of its
// translations. A slug change also covers the old URL.
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
		if r.Kind == "snippet" && ItemPath(*r) != "" {
			for _, f := range r.Files {
				add(RawPath(r.Lang, r.Slug, f))
			}
		}
		// The same slug under the other languages serves the "not translated
		// yet" page (public.post), which renders this entry.
		for _, l := range i18n.Supported {
			if l != r.Lang {
				o := *r
				o.Lang = l
				add(ItemPath(o))
			}
		}
		if seg, ok := listPath[r.Kind]; ok && i18n.IsSupported(r.Lang) {
			l := "/" + r.Lang
			add(l + "/")
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

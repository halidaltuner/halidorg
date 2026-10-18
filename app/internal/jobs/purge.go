package jobs

import (
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
	Listed bool     `json:"listed"` // published and public: appears in lists, feeds, sitemap
}

// Change is one entry change: its state now (nil when deleted), its state
// before the change (nil when new), and its translations, whose pages link
// to it through hreflang and the language switch.
type Change struct {
	Now      *Ref
	Prev     *Ref
	Siblings []Ref
}

// kindPath is the URL segment of a kind's pages (CONTENT_MODEL.md §5).
var kindPath = map[string]string{
	"post": "posts",
}

// ItemPath is the public path of one entry, or "" for kinds without pages yet.
func ItemPath(r Ref) string {
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
// For a post in lang L: its page, the post list, home, each tag page and tag
// feed (old and new tags), the language feeds, the sitemap, and the pages of
// its translations. A slug change also covers the old URL.
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
		if seg, ok := kindPath[r.Kind]; ok && i18n.IsSupported(r.Lang) {
			l := "/" + r.Lang
			add(l + "/")
			add(l + "/" + seg)
			add(l + "/feed.xml")
			add(l + "/atom.xml")
			add(l + "/feed.json")
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
	out := make([]string, 0, len(set))
	for p := range set {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

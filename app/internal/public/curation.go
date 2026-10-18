package public

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/fav"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/i18n"
)

// filterChip is one filter of a list page. Href carries the query for the
// no-JS path; with JS the page filters in place, so the KV copy (no query
// string) serves every filter.
type filterChip struct {
	Key, Label, Href string
	Current          bool
}

func chips(lang, path, param, current string, keys []string, label func(string) string) []filterChip {
	out := make([]filterChip, 0, len(keys)+1)
	out = append(out, filterChip{Key: "", Label: i18n.T(lang, "list.filter_all"), Href: path, Current: current == ""})
	for _, k := range keys {
		out = append(out, filterChip{Key: k, Label: label(k), Href: path + "?" + param + "=" + k, Current: current == k})
	}
	return out
}

// --- Links (design/Links.dc.html) -------------------------------------------

type linkRow struct {
	ID, Title, URL, Domain, Comment, Favicon, Initial string
	Read                                              bool
}

type linkGroup struct {
	Name, Href, Tone string // Tone is empty for "other"
	Items            []linkRow
}

// catTone is a category's tone, else its parent's, else navy.
func catTone(c content.Category, byID map[string]content.Category) string {
	if c.Tone != nil && *c.Tone != "" {
		return *c.Tone
	}
	if c.ParentID != nil {
		if p, ok := byID[*c.ParentID]; ok && p.Tone != nil && *p.Tone != "" {
			return *p.Tone
		}
	}
	return "navy"
}

type linksPage struct {
	page
	Status string // server-side filter (no-JS path); "" on the cached page
	Chips  []filterChip
	Groups []linkGroup
}

func initial(domain string) string {
	r, _ := utf8.DecodeRuneInString(domain)
	if r == utf8.RuneError {
		return "↗"
	}
	return string(r)
}

// links serves /{lang}/links, grouped by category in category order, links
// without one last. ?status=read|unread filters on the server for visitors
// without JS.
func (h *Handlers) links(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	status := r.URL.Query().Get("status")
	if status != link.Read && status != link.Unread {
		status = ""
	}
	list, err := h.linkRepo.List(ctx, link.ListFilter{Lang: lang, ReadStatus: status})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	cats, err := content.Categories(ctx, h.db, link.Kind)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := linksPage{page: h.newPage(lang, "links", i18n.T(lang, "nav.links")+" · halid.org", "/links", nil), Status: status}
	p.Head.Description = i18n.T(lang, "links.intro")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.links"), Href: "/" + lang + "/links/feed.xml"}}
	p.Head.Scripts = []string{"/static/js/filter.js", "/static/js/links.js"}
	p.Chips = chips(lang, "/"+lang+"/links", "status", status, []string{link.Unread, link.Read}, func(k string) string { return i18n.T(lang, "links."+k) })

	byID := map[string]content.Category{}
	for _, c := range cats {
		byID[c.ID] = c
	}
	byCat := map[string][]linkRow{}
	for _, l := range list {
		row := linkRow{ID: l.ID, Title: l.Title, URL: l.URL, Domain: l.Domain, Comment: l.Summary, Initial: initial(l.Domain), Read: l.ReadStatus == link.Read}
		if strings.HasPrefix(l.FaviconURL, "/favicons/") {
			row.Favicon = l.FaviconURL
		}
		byCat[catKey(l.CategoryID)] = append(byCat[catKey(l.CategoryID)], row)
	}
	for _, c := range cats {
		if items := byCat[c.ID]; len(items) > 0 {
			p.Groups = append(p.Groups, linkGroup{Name: c.Name(lang), Href: "/" + lang + "/awesome/" + c.Slug, Tone: catTone(c, byID), Items: items})
			delete(byCat, c.ID)
		}
	}
	// No category, or one that is gone: last, under "other", newest first.
	var rest []linkRow
	for _, items := range byCat {
		rest = append(rest, items...)
	}
	if len(rest) > 0 {
		order := map[string]int{}
		for i, l := range list {
			order[l.ID] = i
		}
		slices.SortFunc(rest, func(a, b linkRow) int { return order[a.ID] - order[b.ID] })
		p.Groups = append(p.Groups, linkGroup{Name: i18n.T(lang, "links.other"), Items: rest})
	}
	h.render(w, r, "links", p)
}

func catKey(id *string) string {
	if id == nil {
		return ""
	}
	return *id
}

// --- Favs (design/Favs.dc.html) ---------------------------------------------

type favCard struct {
	Slug, Type, TypeLabel, Title, Author, URL, Rating string
	Squares                                           []bool
}

type favsPage struct {
	page
	Type  string // server-side filter (no-JS path)
	Chips []filterChip
	Favs  []favCard
}

// favs serves /{lang}/favs. The chips offer the types that have favs;
// ?type= filters on the server for visitors without JS.
func (h *Handlers) favs(w http.ResponseWriter, r *http.Request, lang string) {
	all, err := h.favRepo.List(r.Context(), lang, "")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	typ := r.URL.Query().Get("type")
	if !slices.Contains(fav.Types, typ) {
		typ = ""
	}
	present := map[string]bool{}
	for _, f := range all {
		present[f.Type] = true
	}
	var types []string
	for _, t := range fav.Types {
		if present[t] {
			types = append(types, t)
		}
	}
	p := favsPage{page: h.newPage(lang, "favs", i18n.T(lang, "nav.favs")+" · halid.org", "/favs", nil), Type: typ}
	p.Head.Description = i18n.T(lang, "favs.intro")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.favs"), Href: "/" + lang + "/favs/feed.xml"}}
	p.Head.Scripts = []string{"/static/js/filter.js"}
	label := func(t string) string { return i18n.T(lang, "favs.type."+t) }
	p.Chips = chips(lang, "/"+lang+"/favs", "type", typ, types, label)
	for _, f := range all {
		if typ != "" && f.Type != typ {
			continue
		}
		c := favCard{Slug: f.Slug, Type: f.Type, TypeLabel: label(f.Type), Title: f.Title, Author: f.Author, URL: f.URL}
		if f.Rating != nil {
			c.Rating = strings.Replace(i18n.T(lang, "favs.rating"), "%d", strconv.Itoa(*f.Rating), 1)
			for i := 1; i <= 5; i++ {
				c.Squares = append(c.Squares, i <= *f.Rating)
			}
		}
		p.Favs = append(p.Favs, c)
	}
	h.render(w, r, "favs", p)
}

// --- Awesome lists (derive: a link category as a list page) -----------------

type awesomeSection struct {
	ID, Name string
	Items    []linkRow
}

type awesomePage struct {
	page
	Name, Tone string
	Sections   []awesomeSection
	Count      int
}

// awesome serves /{lang}/awesome/{category}: the published links of a link
// category, its direct links first, then one section per child category.
// An empty list is a 404, so the purge drops it from KV.
func (h *Handlers) awesome(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	slug := r.PathValue("category")
	if !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	cats, err := content.Categories(ctx, h.db, link.Kind)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var top *content.Category
	for i := range cats {
		if cats[i].Slug == slug {
			top = &cats[i]
		}
	}
	if top == nil {
		h.notFound(w, r)
		return
	}
	sections := []awesomeSection{{ID: top.Slug, Name: i18n.T(lang, "awesome.general")}}
	index := map[string]int{top.ID: 0}
	for _, c := range cats {
		if c.ParentID != nil && *c.ParentID == top.ID {
			index[c.ID] = len(sections)
			sections = append(sections, awesomeSection{ID: c.Slug, Name: c.Name(lang)})
		}
	}
	list, err := h.linkRepo.List(ctx, link.ListFilter{Lang: lang})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	n := 0
	for _, l := range list {
		i, ok := index[catKey(l.CategoryID)]
		if !ok || l.CategoryID == nil {
			continue
		}
		sections[i].Items = append(sections[i].Items, linkRow{ID: l.ID, Title: l.Title, URL: l.URL, Domain: l.Domain, Comment: l.Summary})
		n++
	}
	if n == 0 {
		h.notFound(w, r)
		return
	}
	p := awesomePage{page: h.newPage(lang, "links", "awesome "+top.Name(lang)+" · halid.org", "/awesome/"+top.Slug, nil),
		Name: top.Name(lang), Tone: "navy", Count: n}
	if top.Tone != nil {
		p.Tone = *top.Tone
	}
	p.Head.Description = strings.Replace(i18n.T(lang, "awesome.intro"), "%s", top.Name(lang), 1)
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.links"), Href: "/" + lang + "/links/feed.xml"}}
	for _, s := range sections {
		if len(s.Items) > 0 {
			p.Sections = append(p.Sections, s)
		}
	}
	h.render(w, r, "awesome", p)
}

type awesomeEntry struct {
	Name, Href, CountLabel, Tone string
	Count                        int
	Parent                       *awesomeEntry // set for child categories
}

type awesomeIndexPage struct {
	page
	Lists []awesomeEntry
}

// awesomeIndex serves /{lang}/awesome: every link category with links in
// lang is a list, in category order, children too. A parent counts its
// children's links, as its page lists them; a child names its parent.
func (h *Handlers) awesomeIndex(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	cats, err := content.Categories(ctx, h.db, link.Kind)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	list, err := h.linkRepo.List(ctx, link.ListFilter{Lang: lang})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	count := map[string]int{}
	byID := map[string]content.Category{}
	for _, c := range cats {
		byID[c.ID] = c
	}
	for _, l := range list {
		if l.CategoryID == nil {
			continue
		}
		c, ok := byID[*l.CategoryID]
		if !ok {
			continue
		}
		count[c.ID]++
		if c.ParentID != nil {
			count[*c.ParentID]++
		}
	}
	entry := func(c content.Category) awesomeEntry {
		e := awesomeEntry{Name: c.Name(lang), Href: "/" + lang + "/awesome/" + c.Slug, Tone: catTone(c, byID), Count: count[c.ID]}
		key := "awesome.count"
		if e.Count == 1 {
			key = "awesome.count_one"
		}
		e.CountLabel = strings.Replace(i18n.T(lang, key), "%d", strconv.Itoa(e.Count), 1)
		return e
	}
	p := awesomeIndexPage{page: h.newPage(lang, "links", "awesome · halid.org", "/awesome", nil)}
	p.Head.Description = i18n.T(lang, "awesome.index_intro")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.links"), Href: "/" + lang + "/links/feed.xml"}}
	for _, c := range cats {
		if count[c.ID] == 0 {
			continue
		}
		e := entry(c)
		if c.ParentID != nil {
			if parent, ok := byID[*c.ParentID]; ok {
				pe := entry(parent)
				e.Parent = &pe
			}
		}
		p.Lists = append(p.Lists, e)
	}
	h.render(w, r, "awesome-index", p)
}

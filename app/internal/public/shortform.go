package public

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/render/og"
)

// entryTitle is what lists and heads show: the title, else the summary
// (untitled notes), else the slug.
func entryTitle(e content.Entry) string {
	switch {
	case e.Title != "":
		return e.Title
	case e.Summary != "":
		return e.Summary
	}
	return e.Slug
}

// entryCard is the og:image of an entry with an item page (render/og).
func (h *Handlers) entryCard(e content.Entry, seg string) string {
	if e.PublishedAt == nil {
		return ""
	}
	c := og.Entry(e.Kind, e.Lang, e.Title, e.Summary, e.Slug, *e.PublishedAt)
	return h.origin + og.URL(og.EntryKey(e.Lang, seg, e.Slug), c)
}

func (h *Handlers) tagNames(r *http.Request, lang string, lists ...[]string) (map[string]string, error) {
	var all []string
	for _, l := range lists {
		all = append(all, l...)
	}
	return h.posts.TagNames(r.Context(), lang, all)
}

// onlyIn is the language switch of a page that exists in lang only: the
// other languages land on their list page (seg).
func onlyIn(lang, path, list string) func(string) string {
	return func(l string) string {
		if l == lang {
			return "/" + l + path
		}
		return "/" + l + list
	}
}

// --- Snippets (design/Snippets.dc.html) -------------------------------------

type snippetRow struct {
	Href, Title, Meta string
	Current           bool
}

type snippetFile struct {
	ID, Name, Lang, Raw string
	Code                template.HTML
}

type snippetView struct {
	Title string
	Desc  template.HTML
	Date  string
	Tags  []tagLink
	Files []snippetFile
}

type snippetsPage struct {
	page
	List []snippetRow
	Cur  *snippetView
}

func firstLang(files []snippet.File) string {
	if len(files) == 0 {
		return ""
	}
	if files[0].Language != "" {
		return files[0].Language
	}
	return "text"
}

// snippets serves /{lang}/snippets (slug "") and /{lang}/snippets/{slug}:
// the list with one snippet open, the newest on the list page.
func (h *Handlers) snippets(w http.ResponseWriter, r *http.Request, lang string) {
	slug := r.PathValue("slug")
	if slug != "" && !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	items, err := h.snippetRepo.Feed(r.Context(), lang, "", 100)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var cur *snippet.Snippet
	var html string
	if slug != "" {
		s, err := h.snippetRepo.GetPublished(r.Context(), lang, slug)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if html, err = h.snippetRepo.PublishedHTML(r.Context(), s); err != nil {
			h.fail(w, r, err)
			return
		}
		cur = &s
	} else if len(items) > 0 {
		cur, html = &items[0].Snippet, items[0].HTML
	}
	p := snippetsPage{}
	for _, it := range items {
		p.List = append(p.List, snippetRow{
			Href: "/" + lang + "/snippets/" + it.Slug, Title: entryTitle(it.Entry),
			Meta:    strings.TrimSuffix(date(it.PublishedAt)+" · "+firstLang(it.Files), " · "),
			Current: cur != nil && it.ID == cur.ID,
		})
	}
	p.page = h.newPage(lang, "snippets", i18n.T(lang, "snippets.title")+" · halid.org", "/snippets", nil)
	if slug != "" {
		path := "/snippets/" + slug
		p.page = h.newPage(lang, "snippets", entryTitle(cur.Entry)+" · halid.org", path, onlyIn(lang, path, "/snippets"))
		p.Head.Alternates = nil // one language only
	}
	p.Head.Description = i18n.T(lang, "snippets.lead")
	if cur != nil {
		var tags []string
		for _, it := range items {
			if it.ID == cur.ID {
				tags = it.Tags
			}
		}
		if slug != "" {
			tags = tagsOf(r, h.db, cur.ID)
		}
		names, err := h.tagNames(r, lang, tags)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		v := &snippetView{Title: entryTitle(cur.Entry), Desc: template.HTML(html), Date: date(cur.PublishedAt), Tags: tagLinks(lang, tags, names)}
		for i, f := range cur.Files {
			v.Files = append(v.Files, snippetFile{
				ID: "f" + strconv.Itoa(i), Name: f.Filename, Lang: firstLang([]snippet.File{f}),
				Raw:  jobs.RawPath(lang, cur.Slug, f.Filename),
				Code: template.HTML(render.Highlight(f.Language, f.Code, true)), // chroma output, escaped
			})
		}
		p.Cur = v
		if slug != "" {
			p.Head.OGType = "article"
			if cur.Summary != "" {
				p.Head.Description = cur.Summary
			}
			if img := h.entryCard(cur.Entry, "snippets"); img != "" {
				p.Head.Image = img
			}
		}
	}
	p.Head.Scripts = []string{"/static/js/code.js"}
	p.Head.Feed = "snippets"
	h.render(w, r, "snippets", p)
}

// snippetRaw serves one file of a published snippet as plain text. It is
// public so the Worker keeps it in KV; jobs.Paths purges it with the snippet.
func (h *Handlers) snippetRaw(w http.ResponseWriter, r *http.Request, lang string) {
	slug, name := r.PathValue("slug"), r.PathValue("file")
	if !content.ValidSlug(slug) || name == "" {
		h.notFound(w, r)
		return
	}
	s, err := h.snippetRepo.GetPublished(r.Context(), lang, slug)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, f := range s.Files {
		if f.Filename == name {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.Header().Set("Cache-Control", "public, max-age=0, must-revalidate")
			_, _ = w.Write([]byte(f.Code))
			return
		}
	}
	h.notFound(w, r)
}

// --- Notes (derive: list and permalink) -------------------------------------

type noteView struct {
	Href, Title, Date, Source, SourceDomain string
	Titled, TIL                             bool
	Body                                    template.HTML
	Tags                                    []tagLink
}

type notesPage struct {
	page
	Notes []noteView
}

type notePage struct {
	page
	ArticleLang string
	Note        noteView
	Backlinks   []backlink
}

func noteOf(lang string, n note.Note, html string, tags []tagLink) noteView {
	v := noteView{
		Href: "/" + n.Lang + "/notes/" + n.Slug, Title: entryTitle(n.Entry), Titled: n.Title != "",
		Date: date(n.PublishedAt), TIL: n.TIL, Body: template.HTML(html), Tags: tags, // renderer output (ADR-002)
	}
	if n.SourceURL != "" {
		v.Source, v.SourceDomain = n.SourceURL, content.Domain(n.SourceURL)
	}
	return v
}

func (h *Handlers) notes(w http.ResponseWriter, r *http.Request, lang string) {
	items, err := h.noteRepo.Feed(r.Context(), lang, "", 100)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var all [][]string
	for _, n := range items {
		all = append(all, n.Tags)
	}
	names, err := h.tagNames(r, lang, all...)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := notesPage{page: h.newPage(lang, "notes", i18n.T(lang, "notes.title")+" · halid.org", "/notes", nil)}
	p.Head.Description = i18n.T(lang, "notes.lead")
	p.Head.Feed = "notes"
	for _, n := range items {
		p.Notes = append(p.Notes, noteOf(lang, n.Note, n.HTML, tagLinks(lang, n.Tags, names)))
	}
	h.render(w, r, "notes", p)
}

func (h *Handlers) note(w http.ResponseWriter, r *http.Request, lang string) {
	slug := r.PathValue("slug")
	if !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	n, err := h.noteRepo.GetPublished(r.Context(), lang, slug)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	html, err := h.noteRepo.PublishedHTML(r.Context(), n)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	names, err := h.posts.TagNames(r.Context(), lang, tagsOf(r, h.db, n.ID))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	bls, err := content.Backlinks(r.Context(), h.db, n.ID, true)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := notePage{
		page:        h.newPage(lang, "notes", entryTitle(n.Entry)+" · halid.org", "/notes/"+slug, onlyIn(lang, "/notes/"+slug, "/notes")),
		ArticleLang: n.Lang,
		Note:        noteOf(lang, n, html, tagLinks(lang, sortedKeys(names), names)),
	}
	p.Head.Alternates = nil // one language only
	p.Head.Feed = "notes"
	for _, b := range bls {
		if seg, ok := sections[b.Kind]; ok {
			p.Backlinks = append(p.Backlinks, backlink{Href: "/" + b.Lang + "/" + seg + "/" + b.Slug, Title: b.Title, Context: b.Context, Lang: b.Lang, Glyph: glyphs[b.Kind]})
		}
	}
	p.Head.Description = n.Summary
	p.Head.OGType = "article"
	if img := h.entryCard(n.Entry, "notes"); img != "" {
		p.Head.Image = img
	}
	h.render(w, r, "note", p)
}

// --- Tag pages: the short kinds next to the posts ----------------------------

// taggedShort lists published notes and snippets in lang tagged tag, as
// stream rows (home.html).
func (h *Handlers) taggedShort(r *http.Request, lang, tag string) ([]streamItem, error) {
	ns, err := h.noteRepo.Feed(r.Context(), lang, tag, 50)
	if err != nil {
		return nil, err
	}
	ss, err := h.snippetRepo.Feed(r.Context(), lang, tag, 50)
	if err != nil {
		return nil, err
	}
	type row struct {
		at int64
		it streamItem
	}
	var rows []row
	for _, n := range ns {
		rows = append(rows, row{deref(n.PublishedAt), streamItem{Kind: "note", Glyph: glyphs["note"], Tone: kindTones["note"], Title: entryTitle(n.Entry), Href: "/" + lang + "/notes/" + n.Slug, Meta: date(n.PublishedAt)}})
	}
	for _, s := range ss {
		rows = append(rows, row{deref(s.PublishedAt), streamItem{Kind: "snippet", Glyph: glyphs["snippet"], Tone: kindTones["snippet"], Title: entryTitle(s.Entry), Href: "/" + lang + "/snippets/" + s.Slug, Meta: date(s.PublishedAt)}})
	}
	// Newest first across both kinds.
	for i := 1; i < len(rows); i++ {
		for j := i; j > 0 && rows[j].at > rows[j-1].at; j-- {
			rows[j], rows[j-1] = rows[j-1], rows[j]
		}
	}
	out := make([]streamItem, len(rows))
	for i, x := range rows {
		out[i] = x.it
	}
	return out, nil
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}

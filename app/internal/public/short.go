package public

import (
	"context"
	"errors"
	"html/template"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/render/og"
	"halid.org/engine/internal/store"
)

// snippetListSize is how many snippets the list on snippet pages shows.
// jobs.peers re-renders the same number of pages when the list changes.
const snippetListSize = 50

// findPublished loads slug in lang, else the same slug in another language
// (the "not translated yet" fallback). untranslated reports the fallback.
func findPublished[T any](lang, slug string, get func(lang, slug string) (T, error)) (v T, untranslated bool, err error) {
	v, err = get(lang, slug)
	if !errors.Is(err, content.ErrNotFound) {
		return v, false, err
	}
	for _, l := range i18n.Supported {
		if l == lang {
			continue
		}
		if v, err = get(l, slug); err == nil {
			return v, true, nil
		} else if !errors.Is(err, content.ErrNotFound) {
			return v, false, err
		}
	}
	return v, false, content.NotFound("entry", lang+"/"+slug)
}

// itemPage builds the chrome and head of an entry page under /{lang}/{seg}/:
// language switch and hreflang from its translations, canonical to the
// entry, noindex on the untranslated fallback. It answers false after
// redirecting to a translation that lives under another slug.
func (h *Handlers) itemPage(w http.ResponseWriter, r *http.Request, lang, seg, active, title string, e content.Entry, untranslated bool) (page, string, bool) {
	trs, err := content.Translations(r.Context(), h.db, e)
	if err != nil {
		h.fail(w, r, err)
		return page{}, "", false
	}
	where := map[string]string{e.Lang: "/" + e.Lang + "/" + seg + "/" + e.Slug}
	for _, t := range trs {
		where[t.Lang] = "/" + t.Lang + "/" + seg + "/" + t.Slug
	}
	if u, ok := where[lang]; untranslated && ok {
		w.Header().Set("Cache-Control", "no-store")
		http.Redirect(w, r, u, http.StatusMovedPermanently)
		return page{}, "", false
	}
	switchTo := func(l string) string {
		if u, ok := where[l]; ok {
			return u
		}
		return "/" + l + "/" + seg + "/" + e.Slug
	}
	p := h.newPage(lang, active, title+" · halid.org", "", switchTo)
	p.Head.Description = e.Summary
	p.Head.OGType = "article"
	p.Head.Canonical = h.origin + where[e.Lang]
	p.Head.Alternates = nil
	for _, l := range i18n.Supported {
		if u, ok := where[l]; ok {
			p.Head.Alternates = append(p.Head.Alternates, alternate{Lang: l, Href: h.origin + u})
		}
	}
	p.Head.Noindex = untranslated
	if e.PublishedAt != nil {
		card := og.Entry(e.Kind, e.Lang, e.Title, e.Summary, e.Slug, *e.PublishedAt)
		p.Head.Image = h.origin + og.URL(og.EntryKey(e.Lang, seg, e.Slug), card)
	}
	return p, where[e.Lang], true
}

// tagSlugs returns the tag slugs of several entries in one query.
func tagSlugs(ctx context.Context, db store.DB, ids []string) (map[string][]string, error) {
	out := map[string][]string{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	res, err := db.Query(ctx, `SELECT et.entry_id, t.slug FROM entry_tags et JOIN tags t ON t.id = et.tag_id
WHERE et.entry_id IN (?`+strings.Repeat(", ?", len(ids)-1)+`) ORDER BY t.slug`, args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		EntryID string `json:"entry_id"`
		Slug    string `json:"slug"`
	}](res)
	if err != nil {
		return nil, err
	}
	for _, x := range rows {
		out[x.EntryID] = append(out[x.EntryID], x.Slug)
	}
	return out, nil
}

// tagNames resolves slugs to names in lang, in the order given.
func (h *Handlers) tagNames(ctx context.Context, lang string, slugs []string) ([]string, error) {
	names, err := h.posts.TagNames(ctx, lang, slugs)
	if err != nil {
		return nil, err
	}
	out := make([]string, len(slugs))
	for i, s := range slugs {
		out[i] = s
		if n := names[s]; n != "" {
			out[i] = n
		}
	}
	return out, nil
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
	Lang, Title, Date string
	Tags              []string
	Desc              template.HTML
	Files             []snippetFile
}

type snippetPage struct {
	page
	List         []snippetRow
	Cur          *snippetView
	Untranslated bool
	OtherURL     string
}

// snippets serves /{lang}/snippets (the newest snippet in the viewer) and
// /{lang}/snippets/{slug}. Both show the list of the page language.
func (h *Handlers) snippets(w http.ResponseWriter, r *http.Request, lang, slug string) {
	ctx := r.Context()
	if slug != "" && !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	items, err := h.snippetRepo.List(ctx, lang, snippetListSize)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var p snippetPage
	var cur snippet.Snippet
	switch {
	case slug != "":
		var untranslated bool
		cur, untranslated, err = findPublished(lang, slug, func(l, s string) (snippet.Snippet, error) { return h.snippetRepo.GetPublished(ctx, l, s) })
		if err != nil {
			h.fail(w, r, err)
			return
		}
		pg, orig, ok := h.itemPage(w, r, lang, "snippets", "snippets", cur.Title, cur.Entry, untranslated)
		if !ok {
			return
		}
		p = snippetPage{page: pg, Untranslated: untranslated, OtherURL: orig}
	default:
		p = snippetPage{page: h.newPage(lang, "snippets", i18n.T(lang, "nav.snippets")+" · halid.org", "/snippets", nil)}
		p.Head.Description = i18n.T(lang, "snippets.intro")
		if len(items) > 0 {
			if cur, err = h.snippetRepo.GetPublished(ctx, lang, items[0].Slug); err != nil {
				h.fail(w, r, err)
				return
			}
		}
	}
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.snippets"), Href: "/" + lang + "/snippets/feed.xml"}}
	p.Head.Scripts = []string{"/static/js/snippet.js"}
	for _, it := range items {
		meta := date(it.PublishedAt)
		if it.Language != "" {
			meta += " · " + it.Language
		}
		p.List = append(p.List, snippetRow{Href: "/" + lang + "/snippets/" + it.Slug, Title: it.Title, Meta: meta,
			Current: cur.ID != "" && cur.Lang == lang && it.Slug == cur.Slug})
	}
	if cur.ID != "" {
		v, err := h.snippetView(ctx, lang, cur)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		p.Cur = &v
	}
	h.render(w, r, "snippets", p)
}

func (h *Handlers) snippetView(ctx context.Context, lang string, s snippet.Snippet) (snippetView, error) {
	tags, err := tagSlugs(ctx, h.db, []string{s.ID})
	if err != nil {
		return snippetView{}, err
	}
	names, err := h.tagNames(ctx, lang, tags[s.ID])
	if err != nil {
		return snippetView{}, err
	}
	v := snippetView{Lang: s.Lang, Title: s.Title, Date: date(s.PublishedAt), Tags: names,
		Desc: template.HTML(s.HTML)} // renderer output, stored on the revision (ADR-002)
	base := "/" + s.Lang + "/snippets/" + s.Slug + "/raw/"
	for i, f := range s.Files {
		v.Files = append(v.Files, snippetFile{ID: "f" + strconv.Itoa(i), Name: f.Filename, Lang: f.Language,
			Raw: base + url.PathEscape(f.Filename), Code: template.HTML(render.Code(f.Language, f.Code))})
	}
	return v, nil
}

// snippetRaw serves one published file as text/plain. The edge stores it
// like a page; jobs.RawPath purges it.
func (h *Handlers) snippetRaw(w http.ResponseWriter, r *http.Request, lang string) {
	slug, name := r.PathValue("slug"), r.PathValue("file")
	if !content.ValidSlug(slug) {
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
			w.Header().Set("X-Content-Type-Options", "nosniff")
			writeCacheableType(w, r, "text/plain; charset=utf-8", []byte(f.Code))
			return
		}
	}
	h.notFound(w, r)
}

// --- Notes (derive: indigo tone, DESIGN.md §6) ------------------------------

type noteCard struct {
	Href, Title, Time, DateTime string
	Body                        template.HTML
	Tags                        []string
	TIL                         bool
	Source, SourceDomain        string
}

type notesPage struct {
	page
	Notes []noteCard
}

type notePage struct {
	page
	ArticleLang  string
	Note         noteCard
	Untranslated bool
	OtherURL     string
}

func (h *Handlers) noteCards(ctx context.Context, lang string, notes []note.Note) ([]noteCard, error) {
	ids := make([]string, len(notes))
	for i, n := range notes {
		ids[i] = n.ID
	}
	tags, err := tagSlugs(ctx, h.db, ids)
	if err != nil {
		return nil, err
	}
	out := make([]noteCard, len(notes))
	for i, n := range notes {
		names, err := h.tagNames(ctx, lang, tags[n.ID])
		if err != nil {
			return nil, err
		}
		c := noteCard{Href: "/" + n.Lang + "/notes/" + n.Slug, Title: n.Title, Tags: names, TIL: n.TIL,
			Body: template.HTML(n.HTML)} // renderer output (ADR-002)
		if n.PublishedAt != nil {
			t := time.Unix(*n.PublishedAt, 0).UTC()
			c.Time, c.DateTime = t.Format("2006-01-02 15:04"), t.Format(time.RFC3339)
		}
		if n.SourceURL != "" {
			c.Source, c.SourceDomain = n.SourceURL, content.Domain(n.SourceURL)
		}
		out[i] = c
	}
	return out, nil
}

func (h *Handlers) notes(w http.ResponseWriter, r *http.Request, lang string) {
	list, err := h.noteRepo.List(r.Context(), lang, 50)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	cards, err := h.noteCards(r.Context(), lang, list)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := notesPage{page: h.newPage(lang, "notes", i18n.T(lang, "notes.title")+" · halid.org", "/notes", nil), Notes: cards}
	p.Head.Description = i18n.T(lang, "notes.intro")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.notes"), Href: "/" + lang + "/notes/feed.xml"}}
	h.render(w, r, "notes", p)
}

func (h *Handlers) note(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	if !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	n, untranslated, err := findPublished(lang, slug, func(l, s string) (note.Note, error) { return h.noteRepo.GetPublished(ctx, l, s) })
	if err != nil {
		h.fail(w, r, err)
		return
	}
	cards, err := h.noteCards(ctx, lang, []note.Note{n})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	title := n.Title
	if title == "" {
		title = n.Summary
	}
	if title == "" {
		title = i18n.T(lang, "notes.one") + " · " + cards[0].Time
	}
	pg, orig, ok := h.itemPage(w, r, lang, "notes", "notes", title, n.Entry, untranslated)
	if !ok {
		return
	}
	p := notePage{page: pg, ArticleLang: n.Lang, Note: cards[0], Untranslated: untranslated, OtherURL: orig}
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.notes"), Href: "/" + lang + "/notes/feed.xml"}}
	h.render(w, r, "note", p)
}

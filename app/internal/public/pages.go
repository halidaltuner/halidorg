package public

import (
	"errors"
	"html/template"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/render"
	"halid.org/engine/internal/render/og"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/ui"
)

// card is one post card (post/card.html).
type card struct {
	Href    string
	Title   string
	Summary string
	Date    string
	Minutes string
	Tone    string
	Tags    []tagLink
}

type tagLink struct {
	Slug, Name, Href string
}

func date(sec *int64) string {
	if sec == nil {
		return ""
	}
	return time.Unix(*sec, 0).UTC().Format("2006-01-02")
}

func tone(e content.Entry) string {
	if e.Tone != nil {
		return *e.Tone
	}
	return "cyan"
}

func (h *Handlers) cards(r *http.Request, lang string, items []post.Published) ([]card, error) {
	var slugs []string
	for _, p := range items {
		slugs = append(slugs, p.Tags...)
	}
	names, err := h.posts.TagNames(r.Context(), lang, slugs)
	if err != nil {
		return nil, err
	}
	out := make([]card, len(items))
	for i, p := range items {
		out[i] = card{Href: "/" + lang + "/posts/" + p.Slug, Title: p.Title, Summary: p.Summary,
			Date: date(p.PublishedAt), Minutes: minutes(lang, p.ReadingMinutes), Tone: tone(p.Entry), Tags: tagLinks(lang, p.Tags, names)}
	}
	return out, nil
}

func tagLinks(lang string, slugs []string, names map[string]string) []tagLink {
	out := make([]tagLink, 0, len(slugs))
	for _, s := range slugs {
		n := names[s]
		if n == "" {
			n = s
		}
		out = append(out, tagLink{Slug: s, Name: n, Href: "/" + lang + "/tags/" + s})
	}
	return out
}

// --- Home -------------------------------------------------------------------

type streamDay struct {
	Day   string
	Items []streamItem
}

type streamItem struct {
	Kind, Glyph, Title, Href, Meta string
}

type homePage struct {
	page
	Featured *card
	Latest   []card
	Stream   []streamDay
}

func (h *Handlers) home(w http.ResponseWriter, r *http.Request, lang string) {
	items, err := h.posts.Feed(r.Context(), lang, "", 20)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	cs, err := h.cards(r, lang, items)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := homePage{page: h.newPage(lang, "", "halid.org", "/", nil)}
	p.Head.Description = i18n.T(lang, "home.sub")
	if len(cs) > 0 {
		p.Featured = &cs[0]
		p.Latest = cs[:min(3, len(cs))]
	}
	// Stream: posts only until Phase 2 adds the other kinds, grouped by day.
	for _, c := range cs {
		it := streamItem{Kind: "post", Glyph: "¶", Title: c.Title, Href: c.Href, Meta: c.Minutes}
		if n := len(p.Stream); n > 0 && p.Stream[n-1].Day == c.Date {
			p.Stream[n-1].Items = append(p.Stream[n-1].Items, it)
			continue
		}
		p.Stream = append(p.Stream, streamDay{Day: c.Date, Items: []streamItem{it}})
	}
	h.render(w, r, "home", p)
}

func minutes(lang string, n int) string { return ui.MinutesShort(lang, n) }

// --- Lists ------------------------------------------------------------------

type listPage struct {
	page
	Tag   string
	Title string
	Cards []card
}

func (h *Handlers) list(w http.ResponseWriter, r *http.Request, lang, tag string) {
	items, err := h.posts.Feed(r.Context(), lang, tag, 50)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if tag != "" && len(items) == 0 {
		h.notFound(w, r)
		return
	}
	cs, err := h.cards(r, lang, items)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	title := i18n.T(lang, "list.posts")
	path := "/posts"
	if tag != "" {
		names, _ := h.posts.TagNames(r.Context(), lang, []string{tag})
		title = "#" + names[tag]
		if names[tag] == "" {
			title = "#" + tag
		}
		path = "/tags/" + tag
	}
	p := listPage{page: h.newPage(lang, "posts", title+" · halid.org", path, nil), Tag: tag, Title: title, Cards: cs}
	h.render(w, r, "list", p)
}

// --- Post -------------------------------------------------------------------

type postPage struct {
	page
	ArticleLang  string
	Post         post.Post
	Body         template.HTML
	TOC          []render.Heading
	Tags         []tagLink
	Series       *post.SeriesNav
	Translations []translationLink
	Untranslated bool
	OtherURL     string
	Tone         string
}

type translationLink struct {
	Lang, Href string
	Missing    bool
}

// post serves /{lang}/posts/{slug}. When the post exists only in another
// language it renders that post with this language's chrome and the
// "not translated yet" banner (design/Post.dc.html), canonical to the
// original and noindex.
func (h *Handlers) post(w http.ResponseWriter, r *http.Request, lang string) {
	slug := r.PathValue("slug")
	if !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	p, err := h.posts.GetPublished(r.Context(), lang, slug)
	untranslated := false
	if errors.Is(err, content.ErrNotFound) {
		p, err = h.untranslated(r, lang, slug)
		untranslated = err == nil
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	trs, err := h.posts.Translations(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if untranslated && hasLang(trs, lang) {
		// A translation exists under another slug: send readers there.
		for _, t := range trs {
			if t.Lang == lang {
				w.Header().Set("Cache-Control", "no-store")
				http.Redirect(w, r, "/"+lang+"/posts/"+t.Slug, http.StatusMovedPermanently)
				return
			}
		}
	}
	rev, err := h.posts.PublishedBody(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	names, err := h.posts.TagNames(r.Context(), lang, tagsOf(r, h.db, p.ID))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	series, err := h.posts.Series(r.Context(), p)
	if err != nil {
		h.fail(w, r, err)
		return
	}

	// Where each language lives: the post itself, its translations, or the
	// fallback page that shows the banner.
	where := map[string]string{p.Lang: "/" + p.Lang + "/posts/" + p.Slug}
	for _, t := range trs {
		where[t.Lang] = "/" + t.Lang + "/posts/" + t.Slug
	}
	switchTo := func(l string) string {
		if u, ok := where[l]; ok {
			return u
		}
		return "/" + l + "/posts/" + p.Slug
	}
	pg := postPage{
		page:         h.newPage(lang, "posts", p.Title+" · halid.org", "", switchTo),
		ArticleLang:  p.Lang,
		Post:         p,
		Body:         template.HTML(rev.BodyHTML), // renderer output, stored on the revision (ADR-002)
		Tags:         tagLinks(lang, sortedKeys(names), names),
		Series:       series,
		Untranslated: untranslated,
		OtherURL:     where[p.Lang],
		Tone:         tone(p.Entry),
	}
	pg.TOC, _ = decodeTOC(rev.TOCJSON)
	pg.Head.Description = p.Summary
	pg.Head.OGType = "article"
	if p.PublishedAt != nil {
		// The original's card, also on the untranslated fallback page.
		card := og.Post(p.Lang, p.Title, *p.PublishedAt, p.ReadingMinutes)
		pg.Head.Image = h.origin + og.URL(og.EntryKey(p.Lang, "posts", p.Slug), card)
	}
	pg.Head.Canonical = h.origin + where[p.Lang]
	pg.Head.Alternates = nil
	for _, l := range i18n.Supported {
		if u, ok := where[l]; ok {
			pg.Head.Alternates = append(pg.Head.Alternates, alternate{Lang: l, Href: h.origin + u})
		}
		if l != p.Lang {
			_, ok := where[l]
			pg.Translations = append(pg.Translations, translationLink{Lang: l, Href: switchTo(l), Missing: !ok})
		}
	}
	if untranslated {
		// The banner already says which language is missing.
		pg.Head.Noindex = true
		pg.Translations = nil
	}
	pg.Head.Scripts = []string{"/static/js/post.js"}
	h.render(w, r, "post", pg)
}

// untranslated finds the post with this slug in another language.
func (h *Handlers) untranslated(r *http.Request, lang, slug string) (post.Post, error) {
	for _, l := range i18n.Supported {
		if l == lang {
			continue
		}
		if p, err := h.posts.GetPublished(r.Context(), l, slug); err == nil {
			return p, nil
		} else if !errors.Is(err, content.ErrNotFound) {
			return post.Post{}, err
		}
	}
	return post.Post{}, content.NotFound("post", lang+"/"+slug)
}

func hasLang(trs []post.Translation, lang string) bool {
	for _, t := range trs {
		if t.Lang == lang {
			return true
		}
	}
	return false
}

func tagsOf(r *http.Request, db store.DB, id string) []string {
	tags, err := content.TagsOf(r.Context(), db, id)
	if err != nil {
		slog.Error("tags", "entry", id, "err", err)
		return nil
	}
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.Slug
	}
	return out
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

package public

import (
	"fmt"
	"html/template"
	"net/http"
	"slices"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/presentation"
	"halid.org/engine/internal/i18n"
)

// presCard is one card on /{lang}/presentations (design/Presentations).
type presCard struct {
	Href, Title, Desc, Event, Lang, Date, City, Type, Tone string
	Slides                                                 int
	Cover                                                  *image
}

type presentationsPage struct {
	page
	Chips []filterChip
	Cards []presCard
	Type  string // server-side filter (no-JS path); "" on the cached page
}

// presTone is the presentation's tone, else rose (the section's).
func presTone(p presentation.Presentation) string {
	if p.Tone != nil && *p.Tone != "" {
		return *p.Tone
	}
	return "rose"
}

func (h *Handlers) presentations(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	list, err := presentation.NewRepo(h.db).List(ctx, presentation.ListFilter{Lang: lang})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var ids []string
	for _, x := range list {
		if x.CoverMediaID != nil {
			ids = append(ids, *x.CoverMediaID)
		}
	}
	imgs, err := h.images(ctx, lang, ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := presentationsPage{page: h.newPage(lang, "presentations", i18n.T(lang, "nav.presentations")+" · halid.org", "/presentations", nil)}
	p.Head.Description = i18n.T(lang, "pres.sub")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.presentations"), Href: "/" + lang + "/presentations/feed.xml"}}
	count := map[string]int{}
	for _, x := range list {
		c := presCard{Href: "/" + lang + "/presentations/" + x.Slug, Title: x.Title, Desc: x.Summary, Event: x.Event, Lang: x.TalkLang,
			Date: presentation.Day(x.HeldOn), City: x.City, Type: x.Type, Tone: presTone(x), Slides: x.SlideCount}
		if x.CoverMediaID != nil {
			if im, ok := imgs[*x.CoverMediaID]; ok {
				if im.Alt == "" {
					im.Alt = x.Title
				}
				c.Cover = &im
			}
		}
		count[x.Type]++
		p.Cards = append(p.Cards, c)
	}
	var keys []string
	for _, t := range presentation.Types {
		if count[t] > 0 {
			keys = append(keys, t)
		}
	}
	p.Type = r.URL.Query().Get("type")
	if !slices.Contains(keys, p.Type) {
		p.Type = ""
	}
	p.Chips = chips(lang, "/"+lang+"/presentations", "type", p.Type, keys, func(k string) string {
		return i18n.T(lang, "kind.presentation.type."+k)
	})
	if p.Type != "" {
		kept := p.Cards[:0]
		for _, c := range p.Cards {
			if c.Type == p.Type {
				kept = append(kept, c)
			}
		}
		p.Cards = kept
		p.Head.Noindex = true
	}
	p.Head.Scripts = []string{"/static/js/filter.js"}
	h.render(w, r, "presentations", p)
}

// presLink is one row of the "related" list, marked with the glyph of the
// section it points at.
type presLink struct{ Glyph, Label, URL string }

// relatedGlyph is the kind mark of a link: a page here by its section,
// another site ↗.
func relatedGlyph(u string) string {
	parts := strings.Split(strings.TrimPrefix(u, "/"), "/")
	if !strings.HasPrefix(u, "/") || len(parts) < 2 {
		return "↗"
	}
	for kind, seg := range map[string]string{"post": "posts", "note": "notes", "snippet": "snippets", "project": "projects", "presentation": "presentations"} {
		if parts[1] == seg {
			return glyphs[kind]
		}
	}
	return "→"
}

type presView struct {
	Title, Meta, Tone             string
	DeckURL, DeckOrigin, Download string
	Recording, Minutes            string
	Slides                        int
	Cover                         *image
	Outline                       []presentation.Slide
	Related                       []presLink
	BackHref                      string
	Event                         string
}

type presentationPage struct {
	page
	ArticleLang  string
	Untranslated bool
	OtherURL     string
	Pres         presView
	Body         template.HTML // the published revision, from the renderer (ADR-002)
}

func (h *Handlers) presentation(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	if !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	repo := presentation.NewRepo(h.db)
	p, untranslated, err := findPublished(lang, slug, func(l, s string) (presentation.Presentation, error) { return repo.GetPublished(ctx, l, s) })
	if err != nil {
		h.fail(w, r, err)
		return
	}
	pg, orig, ok := h.itemPage(w, r, lang, "presentations", "presentations", p.Title, p.Entry, untranslated)
	if !ok {
		return
	}
	pg.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.presentations"), Href: "/" + lang + "/presentations/feed.xml"}}
	var meta []string
	for _, s := range []string{p.Event, presentation.Day(p.HeldOn), p.City, p.TalkLang} {
		if s != "" {
			meta = append(meta, s)
		}
	}
	v := presView{Title: p.Title, Meta: strings.Join(meta, " · "), Tone: presTone(p), Slides: p.SlideCount, Outline: p.Outline,
		DeckURL: p.DeckURL(), DeckOrigin: presentation.DecksOrigin(), Download: p.DownloadURL(), Event: p.Event,
		BackHref: "/" + lang + "/presentations"}
	if p.RecordingURL != nil {
		v.Recording = *p.RecordingURL
		if p.RecordingMinutes != nil {
			v.Minutes = fmt.Sprintf(i18n.T(lang, "pres.minutes"), *p.RecordingMinutes)
		}
	}
	for _, l := range p.Links {
		v.Related = append(v.Related, presLink{Glyph: relatedGlyph(l.URL), Label: l.Label, URL: l.URL})
	}
	if p.CoverMediaID != nil {
		imgs, err := h.images(ctx, lang, []string{*p.CoverMediaID})
		if err != nil {
			h.fail(w, r, err)
			return
		}
		if im, ok := imgs[*p.CoverMediaID]; ok {
			if im.Alt == "" {
				im.Alt = p.Title
			}
			v.Cover = &im
		}
	}
	if v.DeckURL != "" {
		pg.Head.Scripts = []string{"/static/js/deck.js"}
	}
	h.render(w, r, "presentation", presentationPage{page: pg, ArticleLang: p.Lang, Untranslated: untranslated, OtherURL: orig,
		Pres: v, Body: template.HTML(p.HTML)})
}

package public

import (
	"html/template"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/book"
	"halid.org/engine/internal/content/term"
	"halid.org/engine/internal/i18n"
)

// glossaryRow is one term on /{lang}/glossary.
type glossaryRow struct {
	Href, Term, Aliases, Def, Slug string
}

// glossaryGroup is the terms under one letter.
type glossaryGroup struct {
	Letter string
	Rows   []glossaryRow
}

type glossaryPage struct {
	page
	Groups  []glossaryGroup
	Letters []string
}

// letterOf is the index letter of a term: its first letter in lang's upper
// case, "#" for a digit or a sign.
func letterOf(lang, t string) string {
	r, _ := utf8.DecodeRuneInString(t)
	switch {
	case !unicode.IsLetter(r):
		return "#"
	case lang == "tr" && r == 'i':
		return "İ"
	}
	return string(unicode.ToUpper(r))
}

// glossary serves /{lang}/glossary (derive: the posts list in indigo): the
// terms A to Z under their letters, with a letter bar.
func (h *Handlers) glossary(w http.ResponseWriter, r *http.Request, lang string) {
	list, err := term.NewRepo(h.db).List(r.Context(), lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := glossaryPage{page: h.newPage(lang, "glossary", i18n.T(lang, "glossary.title")+" · halid.org", "/glossary", nil)}
	p.Head.Description = i18n.T(lang, "glossary.sub")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.glossary"), Href: "/" + lang + "/glossary/feed.xml"}}
	for _, t := range list {
		l := letterOf(lang, t.Title)
		if n := len(p.Groups); n == 0 || p.Groups[n-1].Letter != l {
			p.Groups = append(p.Groups, glossaryGroup{Letter: l})
			p.Letters = append(p.Letters, l)
		}
		g := &p.Groups[len(p.Groups)-1]
		g.Rows = append(g.Rows, glossaryRow{Href: "/" + lang + "/glossary/" + t.Slug, Term: t.Title, Aliases: strings.Join(t.Aliases, ", "),
			Def: t.ShortDef, Slug: t.Slug})
	}
	h.render(w, r, "glossary", p)
}

type termPage struct {
	page
	ArticleLang  string
	Untranslated bool
	OtherURL     string
	Term         term.Term
	Body         template.HTML
}

// term serves /{lang}/glossary/{slug}: the term, its other names, the short
// definition as the lead and the explanation.
func (h *Handlers) term(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	if !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	repo := term.NewRepo(h.db)
	t, untranslated, err := findPublished(lang, slug, func(l, s string) (term.Term, error) { return repo.GetPublished(ctx, l, s) })
	if err != nil {
		h.fail(w, r, err)
		return
	}
	pg, orig, ok := h.itemPage(w, r, lang, "glossary", "glossary", t.Title, t.Entry, untranslated)
	if !ok {
		return
	}
	if t.Summary == "" {
		pg.Head.Description = t.ShortDef
	}
	rev, err := content.GetRevision(ctx, h.db, *t.PublishedRevisionID)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, r, "term", termPage{page: pg, ArticleLang: t.Lang, Untranslated: untranslated, OtherURL: orig, Term: t,
		Body: template.HTML(rev.BodyHTML)}) // renderer output (ADR-002)
}

// readingBook is one book on /{lang}/reading.
type readingBook struct {
	Slug, Title, Author, Take, Started, Finished, Tone string
	Progress                                           int
	Rating                                             []bool // five stars, on or off
	Cover                                              *image
}

type readingGroup struct {
	State, Label string
	Books        []readingBook
}

type readingPage struct {
	page
	Groups []readingGroup
}

// reading serves /{lang}/reading (derive: amber): the books being read with
// their progress, then the ones wanted, read and dropped.
func (h *Handlers) reading(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	list, err := book.NewRepo(h.db).List(ctx, lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var ids []string
	for _, b := range list {
		if b.CoverMediaID != nil {
			ids = append(ids, *b.CoverMediaID)
		}
	}
	imgs, err := h.images(ctx, lang, ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := readingPage{page: h.newPage(lang, "reading", i18n.T(lang, "reading.title")+" · halid.org", "/reading", nil)}
	p.Head.Description = i18n.T(lang, "reading.sub")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.reading"), Href: "/" + lang + "/reading/feed.xml"}}
	byState := map[string][]readingBook{}
	for _, b := range list {
		x := readingBook{Slug: b.Slug, Title: b.Title, Author: b.Author, Take: b.Summary, Progress: b.Progress, Tone: "amber",
			Started: book.Day(b.StartedAt), Finished: book.Day(b.FinishedAt)}
		if b.Tone != nil {
			x.Tone = *b.Tone
		}
		if b.Rating != nil {
			x.Rating = make([]bool, 5)
			for i := range x.Rating {
				x.Rating[i] = i < *b.Rating
			}
		}
		if b.CoverMediaID != nil {
			if im, ok := imgs[*b.CoverMediaID]; ok {
				if im.Alt == "" {
					im.Alt = b.Title
				}
				x.Cover = &im
			}
		}
		byState[b.State] = append(byState[b.State], x)
	}
	for _, s := range book.Statuses {
		if len(byState[s]) > 0 {
			p.Groups = append(p.Groups, readingGroup{State: s, Label: i18n.T(lang, "reading.status."+s), Books: byState[s]})
		}
	}
	h.render(w, r, "reading", p)
}

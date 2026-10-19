package public

import (
	"fmt"
	"net/http"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/content/series"
	"halid.org/engine/internal/i18n"
)

// seriesCard is one series on /{lang}/series.
type seriesCard struct {
	Href, Title, Summary, Status, StatusKey, Count string
}

type seriesIndexPage struct {
	page
	Cards []seriesCard
}

// seriesIndex serves /{lang}/series (derive: the posts list): the series
// with a published part in lang, archived ones left out.
func (h *Handlers) seriesIndex(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	repo := series.NewRepo(h.db)
	all, err := repo.All(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	counts, err := repo.Counts(ctx, lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := seriesIndexPage{page: h.newPage(lang, "posts", i18n.T(lang, "series.title")+" · halid.org", "/series", nil)}
	p.Head.Description = i18n.T(lang, "series.sub")
	for _, s := range all {
		if s.Status == "archived" || counts[s.ID] == 0 {
			continue
		}
		p.Cards = append(p.Cards, seriesCard{Href: "/" + lang + "/series/" + s.Slug, Title: s.Title(lang), Summary: s.Summary(lang),
			Status: i18n.T(lang, "series.status."+s.Status), StatusKey: s.Status, Count: fmt.Sprintf(i18n.T(lang, "series.parts_n"), counts[s.ID])})
	}
	h.render(w, r, "series-index", p)
}

// seriesPart is one row on a series page.
type seriesPart struct {
	N                                 int
	Href, Title, Summary, Date, Glyph string
}

type seriesPage struct {
	page
	Title, Summary, Status, StatusKey string
	Parts                             []seriesPart
}

// seriesShow serves /{lang}/series/{slug}: the series and its published
// parts in lang, in order.
func (h *Handlers) seriesShow(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	if !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	repo := series.NewRepo(h.db)
	s, err := repo.BySlug(ctx, slug)
	if err == nil && s.Status == "archived" {
		err = content.NotFound("series", slug)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	parts, err := repo.Parts(ctx, s.ID, lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if len(parts) == 0 {
		h.fail(w, r, content.NotFound("series", lang+"/"+slug))
		return
	}
	p := seriesPage{page: h.newPage(lang, "posts", s.Title(lang)+" · halid.org", "/series/"+s.Slug, nil),
		Title: s.Title(lang), Summary: s.Summary(lang), Status: i18n.T(lang, "series.status."+s.Status), StatusKey: s.Status}
	p.Head.Description = s.Summary(lang)
	for i, x := range parts {
		p.Parts = append(p.Parts, seriesPart{N: i + 1, Href: "/" + x.Lang + "/" + post.Section(x.Kind) + "/" + x.Slug, Title: x.Title,
			Summary: x.Summary, Date: date(x.PublishedAt), Glyph: glyphs[x.Kind]})
	}
	h.render(w, r, "series", p)
}

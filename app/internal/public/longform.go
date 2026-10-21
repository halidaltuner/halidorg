package public

import (
	"net/http"
	"slices"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/i18n"
)

// gardenCard is one garden page on /{lang}/garden: the post card with its
// stage.
type gardenCard struct {
	card
	Maturity string // seedling, budding, evergreen
	Stage    string // its label
	Tended   string
}

type gardenPage struct {
	page
	Chips    []filterChip
	Cards    []gardenCard
	Maturity string // server-side filter (no-JS path); "" on the cached page
}

// garden serves /{lang}/garden (derive: the posts list in green): every
// published page, latest tended first, with a chip per stage.
func (h *Handlers) garden(w http.ResponseWriter, r *http.Request, lang string) {
	list, err := h.posts.List(r.Context(), post.ListFilter{Kind: post.Garden, Lang: lang, Status: "published", Limit: 100})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := gardenPage{page: h.newPage(lang, "garden", i18n.T(lang, "nav.garden")+" · halid.org", "/garden", nil)}
	p.Head.Description = i18n.T(lang, "garden.sub")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.garden"), Href: "/" + lang + "/garden/feed.xml"}}
	es := make([]content.Entry, len(list))
	for i, x := range list {
		es[i] = x.Entry
	}
	covers, err := h.covers(r.Context(), lang, es)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	count := map[string]int{}
	for _, x := range list {
		count[x.Maturity]++
		p.Cards = append(p.Cards, gardenCard{
			card: card{Href: x.Path(), Other: other(lang, x.Lang), Title: x.Title, Summary: x.Summary, Date: date(x.PublishedAt),
				Minutes: minutes(lang, x.ReadingMinutes), Tone: articleTone(x), Cover: coverOf(covers, x.Entry)},
			Maturity: x.Maturity, Stage: i18n.T(lang, "garden.maturity."+x.Maturity), Tended: date(x.LastTendedAt)})
	}
	var keys []string
	for _, m := range post.Maturities {
		if count[m] > 0 {
			keys = append(keys, m)
		}
	}
	p.Maturity = r.URL.Query().Get("stage")
	if !slices.Contains(keys, p.Maturity) {
		p.Maturity = ""
	}
	p.Chips = chips(lang, "/"+lang+"/garden", "stage", p.Maturity, keys, func(k string) string { return i18n.T(lang, "garden.maturity."+k) })
	if p.Maturity != "" {
		kept := p.Cards[:0]
		for _, c := range p.Cards {
			if c.Maturity == p.Maturity {
				kept = append(kept, c)
			}
		}
		p.Cards = kept
		p.Head.Noindex = true
	}
	p.Head.Scripts = []string{"/static/js/filter.js"}
	h.render(w, r, "garden", p)
}

// runbookRow is one runbook on /{lang}/runbooks.
type runbookRow struct {
	Href, Title, Summary, Platform, Risk, RiskLabel, Verified string
	Other                                                     string // its language, when not the page's (fallback)
}

type runbooksPage struct {
	page
	Rows []runbookRow
}

// runbooks serves /{lang}/runbooks (derive: a dense list in navy): title,
// platform, risk and when the steps last worked.
func (h *Handlers) runbooks(w http.ResponseWriter, r *http.Request, lang string) {
	list, err := h.posts.List(r.Context(), post.ListFilter{Kind: post.Runbook, Lang: lang, Status: "published", Limit: 100})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := runbooksPage{page: h.newPage(lang, "runbooks", i18n.T(lang, "nav.runbooks")+" · halid.org", "/runbooks", nil)}
	p.Head.Description = i18n.T(lang, "runbook.sub")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.runbooks"), Href: "/" + lang + "/runbooks/feed.xml"}}
	for _, x := range list {
		p.Rows = append(p.Rows, runbookRow{Href: x.Path(), Other: other(lang, x.Lang), Title: x.Title, Summary: x.Summary, Platform: x.Platform,
			Risk: x.Risk, RiskLabel: i18n.T(lang, "runbook.risk."+x.Risk), Verified: date(x.VerifiedOn)})
	}
	h.render(w, r, "runbooks", p)
}

package public

import (
	"net/http"
	"strconv"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/certificate"
	"halid.org/engine/internal/i18n"
)

// Certificates (design/Certificates.dc.html), Phase 5.2: the cyan hero
// with stat tiles, issuer chips (?issuer=, filtered in place by filter.js)
// and the badge cards. One list for both languages; the status comes from
// today's date, so the daily job re-renders the page.

type certCard struct {
	ID, Name, Issuer, IssuerKey, Tone, Date, Status, StatusKey, Verify string
	Skills                                                             []string
	Badge                                                              *image
}

type certStat struct {
	Value int
	Key   string
	Tone  string // "" is the text color
}

type certPage struct {
	page
	Stats  []certStat
	Chips  []certChip
	Cards  []certCard
	Issuer string // server-side filter (no-JS path); "" on the cached page
}

// certChip is an issuer chip in the issuer's tone.
type certChip struct {
	filterChip
	Tone string
}

func (h *Handlers) certificates(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	repo := certificate.NewRepo(h.db)
	issuers, err := repo.Issuers(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	certs, err := repo.Published(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var ids []string
	for _, c := range certs {
		if c.BadgeID != nil {
			ids = append(ids, *c.BadgeID)
		}
	}
	imgs, err := h.images(ctx, lang, ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	byID := map[string]certificate.Issuer{}
	for _, i := range issuers {
		byID[i.ID] = i
	}
	now := content.Clock()
	p := certPage{page: h.newPage(lang, "certificates", i18n.T(lang, "cert.title")+" · halid.org", "/certificates", nil)}
	p.Head.Description = i18n.T(lang, "cert.sub")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.certificates"), Href: "/" + lang + "/certificates/feed.xml"}}
	active, used := 0, map[string]bool{}
	for _, c := range certs {
		iss := byID[c.IssuerID]
		st := c.Status(now)
		if st != certificate.Expired {
			active++
		}
		used[iss.Slug] = true
		card := certCard{ID: c.ID, Name: c.Name, Issuer: iss.Name, IssuerKey: iss.Slug, Tone: iss.Tone, Date: certificate.Month(&c.IssuedAt),
			StatusKey: st, Status: i18n.T(lang, "cert.status."+st), Skills: c.Skills}
		if c.CredlyURL != nil {
			card.Verify = *c.CredlyURL
		}
		if c.BadgeID != nil {
			if im, ok := imgs[*c.BadgeID]; ok {
				card.Badge = &im
			}
		}
		p.Cards = append(p.Cards, card)
	}
	p.Stats = []certStat{{active, i18n.T(lang, "cert.stat.active"), "green"}, {len(used), i18n.T(lang, "cert.stat.issuers"), "cyan"},
		{len(certs), i18n.T(lang, "cert.stat.total"), ""}}
	var keys []string
	for _, i := range issuers {
		if used[i.Slug] {
			keys = append(keys, i.Slug)
		}
	}
	p.Issuer = r.URL.Query().Get("issuer")
	if !used[p.Issuer] {
		p.Issuer = ""
	}
	bySlug := map[string]certificate.Issuer{}
	for _, i := range issuers {
		bySlug[i.Slug] = i
	}
	for _, c := range chips(lang, "/"+lang+"/certificates", "issuer", p.Issuer, keys, func(k string) string { return bySlug[k].Name }) {
		t := bySlug[c.Key].Tone
		if c.Key == "" {
			t = "navy"
		}
		p.Chips = append(p.Chips, certChip{filterChip: c, Tone: t})
	}
	if p.Issuer != "" {
		kept := p.Cards[:0]
		for _, c := range p.Cards {
			if c.IssuerKey == p.Issuer {
				kept = append(kept, c)
			}
		}
		p.Cards = kept
		p.Head.Noindex = true
	}
	p.Head.Scripts = []string{"/static/js/filter.js"}
	h.render(w, r, "certificates", p)
}

// certSummary is About's certificates card: up to five badges of active
// certificates and "A, B, C and N more".
func (h *Handlers) certSummary(r *http.Request, lang string) (*aboutCard, error) {
	ctx := r.Context()
	repo := certificate.NewRepo(h.db)
	certs, err := repo.Published(ctx)
	if err != nil || len(certs) == 0 {
		return nil, err
	}
	issuers, err := repo.Issuers(ctx)
	if err != nil {
		return nil, err
	}
	tone := map[string]string{}
	for _, i := range issuers {
		tone[i.ID] = i.Tone
	}
	now := content.Clock()
	card := &aboutCard{Href: "/" + lang + "/certificates", Glyph: "✓", Label: i18n.T(lang, "cert.eyebrow"), Tone: "cyan"}
	var names []string
	var ids []string
	for _, c := range certs {
		if c.Status(now) == certificate.Expired {
			continue
		}
		names = append(names, c.Name)
		if len(card.Badges) < 5 {
			b := aboutBadge{Tone: tone[c.IssuerID]}
			if c.BadgeID != nil {
				b.ID = *c.BadgeID
				ids = append(ids, b.ID)
			}
			card.Badges = append(card.Badges, b)
		}
	}
	if len(names) == 0 {
		return nil, nil
	}
	imgs, err := h.images(ctx, lang, ids)
	if err != nil {
		return nil, err
	}
	for i, b := range card.Badges {
		if im, ok := imgs[b.ID]; ok {
			card.Badges[i].Image = &im
		}
	}
	switch n := len(names); {
	case n <= 3:
		card.Text = joinNames(lang, names)
	default:
		card.Text = names[0] + ", " + names[1] + ", " + names[2] + " " + i18n.T(lang, "cert.and") + " " + strconv.Itoa(n-3) + " " + i18n.T(lang, "cert.more")
	}
	return card, nil
}

func joinNames(lang string, names []string) string {
	switch len(names) {
	case 1:
		return names[0]
	case 2:
		return names[0] + " " + i18n.T(lang, "cert.and") + " " + names[1]
	}
	return names[0] + ", " + names[1] + " " + i18n.T(lang, "cert.and") + " " + names[2]
}

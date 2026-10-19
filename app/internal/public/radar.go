package public

import (
	"errors"
	"net/http"
	"strconv"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/radar"
	"halid.org/engine/internal/i18n"
)

// radarBlip is one item on the radar and its row in the list.
type radarBlip struct {
	N                     int
	Href, Name, Note, Tag string
	Ring, Quadrant        string
	X, Y                  string // the blip's center in the 500×500 viewBox
	TrailX, TrailY        string // where the "moved" trail starts, relative to the blip
	New, Moved            bool
}

type radarRing struct {
	Key, Name string
	Items     []radarBlip
}

type radarEdition struct {
	Label, Href string
	Current     bool
}

type radarLabel struct {
	Key, Name, X string
}

type radarPage struct {
	page
	Edition   string
	Editions  []radarEdition
	Blips     []radarBlip
	Rings     []radarRing
	RingNames []radarLabel
	Quadrants map[string]string
	Dropped   []string
}

// radarTrail is how far out (or in) the dashed "moved" trail starts.
const radarTrail = 18

func fmtf(f float64) string { return strconv.FormatFloat(f, 'f', 1, 64) }

// radarHref is where an edition lives: the latest at /{lang}/radar.
func radarHref(lang, label, latest string) string {
	if label == latest {
		return "/" + lang + "/radar"
	}
	return "/" + lang + "/radar/" + label
}

// radarTag is the list tag of a blip: new, or the ring it moved from.
func radarTag(lang string, b radar.Blip) string {
	switch {
	case b.New:
		return i18n.T(lang, "radar.new")
	case b.Moved && radar.RingIndex(b.Prev) > radar.RingIndex(b.Ring):
		return "↑ " + i18n.T(lang, "radar.from") + " " + i18n.T(lang, "radar.ring."+b.Prev)
	case b.Moved:
		return "↓ " + i18n.T(lang, "radar.from") + " " + i18n.T(lang, "radar.ring."+b.Prev)
	}
	return ""
}

// radarIndex serves /{lang}/radar (the latest edition) and
// /{lang}/radar/{YYYY-MM}; any other /{lang}/radar/{slug} is an item.
func (h *Handlers) radarIndex(w http.ResponseWriter, r *http.Request, lang, label string) {
	if label != "" && !radar.ValidLabel(label) {
		h.radarItem(w, r, lang, label)
		return
	}
	v, err := radar.NewRepo(h.db).Edition(r.Context(), label)
	if errors.Is(err, radar.ErrNoEdition) && label != "" {
		h.notFound(w, r)
		return
	}
	if err != nil && !errors.Is(err, radar.ErrNoEdition) {
		h.fail(w, r, err)
		return
	}
	title := i18n.T(lang, "radar.title")
	path := "/radar"
	latest := ""
	if len(v.Editions) > 0 {
		latest = v.Editions[0].Label
	}
	if label != "" && label != latest {
		title += " · " + label
		path += "/" + label
	}
	p := radarPage{page: h.newPage(lang, "radar", title+" · halid.org", path, nil), Edition: v.Edition.Label, Quadrants: map[string]string{}}
	p.Head.Description = i18n.T(lang, "radar.intro")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.radar"), Href: "/" + lang + "/radar/feed.xml"}}
	p.Head.Scripts = []string{"/static/js/radar.js"}
	if label == latest && label != "" {
		// The latest edition's own URL repeats /radar.
		p.Head.Canonical = h.origin + "/" + lang + "/radar"
	}
	for _, e := range v.Editions {
		p.Editions = append(p.Editions, radarEdition{Label: e.Label, Href: radarHref(lang, e.Label, latest), Current: e.Label == v.Edition.Label})
	}
	for i, k := range radar.Rings {
		p.RingNames = append(p.RingNames, radarLabel{Key: k, Name: i18n.T(lang, "radar.ring."+k), X: fmtf(radar.Center + radar.RingRadii[i] + 4)})
	}
	for _, q := range radar.Quadrants {
		p.Quadrants[q] = i18n.T(lang, "radar.q."+q)
	}
	pts := radar.Layout(v.Blips)
	rings := map[string]*radarRing{}
	for _, k := range radar.Rings {
		p.Rings = append(p.Rings, radarRing{Key: k, Name: i18n.T(lang, "radar.ring."+k)})
	}
	for i := range p.Rings {
		rings[p.Rings[i].Key] = &p.Rings[i]
	}
	for i, b := range v.Blips {
		pt := pts[b.Group]
		x := radarBlip{N: i + 1, Href: "/" + lang + "/radar/" + b.Slug, Name: b.Name, Note: b.Note(lang), Tag: radarTag(lang, b),
			Ring: b.Ring, Quadrant: b.Quadrant, X: fmtf(pt.X), Y: fmtf(pt.Y), New: b.New, Moved: b.Moved}
		if b.Moved {
			// The trail comes from the ring it left: outside when promoted.
			out := float64(radarTrail)
			if radar.RingIndex(b.Prev) < radar.RingIndex(b.Ring) {
				out = -out
			}
			x.TrailX, x.TrailY = fmtf(pt.DX*out), fmtf(pt.DY*out)
		}
		p.Blips = append(p.Blips, x)
		rings[b.Ring].Items = append(rings[b.Ring].Items, x)
	}
	for _, b := range v.Dropped {
		p.Dropped = append(p.Dropped, b.Name)
	}
	h.render(w, r, "radar", p)
}

// radarHistRow is one edition on an item's page.
type radarHistRow struct {
	Label, Href, Ring, RingName, Tag, Note string
	Off                                    bool
}

type radarItemPage struct {
	page
	ArticleLang  string
	Untranslated bool
	OtherURL     string
	Name         string
	Quadrant     string
	Ring         string // on the latest edition, "" when not on it
	RingName     string
	Latest       string
	Note         string
	History      []radarHistRow
}

// radarItem serves /{lang}/radar/{slug}: the item, its ring on the latest
// edition and its rings across editions.
func (h *Handlers) radarItem(w http.ResponseWriter, r *http.Request, lang, slug string) {
	ctx := r.Context()
	if !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	repo := radar.NewRepo(h.db)
	e, untranslated, err := findPublished(lang, slug, func(l, s string) (content.Entry, error) { return repo.GetPublished(ctx, l, s) })
	if err != nil {
		h.fail(w, r, err)
		return
	}
	hist, err := repo.HistoryOf(ctx, e.TranslationGroupID, lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	eds, err := repo.Editions(ctx)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	pg, orig, ok := h.itemPage(w, r, lang, "radar", "radar", e.Title, e, untranslated)
	if !ok {
		return
	}
	p := radarItemPage{page: pg, ArticleLang: e.Lang, Untranslated: untranslated, OtherURL: orig, Name: e.Title, Note: e.Summary}
	if len(eds) > 0 {
		p.Latest = eds[0].Label
	}
	shown := ""
	for _, x := range hist {
		row := radarHistRow{Label: x.Label, Href: radarHref(lang, x.Label, p.Latest), Off: x.Off}
		if x.Off {
			row.Tag = i18n.T(lang, "radar.left")
		} else {
			row.Ring, row.RingName = x.Ring, i18n.T(lang, "radar.ring."+x.Ring)
			b := radar.Blip{Ring: x.Ring, Prev: x.Prev, Moved: x.Prev != "" && x.Prev != x.Ring}
			// New when it joins after the first edition.
			b.New = x.Prev == "" && x.Label != eds[len(eds)-1].Label
			row.Tag = radarTag(lang, b)
			// A note shows where it changed: under the newest edition with it.
			if x.Note != shown {
				row.Note = x.Note
			}
			shown = x.Note
			if p.Quadrant == "" {
				p.Quadrant = i18n.T(lang, "radar.q."+x.Quadrant)
			}
		}
		p.History = append(p.History, row)
	}
	if len(hist) > 0 && !hist[0].Off && hist[0].Label == p.Latest {
		p.Ring, p.RingName = hist[0].Ring, i18n.T(lang, "radar.ring."+hist[0].Ring)
	}
	h.render(w, r, "radar-item", p)
}

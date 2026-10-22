package public

import (
	"context"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/book"
	pages "halid.org/engine/internal/content/page"
	"halid.org/engine/internal/i18n"
)

// About (design/About.dc.html) and Now (design/Now.dc.html), Phase 5.1,
// with /now/archive (derive). Both are singleton pages per language.

type aboutCTA struct {
	Label, Href string
	Primary     bool
	Download    bool
}

type aboutPage struct {
	page
	Name     string
	Bio      template.HTML
	CTAs     []aboutCTA
	Portrait *image
	Skills   []pages.SkillGroup
	Jobs     []aboutJob
	Cards    []aboutCard
}

type aboutJob struct {
	When string
	pages.Job
}

// aboutCard is one link card next to the timeline: the sections with a
// page, with their live counts (the home tiles), and Now.
type aboutCard struct {
	Href, Glyph, Label, Text, Tone string
	Badges                         []aboutBadge // certificates: mini hexagons
}

type aboutBadge struct {
	ID, Tone string
	Image    *image
}

// author is the site owner as an h-card on articles (Phase 8.2): the About
// page's name and portrait, in lang or else the other language. Empty
// fields when there is no About page yet; the card then names the host.
type author struct {
	Name, URL, Photo string
}

func (h *Handlers) author(ctx context.Context, lang string) (author, error) {
	a := author{URL: h.origin + "/" + lang + "/about"}
	langs := []string{lang}
	for _, l := range i18n.Supported {
		if l != lang {
			langs = append(langs, l)
		}
	}
	for _, l := range langs {
		pg, err := h.pageRepo.GetPublished(ctx, l, pages.About)
		if errors.Is(err, content.ErrNotFound) {
			continue
		}
		if err != nil {
			return a, err
		}
		a.Name, a.URL = pg.Data.Name, h.origin+"/"+l+"/about"
		if pg.Data.Portrait != "" {
			imgs, err := h.images(ctx, l, []string{pg.Data.Portrait})
			if err != nil {
				return a, err
			}
			if im, ok := imgs[pg.Data.Portrait]; ok {
				a.Photo = h.origin + im.URL
			}
		}
		break
	}
	if a.Name == "" {
		if u, err := url.Parse(h.origin); err == nil {
			a.Name = u.Hostname()
		}
	}
	return a, nil
}

func (h *Handlers) about(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	pg, err := h.pageRepo.GetPublished(ctx, lang, pages.About)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	d := pg.Data
	p := aboutPage{page: h.newPage(lang, "about", pg.Title+" · halid.org", "/about", nil), Name: d.Name, Bio: template.HTML(pg.HTML), Skills: d.Skills}
	p.Head.Description = pg.Summary
	ids := []string{}
	if d.Portrait != "" {
		ids = append(ids, d.Portrait)
	}
	if d.CV != "" {
		ids = append(ids, d.CV)
	}
	imgs, err := h.images(ctx, lang, ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if im, ok := imgs[d.Portrait]; ok {
		p.Portrait = &im
	}
	// The first button is the primary one; the CV follows it.
	for i, c := range d.CTAs {
		p.CTAs = append(p.CTAs, aboutCTA{Label: c.Label, Href: c.URL, Primary: i == 0})
		if i == 0 {
			if cv, ok := imgs[d.CV]; ok {
				p.CTAs = append(p.CTAs, aboutCTA{Label: i18n.T(lang, "about.cv"), Href: cv.URL, Download: true})
			}
		}
	}
	if len(d.CTAs) == 0 {
		if cv, ok := imgs[d.CV]; ok {
			p.CTAs = append(p.CTAs, aboutCTA{Label: i18n.T(lang, "about.cv"), Href: cv.URL, Download: true})
		}
	}
	for _, j := range d.Experience {
		when := j.From
		if j.To != "" {
			when += " – " + j.To
		}
		p.Jobs = append(p.Jobs, aboutJob{When: when, Job: j})
	}
	cert, err := h.certSummary(r, lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if cert != nil {
		p.Cards = append(p.Cards, *cert)
	}
	tiles, err := h.tiles(r, lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, t := range tiles {
		text := t.Count
		if t.About != "" {
			text = t.About
		}
		p.Cards = append(p.Cards, aboutCard{Href: t.Href, Glyph: t.Glyph, Label: t.Label, Text: text, Tone: t.Tone})
	}
	if _, err := h.pageRepo.GetPublished(ctx, lang, pages.Now); err == nil {
		p.Cards = append(p.Cards, aboutCard{Href: "/" + lang + "/now", Glyph: "●", Label: i18n.T(lang, "nav.now"), Text: i18n.T(lang, "about.now_card"), Tone: "indigo"})
	} else if !errors.Is(err, content.ErrNotFound) {
		h.fail(w, r, err)
		return
	}
	h.render(w, r, "about", p)
}

// --- Now --------------------------------------------------------------------

type nowBuild struct {
	pages.Build
	Cover *image
}

type nowBook struct {
	pages.Book
	Cover *image
}

type nowLearning struct {
	pages.Learning
	LevelLabel string
	Cells      []bool
}

type nowPage struct {
	page
	Updated  string // "updated 4 days ago", or the version's date in the archive
	Date     string
	Place    string
	Archive  string // "11 earlier versions →", empty without any
	Version  bool   // an archived version
	Current  string // the live page, from a version
	Building []nowBuild
	Reading  []nowBook
	Learning []nowLearning
	NotDoing []string
}

// daysAgo is "updated today", "updated 1 day ago" or "updated N days ago".
func daysAgo(lang string, at, now time.Time) string {
	d := int(now.UTC().Truncate(24*time.Hour).Sub(at.UTC().Truncate(24*time.Hour)).Hours() / 24)
	switch {
	case d <= 0:
		return i18n.T(lang, "now.updated_today")
	case d == 1:
		return i18n.T(lang, "now.updated_yesterday")
	}
	return fmt.Sprintf(i18n.T(lang, "now.updated_days"), d)
}

// nowView fills the sections of a Now page from its data.
func (h *Handlers) nowView(ctx context.Context, lang string, p *nowPage, d pages.Data) error {
	var ids []string
	for _, b := range d.Building {
		if b.Cover != "" {
			ids = append(ids, b.Cover)
		}
	}
	for _, b := range d.Reading {
		if b.Cover != "" {
			ids = append(ids, b.Cover)
		}
	}
	imgs, err := h.images(ctx, lang, ids)
	if err != nil {
		return err
	}
	cover := func(id string) *image {
		if im, ok := imgs[id]; ok {
			return &im
		}
		return nil
	}
	p.Place, p.NotDoing = d.Place, d.NotDoing
	for _, b := range d.Building {
		if b.Tone == "" {
			b.Tone = "green"
		}
		p.Building = append(p.Building, nowBuild{Build: b, Cover: cover(b.Cover)})
	}
	for _, b := range d.Reading {
		if b.Tone == "" {
			b.Tone = "amber"
		}
		p.Reading = append(p.Reading, nowBook{Book: b, Cover: cover(b.Cover)})
	}
	for _, l := range d.Learning {
		if l.Tone == "" {
			l.Tone = "indigo"
		}
		cells := make([]bool, 10)
		for i := range cells {
			cells[i] = i < l.Level
		}
		p.Learning = append(p.Learning, nowLearning{Learning: l, LevelLabel: i18n.T(lang, pages.LevelKey(l.Level)), Cells: cells})
	}
	return nil
}

func (h *Handlers) now(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	pg, err := h.pageRepo.GetPublished(ctx, lang, pages.Now)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	versions, err := h.pageRepo.Versions(ctx, pg.ID, 200)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	at := time.Unix(*pg.PublishedAt, 0)
	if len(versions) > 0 {
		at = time.Unix(versions[0].PublishedAt, 0)
	}
	p := nowPage{page: h.newPage(lang, "now", pg.Title+" · halid.org", "/now", nil),
		Updated: daysAgo(lang, at, content.Clock()), Date: at.UTC().Format("2006-01-02")}
	p.Head.Description = pg.Summary
	switch n := len(versions) - 1; {
	case n == 1:
		p.Archive = i18n.T(lang, "now.earlier_one")
	case n > 1:
		p.Archive = fmt.Sprintf(i18n.T(lang, "now.earlier"), n)
	}
	// The books being read come from the reading list (5.5); the ones typed
	// in the Now form stay as the fallback, and archived versions keep theirs.
	data := pg.Data
	reading, err := book.NewRepo(h.db).List(ctx, lang, "reading")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if len(reading) > 0 {
		data.Reading = nil
		for _, b := range reading {
			nb := pages.Book{Title: b.Title, Author: b.Author, Pct: b.Progress}
			if b.CoverMediaID != nil {
				nb.Cover = *b.CoverMediaID
			}
			if b.Tone != nil {
				nb.Tone = *b.Tone
			}
			data.Reading = append(data.Reading, nb)
		}
	}
	if err := h.nowView(ctx, lang, &p, data); err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, r, "now", p)
}

// --- /now/archive -----------------------------------------------------------

type nowVersion struct {
	Href, Date, Place string
}

type nowArchivePage struct {
	page
	Versions []nowVersion
}

// nowVersions are the published versions before the live one, newest
// first, one per day (the last of the day).
func (h *Handlers) nowVersions(ctx context.Context, lang string) (pages.Page, []pages.Version, error) {
	pg, err := h.pageRepo.GetPublished(ctx, lang, pages.Now)
	if err != nil {
		return pg, nil, err
	}
	all, err := h.pageRepo.Versions(ctx, pg.ID, 500)
	if err != nil {
		return pg, nil, err
	}
	var out []pages.Version
	seen := map[string]bool{}
	for i, v := range all {
		day := time.Unix(v.PublishedAt, 0).UTC().Format("2006-01-02")
		if i == 0 || seen[day] {
			seen[day] = true
			continue
		}
		seen[day] = true
		out = append(out, v)
	}
	return pg, out, nil
}

func (h *Handlers) nowArchive(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	_, versions, err := h.nowVersions(ctx, lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := nowArchivePage{page: h.newPage(lang, "now", i18n.T(lang, "now.archive_title")+" · halid.org", "/now/archive", nil)}
	for _, v := range versions {
		day := time.Unix(v.PublishedAt, 0).UTC().Format("2006-01-02")
		d, err := h.pageRepo.VersionData(ctx, v.RevisionID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		p.Versions = append(p.Versions, nowVersion{Href: "/" + lang + "/now/archive/" + day, Date: day, Place: d.Place})
	}
	h.render(w, r, "now-archive", p)
}

func (h *Handlers) nowVersion(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	day := r.PathValue("day")
	if _, err := time.Parse("2006-01-02", day); err != nil {
		h.notFound(w, r)
		return
	}
	pg, versions, err := h.nowVersions(ctx, lang)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, v := range versions {
		if time.Unix(v.PublishedAt, 0).UTC().Format("2006-01-02") != day {
			continue
		}
		d, err := h.pageRepo.VersionData(ctx, v.RevisionID)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		p := nowPage{page: h.newPage(lang, "now", pg.Title+" · "+day+" · halid.org", "/now/archive/"+day, nil),
			Updated: fmt.Sprintf(i18n.T(lang, "now.version_of"), day), Date: day, Version: true, Current: "/" + lang + "/now"}
		p.Head.Noindex = true
		p.Archive = i18n.T(lang, "now.archive_all")
		if err := h.nowView(ctx, lang, &p, d); err != nil {
			h.fail(w, r, err)
			return
		}
		h.render(w, r, "now", p)
		return
	}
	h.notFound(w, r)
}

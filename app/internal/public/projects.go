package public

import (
	"context"
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/github"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/render/og"
	"halid.org/engine/internal/store"
)

// Projects (design/Projects.dc.html) and Project Detail (design/Project
// Detail.dc.html, one template for software and homelab), Phase 4.2. The
// projects page lists software; homelab builds have their own page (4.3).
// The GitHub section of the projects page waits for the sync (4.4): it is
// left out rather than filled with sample data.

// statusTone maps a project status to the badge color class.
var statusTone = map[string]string{
	"active": "ok", "running": "ok", "maintained": "link", "in_progress": "link",
	"paused": "warn", "idea": "muted", "done": "muted", "archived": "muted",
}

type image struct{ URL, Alt string }

// images reads the media items with ids, keyed by id, with alt text in lang.
func (h *Handlers) images(ctx context.Context, lang string, ids []string) (map[string]image, error) {
	out := map[string]image{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	res, err := h.db.Query(ctx, `SELECT id, r2_key, alt_tr, alt_en FROM media WHERE id IN (?`+strings.Repeat(", ?", len(ids)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		ID    string `json:"id"`
		Key   string `json:"r2_key"`
		AltTR string `json:"alt_tr"`
		AltEN string `json:"alt_en"`
	}](res)
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		alt := m.AltEN
		if lang == "tr" {
			alt = m.AltTR
		}
		out[m.ID] = image{URL: "/" + m.Key, Alt: alt}
	}
	return out, nil
}

// projectTone is the project's tone, else its category's, else green
// (software) or orange (homelab), as the prototypes.
func (h *Handlers) projectTone(ctx context.Context, p project.Project) string {
	if p.Tone != nil && *p.Tone != "" {
		return *p.Tone
	}
	if p.CategoryID != nil {
		if c, err := content.GetCategory(ctx, h.db, *p.CategoryID); err == nil && c.Tone != nil {
			return *c.Tone
		}
	}
	if p.Type == project.Homelab {
		return "orange"
	}
	return "green"
}

// kindLine is the small line over the title, and the card pills: the
// project's own, else the first three stack items (software) or the
// category (homelab).
func (h *Handlers) kindLine(ctx context.Context, lang string, p project.Project) []string {
	if p.KindLine != "" {
		var out []string
		for _, s := range strings.Split(p.KindLine, "·") {
			if s = strings.TrimSpace(s); s != "" {
				out = append(out, s)
			}
		}
		return out
	}
	if p.Type == project.Homelab && p.CategoryID != nil {
		if c, err := content.GetCategory(ctx, h.db, *p.CategoryID); err == nil {
			return []string{c.Name(lang)}
		}
	}
	var out []string
	for i, s := range p.Stack {
		if i == 3 {
			break
		}
		out = append(out, strings.ToLower(s.Name))
	}
	return out
}

type projectCard struct {
	Href, Name, Tagline, Status, StatusTone, Tone string
	Other                                         string // its language, when not the page's (fallback)
	Pills                                         []string
	Cover                                         *image
	Repo                                          string // github_repo, for the stars
	Stars                                         *int
	Activity                                      string
}

type projectsPage struct {
	page
	Cards  []projectCard
	GitHub *github.Section // nil before the first sync
}

func (h *Handlers) projects(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	list, err := h.projectRepo.List(ctx, project.ListFilter{Lang: lang, Type: project.Software})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var ids []string
	for _, p := range list {
		if p.CoverMediaID != nil {
			ids = append(ids, *p.CoverMediaID)
		}
	}
	imgs, err := h.images(ctx, lang, ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	p := projectsPage{page: h.newPage(lang, "projects", i18n.T(lang, "nav.projects")+" · halid.org", "/projects", nil)}
	p.Head.Description = i18n.T(lang, "projects.intro")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.projects"), Href: "/" + lang + "/projects/feed.xml"}}
	for _, x := range list {
		// The pills come from the kind line; only without one do they need
		// the published stack.
		full := x
		if x.KindLine == "" && x.Type == project.Software {
			if full, err = h.projectRepo.GetPublished(ctx, x.Lang, x.Slug); err != nil {
				h.fail(w, r, err)
				return
			}
		}
		c := projectCard{Href: "/" + x.Lang + "/projects/" + x.Slug, Other: other(lang, x.Lang), Name: x.Title, Tagline: x.Tagline,
			Status: i18n.T(lang, "kind.project.status."+x.ProjectStatus), StatusTone: statusTone[x.ProjectStatus],
			Tone: h.projectTone(ctx, x), Pills: h.kindLine(ctx, lang, full)}
		if x.CoverMediaID != nil {
			if im, ok := imgs[*x.CoverMediaID]; ok {
				c.Cover = &im
			}
		}
		c.Repo = x.GitHubRepo
		p.Cards = append(p.Cards, c)
	}
	// Stars and activity from the GitHub sync (4.4); cards without a linked
	// repo, or before the first sync, show neither.
	var repos []string
	for _, c := range p.Cards {
		if c.Repo != "" {
			repos = append(repos, c.Repo)
		}
	}
	stats, err := github.Stats(ctx, h.db, lang, repos, content.Clock())
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for i, c := range p.Cards {
		if st, ok := stats[strings.ToLower(c.Repo)]; ok {
			stars := st.Stars
			p.Cards[i].Stars, p.Cards[i].Activity = &stars, st.Activity
		}
	}
	if p.GitHub, err = github.Load(ctx, h.db, lang); err != nil {
		h.fail(w, r, err)
		return
	}
	h.render(w, r, "projects", p)
}

type factCell struct{ Key, Value string }

type asideRow struct{ Name, Note string }

type logEntry struct{ Href, Date, Title string }

type projectView struct {
	Name, Tagline, Status, Tone, Back, BackHref string
	Kind                                        []string
	Links                                       []project.Link
	Cover                                       *image
	Facts                                       []factCell
	Gallery                                     []image
	ListTitle                                   string
	List                                        []asideRow
	Log                                         []logEntry
}

type projectPage struct {
	page
	ArticleLang  string
	Untranslated bool
	OtherURL     string
	Project      projectView
	Body         template.HTML // the published revision, from the renderer (ADR-002)
}

func (h *Handlers) project(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	slug := r.PathValue("slug")
	if !content.ValidSlug(slug) {
		h.notFound(w, r)
		return
	}
	p, untranslated, err := findPublished(lang, slug, func(l, s string) (project.Project, error) { return h.projectRepo.GetPublished(ctx, l, s) })
	if err != nil {
		h.fail(w, r, err)
		return
	}
	active := "projects"
	if p.Type == project.Homelab {
		active = "homelab"
	}
	pg, orig, ok := h.itemPage(w, r, lang, "projects", active, p.Title, p.Entry, untranslated)
	if !ok {
		return
	}
	if p.Summary == "" {
		pg.Head.Description = p.Tagline
	}
	pg.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.projects"), Href: "/" + lang + "/projects/feed.xml"}}
	v := projectView{Name: p.Title, Tagline: p.Tagline, Status: i18n.T(lang, "kind.project.status."+p.ProjectStatus),
		Tone: h.projectTone(ctx, p), Kind: h.kindLine(ctx, lang, p), Links: p.Links,
		Back: i18n.T(lang, "nav.projects"), BackHref: "/" + lang + "/projects"}
	if p.Type == project.Homelab {
		v.Back, v.BackHref = i18n.T(lang, "projects.back_homelab"), "/"+lang+"/homelab"
	}
	for _, f := range p.Facts {
		v.Facts = append(v.Facts, factCell{f.Key, f.Value})
	}
	if amount, ok := p.Cost(); ok && p.Type == project.Homelab {
		v.Facts = append(v.Facts, factCell{i18n.T(lang, "projects.cost"), project.CompactMoney(amount, p.Currency)})
	}
	ids := append([]string(nil), p.Gallery...)
	if p.CoverMediaID != nil {
		ids = append(ids, *p.CoverMediaID)
	}
	imgs, err := h.images(ctx, lang, ids)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if p.CoverMediaID != nil {
		if im, ok := imgs[*p.CoverMediaID]; ok {
			v.Cover = &im
		}
	}
	for _, id := range p.Gallery {
		if im, ok := imgs[id]; ok {
			v.Gallery = append(v.Gallery, im)
		}
	}
	if p.Type == project.Homelab {
		v.ListTitle = i18n.T(lang, "projects.parts")
		for _, x := range p.Parts {
			name := x.Name
			if x.Qty > 1 && !strings.Contains(name, "×") {
				name = strconv.Itoa(x.Qty) + "× " + name
			}
			row := asideRow{Name: name}
			if x.Price != nil {
				row.Note = project.CompactMoney(*x.Price, x.Currency)
			}
			v.List = append(v.List, row)
		}
	} else {
		v.ListTitle = i18n.T(lang, "projects.stack")
		for _, s := range p.Stack {
			v.List = append(v.List, asideRow{s.Name, s.Role})
		}
	}
	logs, err := h.logRepo.ForProject(ctx, p.ID, false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for _, l := range logs {
		v.Log = append(v.Log, logEntry{Href: "/" + p.Lang + "/projects/" + p.Slug + "/log/" + l.Slug, Date: project.FormatDay(l.LogDate), Title: l.Title})
	}
	h.render(w, r, "project", projectPage{page: pg, ArticleLang: p.Lang, Untranslated: untranslated, OtherURL: orig, Project: v, Body: template.HTML(p.HTML)})
}

type logPage struct {
	page
	Project, ProjectHref, Date, Hours, Tone string
	Title                                   string
	Body                                    template.HTML
	Prev, Next                              *logEntry
}

// buildlog serves /{lang}/projects/{project}/log/{slug}: one entry of a
// project's build log (derive: the article column of Project Detail).
func (h *Handlers) buildlog(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	slug, proj := r.PathValue("log"), r.PathValue("slug")
	if !content.ValidSlug(slug) || !content.ValidSlug(proj) {
		h.notFound(w, r)
		return
	}
	b, html, err := h.logRepo.GetPublished(ctx, lang, slug)
	if err == nil && b.ProjectSlug != proj {
		err = content.NotFound(buildlog.Kind, proj+"/"+slug)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	pr, err := h.projectRepo.GetPublished(ctx, lang, proj)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	base := "/" + lang + "/projects/" + proj
	pg := h.newPage(lang, "projects", b.Title+" · "+pr.Title+" · halid.org", "/projects/"+proj+"/log/"+slug, func(l string) string {
		if l == lang {
			return base + "/log/" + slug
		}
		return "/" + l + "/projects"
	})
	pg.Head.Alternates = nil // a log exists in its project's language only
	pg.Head.Description = b.Summary
	pg.Head.OGType = "article"
	if b.PublishedAt != nil {
		pg.Head.Image = h.origin + og.URL(og.EntryKey(lang, og.LogSection, slug), og.BuildLog(lang, b.Title, slug, *b.PublishedAt))
	}
	p := logPage{page: pg, Project: pr.Title, ProjectHref: base, Date: project.FormatDay(b.LogDate), Title: b.Title,
		Tone: h.projectTone(ctx, pr), Body: template.HTML(html)}
	if b.Hours != nil {
		p.Hours = strconv.FormatFloat(*b.Hours, 'f', -1, 64)
	}
	logs, err := h.logRepo.ForProject(ctx, pr.ID, false)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	for i, l := range logs { // newest first
		if l.ID != b.ID {
			continue
		}
		if i > 0 {
			n := logs[i-1]
			p.Next = &logEntry{Href: base + "/log/" + n.Slug, Date: project.FormatDay(n.LogDate), Title: n.Title}
		}
		if i+1 < len(logs) {
			o := logs[i+1]
			p.Prev = &logEntry{Href: base + "/log/" + o.Slug, Date: project.FormatDay(o.LogDate), Title: o.Title}
		}
	}
	h.render(w, r, "buildlog", p)
}

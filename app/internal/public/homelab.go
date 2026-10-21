package public

import (
	"net/http"
	"strconv"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/settings"
)

// Homelab & DIY (design/Homelab.dc.html), Phase 4.3: the orange hero with
// four stat tiles (builds is counted, the rest are typed in admin
// settings), the featured build, category chips (?category=, filtered in
// place by filter.js) and the build cards.

type homelabChip struct {
	filterChip
	Tone string
}

type buildCard struct {
	Href, Name, Tagline, Category, CategoryKey, Tone, Status, Cost, Time string
	Other                                                                string // its language, when not the page's (fallback)
	Cover                                                                *image
}

type homelabStat struct{ Value, Key string }

type homelabPage struct {
	page
	Category string // server-side filter (no-JS path); "" on the cached page
	Stats    []homelabStat
	Featured *buildCard
	Chips    []homelabChip
	Builds   []buildCard
}

func (h *Handlers) homelab(w http.ResponseWriter, r *http.Request, lang string) {
	ctx := r.Context()
	list, err := h.projectRepo.List(ctx, project.ListFilter{Lang: lang, Type: project.Homelab})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	cats, err := content.Categories(ctx, h.db, "homelab")
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var typed settings.Homelab
	if err := settings.Get(ctx, h.db, settings.HomelabKey, &typed); err != nil {
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
	byID := map[string]content.Category{}
	for _, c := range cats {
		byID[c.ID] = c
	}
	p := homelabPage{page: h.newPage(lang, "homelab", i18n.T(lang, "homelab.title")+" · halid.org", "/homelab", nil)}
	p.Head.Description = i18n.T(lang, "homelab.sub")
	p.Head.Feeds = []feedLink{{Title: i18n.T(lang, "feed.projects"), Href: "/" + lang + "/projects/feed.xml"}}
	p.Head.Scripts = []string{"/static/js/filter.js"}
	dash := func(s string) string {
		if s == "" {
			return "–"
		}
		return s
	}
	p.Stats = []homelabStat{{strconv.Itoa(len(list)), i18n.T(lang, "homelab.builds")}, {dash(typed.Nodes), i18n.T(lang, "settings.nodes")},
		{dash(typed.Idle), i18n.T(lang, "settings.idle")}, {dash(typed.Uptime), i18n.T(lang, "settings.uptime")}}

	var all []buildCard
	present := map[string]bool{}
	for _, x := range list {
		c := buildCard{Href: "/" + x.Lang + "/projects/" + x.Slug, Other: other(lang, x.Lang), Name: x.Title, Tagline: x.Tagline, Time: x.TimeSpent,
			Status: i18n.T(lang, "kind.project.status."+x.ProjectStatus), Tone: h.projectTone(ctx, x)}
		if x.CategoryID != nil {
			if cat, ok := byID[*x.CategoryID]; ok {
				c.Category, c.CategoryKey = cat.Name(lang), cat.Slug
				present[cat.Slug] = true
			}
		}
		if x.CoverMediaID != nil {
			if im, ok := imgs[*x.CoverMediaID]; ok {
				c.Cover = &im
			}
		}
		// The list has no rows; without a total the cost needs the parts.
		full := x
		if x.CostTotal == nil {
			if full, err = h.projectRepo.GetPublished(ctx, x.Lang, x.Slug); err != nil {
				h.fail(w, r, err)
				return
			}
		}
		if amount, ok := full.Cost(); ok {
			c.Cost = project.CompactMoney(amount, x.Currency)
		}
		all = append(all, c)
	}
	if len(all) > 0 { // featured first, else the newest (project.List order)
		f := all[0]
		p.Featured = &f
	}
	category := r.URL.Query().Get("category")
	if !present[category] {
		category = ""
	}
	p.Category = category
	chipTone := map[string]string{}
	var keys []string
	for _, c := range cats {
		if present[c.Slug] {
			keys = append(keys, c.Slug)
			chipTone[c.Slug] = catTone(c, byID)
		}
	}
	for _, c := range chips(lang, "/"+lang+"/homelab", "category", category, keys, func(k string) string {
		for _, x := range cats {
			if x.Slug == k {
				return x.Name(lang)
			}
		}
		return k
	}) {
		tone := chipTone[c.Key]
		if tone == "" {
			tone = "orange"
		}
		p.Chips = append(p.Chips, homelabChip{filterChip: c, Tone: tone})
	}
	for _, c := range all {
		if category == "" || c.CategoryKey == category {
			p.Builds = append(p.Builds, c)
		}
	}
	h.render(w, r, "homelab", p)
}

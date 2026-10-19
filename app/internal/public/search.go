package public

import (
	"context"
	"net/http"
	"sort"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/search"
	"halid.org/engine/internal/store"
)

// WithSearch gives the handlers the SEARCH database for /{lang}/search.
func (h *Handlers) WithSearch(db store.DB) *Handlers {
	h.searchDB = db
	return h
}

// searchLimit is how many results the no-JS page shows.
const searchLimit = 30

type searchPage struct {
	page
	Query   string
	Results []streamItem
}

// searchResults serves /{lang}/search?q= (derive: stream rows): the
// palette's search for browsers without JS, and its "all results" page.
// It ranks like the palette (Worker, ADR-010): bm25 with a recency boost,
// the page language first.
func (h *Handlers) searchResults(w http.ResponseWriter, r *http.Request, lang string) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) > 100 {
		q = string([]rune(q)[:100])
	}
	p := searchPage{page: h.newPage(lang, "", i18n.T(lang, "search.title")+" · halid.org", "/search", nil), Query: q}
	p.Head.Noindex = true
	if q != "" && h.searchDB != nil {
		rows, err := h.find(r.Context(), lang, q)
		if err != nil {
			h.fail(w, r, err)
			return
		}
		p.Results = rows
	}
	if q == "" {
		h.render(w, r, "search", p)
		return
	}
	// Results vary by query: the Worker keeps them out of KV (a query
	// string), and browsers should not reuse them either.
	w.Header().Set("Cache-Control", "private, no-store")
	var buf strings.Builder
	if err := h.tmpl["search"].ExecuteTemplate(&buf, "base", p); err != nil {
		h.fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(buf.String()))
}

// searchHit is a search row with what the result list shows.
type searchHit struct {
	ID, Kind, Lang, Slug, Title, Summary string
	URL                                  *string
	PublishedAt                          *int64
	Score                                float64
}

// find runs the query and turns hits into rows: entries from the main
// database (their titles may be newer than the index), photos and
// certificates from their tables.
func (h *Handlers) find(ctx context.Context, lang, raw string) ([]streamItem, error) {
	st, ok := search.Parse(raw).Stmt(search.Filter{Limit: 50})
	if !ok {
		return nil, nil
	}
	res, err := h.searchDB.Query(ctx, st.SQL, st.Args...)
	if err != nil {
		return nil, err
	}
	hits, err := store.Scan[search.Hit](res)
	if err != nil || len(hits) == 0 {
		return nil, err
	}
	rank := map[string]float64{}
	var entryIDs, photoIDs, certIDs []any
	for _, x := range hits {
		key := x.EntryID + "/" + x.Lang
		if _, seen := rank[key]; seen {
			continue
		}
		rank[key] = x.Rank
		switch x.Kind {
		case search.KindPhoto:
			photoIDs = append(photoIDs, x.EntryID)
		case search.KindCertificate:
			certIDs = append(certIDs, x.EntryID)
		default:
			entryIDs = append(entryIDs, x.EntryID)
		}
	}
	var found []searchHit
	in := func(n int) string { return "(?" + strings.Repeat(", ?", n-1) + ")" }
	if len(entryIDs) > 0 {
		res, err := h.db.Query(ctx, `SELECT e.id, CASE WHEN p.project_type = 'homelab' THEN 'homelab' ELSE e.kind END AS kind,
  e.lang, e.slug, e.title, e.summary, l.url, e.published_at
FROM entries e LEFT JOIN links l ON l.entry_id = e.id LEFT JOIN projects p ON p.entry_id = e.id
WHERE e.status = 'published' AND e.visibility = 'public' AND e.id IN `+in(len(entryIDs)), entryIDs...)
		if err != nil {
			return nil, err
		}
		rows, err := store.Scan[struct {
			ID          string  `json:"id"`
			Kind        string  `json:"kind"`
			Lang        string  `json:"lang"`
			Slug        string  `json:"slug"`
			Title       string  `json:"title"`
			Summary     string  `json:"summary"`
			URL         *string `json:"url"`
			PublishedAt *int64  `json:"published_at"`
		}](res)
		if err != nil {
			return nil, err
		}
		for _, e := range rows {
			found = append(found, searchHit{ID: e.ID, Kind: e.Kind, Lang: e.Lang, Slug: e.Slug, Title: e.Title, Summary: e.Summary, URL: e.URL, PublishedAt: e.PublishedAt})
		}
	}
	if len(photoIDs) > 0 {
		res, err := h.db.Query(ctx, `SELECT id, CASE WHEN ? = 'tr' AND title_tr <> '' OR title_en = '' THEN title_tr ELSE title_en END AS title,
  coalesce(taken_at, created_at) AS published_at FROM photos WHERE published = 1 AND id IN `+in(len(photoIDs)), append([]any{lang}, photoIDs...)...)
		if err != nil {
			return nil, err
		}
		rows, err := store.Scan[struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			PublishedAt int64  `json:"published_at"`
		}](res)
		if err != nil {
			return nil, err
		}
		for _, x := range rows {
			at := x.PublishedAt
			found = append(found, searchHit{ID: x.ID, Kind: search.KindPhoto, Lang: lang, Title: x.Title, PublishedAt: &at})
		}
	}
	if len(certIDs) > 0 {
		res, err := h.db.Query(ctx, `SELECT c.id, c.name AS title, i.name AS summary, c.issued_at AS published_at
FROM certificates c JOIN cert_issuers i ON i.id = c.issuer_id WHERE c.published = 1 AND c.id IN `+in(len(certIDs)), certIDs...)
		if err != nil {
			return nil, err
		}
		rows, err := store.Scan[struct {
			ID          string `json:"id"`
			Title       string `json:"title"`
			Summary     string `json:"summary"`
			PublishedAt int64  `json:"published_at"`
		}](res)
		if err != nil {
			return nil, err
		}
		for _, x := range rows {
			at := x.PublishedAt
			found = append(found, searchHit{ID: x.ID, Kind: search.KindCertificate, Lang: lang, Title: x.Title, Summary: x.Summary, PublishedAt: &at})
		}
	}
	now := content.Clock().Unix()
	for i := range found {
		x := &found[i]
		lk := x.Lang
		if x.Kind == search.KindPhoto || x.Kind == search.KindCertificate {
			// Indexed in both languages: the row in this language ranks it.
			lk = lang
		}
		var at int64
		if x.PublishedAt != nil {
			at = *x.PublishedAt
		}
		x.Score = search.Score(rank[x.ID+"/"+lk], at, now)
	}
	sort.SliceStable(found, func(i, j int) bool {
		a, b := found[i], found[j]
		if (a.Lang == lang) != (b.Lang == lang) {
			return a.Lang == lang
		}
		return a.Score > b.Score
	})
	var out []streamItem
	for _, x := range found {
		href := searchHref(lang, x)
		if href == "" {
			continue
		}
		title := x.Title
		if title == "" {
			title = untitled(lang, x.Summary, x.Kind, x.PublishedAt)
		}
		meta := i18n.T(lang, "palette.kind."+x.Kind)
		if x.Lang != lang {
			meta += " · " + x.Lang
		}
		out = append(out, streamItem{Kind: x.Kind, Glyph: searchGlyph(x.Kind), Title: title, Href: href, Meta: meta})
		if len(out) == searchLimit {
			break
		}
	}
	return out, nil
}

// searchHref is where a result opens, like the Worker's entryHref.
func searchHref(lang string, x searchHit) string {
	switch x.Kind {
	case "homelab":
		return "/" + x.Lang + "/projects/" + x.Slug
	case "link":
		if x.URL != nil {
			return *x.URL
		}
		return ""
	case "fav":
		return "/" + x.Lang + "/favs#" + x.Slug
	case "book":
		return "/" + x.Lang + "/reading#" + x.Slug
	case "page":
		return "/" + x.Lang + "/" + x.Slug
	case search.KindPhoto:
		return "/" + lang + "/photos#p-" + x.ID
	case search.KindCertificate:
		return "/" + lang + "/certificates#c-" + x.ID
	}
	if seg, ok := sections[x.Kind]; ok {
		return "/" + x.Lang + "/" + seg + "/" + x.Slug
	}
	return ""
}

func searchGlyph(kind string) string {
	switch kind {
	case search.KindPhoto:
		return "◐"
	case search.KindCertificate:
		return "✓"
	case "page":
		return "#"
	}
	return glyphs[kind]
}

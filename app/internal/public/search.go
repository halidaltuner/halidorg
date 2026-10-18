package public

import (
	"html/template"
	"log/slog"
	"net/http"
	"strings"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/search"
	"halid.org/engine/internal/store"
)

// paletteRow is one command palette result (design/Header.dc.html).
type paletteRow struct {
	Href, Title, Kind, KindLabel, Lang string
}

var paletteTmpl = template.Must(template.New("palette").Parse(
	`{{range .}}<a class="pal-row" href="{{.Href}}" role="option"{{if .Lang}} lang="{{.Lang}}"{{end}}><span class="kdot" style="--k: var(--kind-{{.Kind}})"></span><span class="t">{{.Title}}</span><span class="k">{{.KindLabel}}</span></a>{{end}}`))

// search answers the palette: GET /x/search?q=…&lang=…, an HTML fragment
// of rows. Published, public entries only (search.Filter). Results in the
// page language come first. It never goes to KV: the Worker skips URLs with
// a query string, and the response is private.
func (h *Handlers) search(w http.ResponseWriter, r *http.Request) {
	lang := r.URL.Query().Get("lang")
	if !i18n.IsSupported(lang) {
		lang = i18n.FromRequest(r)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "private, max-age=30")
	w.Header().Set("X-Robots-Tag", "noindex")
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) > 100 {
		q = string([]rune(q)[:100])
	}
	st, ok := search.Parse(q).Stmt(search.Filter{Limit: 20})
	if !ok || h.searchDB == nil {
		return
	}
	res, err := h.searchDB.Query(r.Context(), st.SQL, st.Args...)
	if err != nil {
		slog.Error("palette search", "err", err)
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
	hits, err := store.Scan[search.Hit](res)
	if err != nil {
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
	rows, err := h.paletteRows(r, lang, hits)
	if err != nil {
		slog.Error("palette titles", "err", err)
		http.Error(w, "search failed", http.StatusInternalServerError)
		return
	}
	_ = paletteTmpl.Execute(w, rows)
}

// paletteRows looks titles up in the main database (the index may lag a
// publish by a job) and keeps hits that still resolve to a public page.
func (h *Handlers) paletteRows(r *http.Request, lang string, hits []search.Hit) ([]paletteRow, error) {
	if len(hits) == 0 {
		return nil, nil
	}
	args := make([]any, len(hits))
	for i, x := range hits {
		args[i] = x.EntryID
	}
	res, err := h.db.Query(r.Context(), `SELECT id, kind, lang, slug, title FROM entries
WHERE status = 'published' AND visibility = 'public' AND id IN (?`+strings.Repeat(", ?", len(hits)-1)+`)`, args...)
	if err != nil {
		return nil, err
	}
	recs, err := store.Scan[struct {
		ID    string `json:"id"`
		Kind  string `json:"kind"`
		Lang  string `json:"lang"`
		Slug  string `json:"slug"`
		Title string `json:"title"`
	}](res)
	if err != nil {
		return nil, err
	}
	byID := map[string]int{}
	for i, x := range recs {
		byID[x.ID] = i
	}
	var mine, other []paletteRow
	for _, hit := range hits {
		i, ok := byID[hit.EntryID]
		if !ok {
			continue
		}
		x := recs[i]
		seg, ok := sections[x.Kind]
		if !ok {
			continue
		}
		row := paletteRow{Href: "/" + x.Lang + "/" + seg + "/" + x.Slug, Title: x.Title, Kind: x.Kind, KindLabel: i18n.T(lang, "palette.kind."+x.Kind)}
		if x.Lang == lang {
			mine = append(mine, row)
		} else {
			row.Lang = x.Lang
			row.KindLabel += " · " + x.Lang
			other = append(other, row)
		}
	}
	out := append(mine, other...)
	if len(out) > 10 {
		out = out[:10]
	}
	return out, nil
}

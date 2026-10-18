package public

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/store"
)

// streamKinds are the kinds in the stream, in chip order
// (design/Home.dc.html). Homelab is a project type, not an entry kind;
// GitHub releases and new repos show as project rows.
var streamKinds = []string{"post", "note", "snippet", "link", "fav", "project", "homelab"}

// streamSize is how many rows the home stream and each /stream page show.
const streamSize = 30

type streamRow struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"` // stream kind: project rows of homelab builds say homelab
	Entry       string  `json:"entry_kind"`
	Lang        string  `json:"lang"`
	Slug        string  `json:"slug"`
	Title       string  `json:"title"`
	Summary     string  `json:"summary"`
	PublishedAt *int64  `json:"published_at"`
	Minutes     *int    `json:"reading_minutes"`
	URL         *string `json:"url"`
	Domain      *string `json:"domain"`
	FavType     *string `json:"fav_type"`
	FileLang    *string `json:"file_lang"`
	Body        *string `json:"body_md"`
	Status      *string `json:"project_status"`
	Parent      *string `json:"parent"`
}

// streamCursor is where a /stream page starts: rows older than At, or as
// old with a smaller id. The zero value is the first page.
type streamCursor struct {
	At int64
	ID string
}

func (c streamCursor) String() string { return strconv.FormatInt(c.At, 10) + "." + c.ID }

// parseCursor reads ?before=; anything malformed is the first page.
func parseCursor(s string) streamCursor {
	at, id, ok := strings.Cut(s, ".")
	n, err := strconv.ParseInt(at, 10, 64)
	if !ok || err != nil || id == "" || len(id) > 120 {
		return streamCursor{}
	}
	return streamCursor{At: n, ID: id}
}

// streamRows reads up to limit rows in lang, newest first, before the
// cursor and of one kind when kind is set: published public entries of the
// stream kinds (build logs under a published project) and GitHub activity
// (releases, new repos; the same rows in both languages).
func streamRows(ctx context.Context, db store.DB, lang, kind string, before streamCursor, limit int) ([]streamRow, error) {
	where, args := "", []any{lang}
	if kind != "" {
		where += " AND kind = ?"
		args = append(args, kind)
	}
	if before.ID != "" {
		where += " AND (published_at < ? OR (published_at = ? AND id < ?))"
		args = append(args, before.At, before.At, before.ID)
	}
	ghArgs := append([]any{lang}, args[1:]...)
	res, err := db.Query(ctx, `SELECT * FROM (
SELECT e.id, CASE WHEN coalesce(pj.project_type, ppj.project_type) = 'homelab' THEN 'homelab'
    WHEN e.kind = 'buildlog' THEN 'project' ELSE e.kind END AS kind,
  e.kind AS entry_kind, e.lang, e.slug, e.title, e.summary, e.published_at,
  p.reading_minutes, l.url, l.domain, f.fav_type, pj.project_status, pe.slug AS parent,
  CASE e.kind WHEN 'snippet' THEN json_extract(r.data_json, '$.files[0].language') END AS file_lang,
  CASE WHEN e.kind = 'note' AND e.title = '' THEN r.body_md END AS body_md
FROM entries e JOIN revisions r ON r.id = e.published_revision_id
LEFT JOIN posts p ON p.entry_id = e.id LEFT JOIN links l ON l.entry_id = e.id LEFT JOIN favs f ON f.entry_id = e.id
LEFT JOIN projects pj ON pj.entry_id = e.id
LEFT JOIN buildlogs b ON b.entry_id = e.id LEFT JOIN entries pe ON pe.id = b.project_entry_id
LEFT JOIN projects ppj ON ppj.entry_id = b.project_entry_id
WHERE e.lang = ? AND e.status = 'published' AND e.visibility = 'public'
  AND e.kind IN ('post', 'note', 'snippet', 'link', 'fav', 'project', 'buildlog')
  AND (e.kind <> 'buildlog' OR (pe.status = 'published' AND pe.visibility = 'public'))
) WHERE 1 = 1`+where+`
ORDER BY published_at DESC, id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[streamRow](res)
	if err != nil {
		return nil, err
	}
	if kind != "" && kind != "project" {
		return rows, nil
	}
	// GitHub activity opens the linked project's page when there is one.
	res, err = db.Query(ctx, `SELECT * FROM (
SELECT a.id, 'project' AS kind, 'github.' || a.type AS entry_kind, a.title, a.url, a.occurred_at AS published_at,
  (SELECT e.slug FROM projects p JOIN entries e ON e.id = p.entry_id
   WHERE lower(p.github_repo) = lower(a.repo) AND e.lang = ? AND e.status = 'published' AND e.visibility = 'public'
   ORDER BY e.published_at LIMIT 1) AS parent
FROM gh_activity a JOIN gh_repos g ON g.full_name = a.repo WHERE g.gone = 0
) WHERE 1 = 1`+where+`
ORDER BY published_at DESC, id DESC LIMIT ?`, append(ghArgs, limit)...)
	if err != nil {
		return nil, err
	}
	gh, err := store.Scan[streamRow](res)
	if err != nil {
		return nil, err
	}
	for i := range gh {
		gh[i].Lang = lang
	}
	rows = append(rows, gh...)
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := at(rows[i]), at(rows[j])
		return a > b || (a == b && rows[i].ID > rows[j].ID)
	})
	return rows[:min(limit, len(rows))], nil
}

func at(r streamRow) int64 {
	if r.PublishedAt == nil {
		return 0
	}
	return *r.PublishedAt
}

var mdMarks = regexp.MustCompile("[`*_#>\\[\\]]|\\]\\([^)]*\\)")

// noteTitle is an untitled note's first line as plain text, cut short.
func noteTitle(md string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(md), "\n")
	line = strings.TrimSpace(mdMarks.ReplaceAllString(line, ""))
	if utf8.RuneCountInString(line) > 90 {
		line = strings.TrimSpace(string([]rune(line)[:88])) + "…"
	}
	return line
}

// item turns a row into a stream line: where it opens, its title and meta
// (reading time, time of day, file language, domain or fav type).
func (r streamRow) item(lang string) streamItem {
	it := streamItem{Kind: r.Kind, Glyph: glyphs[r.Kind], Title: r.Title}
	switch r.Entry {
	case "post":
		it.Href = "/" + r.Lang + "/posts/" + r.Slug
		if r.Minutes != nil {
			it.Meta = minutes(lang, *r.Minutes)
		}
	case "note":
		it.Href = "/" + r.Lang + "/notes/" + r.Slug
		if it.Title == "" {
			it.Title = r.Summary
		}
		if it.Title == "" && r.Body != nil {
			it.Title = noteTitle(*r.Body)
		}
		if r.PublishedAt != nil {
			it.Meta = time.Unix(*r.PublishedAt, 0).UTC().Format("15:04")
		}
	case "snippet":
		it.Href = "/" + r.Lang + "/snippets/" + r.Slug
		if r.FileLang != nil {
			it.Meta = *r.FileLang
		}
	case "link":
		if r.URL != nil {
			it.Href = *r.URL
		}
		if r.Domain != nil {
			it.Meta = *r.Domain
		}
	case "fav":
		it.Href = "/" + r.Lang + "/favs#" + r.Slug
		if r.FavType != nil {
			it.Meta = i18n.T(lang, "favs.type."+*r.FavType)
		}
	case "project":
		it.Href = "/" + r.Lang + "/projects/" + r.Slug
		if r.Status != nil {
			it.Meta = i18n.T(lang, "kind.project.status."+*r.Status)
		}
	case "buildlog":
		if r.Parent != nil {
			it.Href = "/" + r.Lang + "/projects/" + *r.Parent + "/log/" + r.Slug
		}
		it.Meta = i18n.T(lang, "stream.buildlog")
	case "github.release", "github.repo":
		if r.URL != nil {
			it.Href = *r.URL
		}
		if r.Parent != nil {
			it.Href = "/" + lang + "/projects/" + *r.Parent
		}
		it.Meta = i18n.T(lang, "stream."+strings.TrimPrefix(r.Entry, "github."))
	}
	if it.Title == "" {
		it.Title = r.Slug
	}
	return it
}

// stream groups the rows by day, newest first; kind filters to one kind
// (the no-JS path of the chips).
func stream(lang, kind string, rows []streamRow) []streamDay {
	var days []streamDay
	for _, r := range rows {
		if kind != "" && r.Kind != kind {
			continue
		}
		day := date(r.PublishedAt)
		it := r.item(lang)
		if n := len(days); n > 0 && days[n-1].Day == day {
			days[n-1].Items = append(days[n-1].Items, it)
			continue
		}
		days = append(days, streamDay{Day: day, Items: []streamItem{it}})
	}
	return days
}

// streamChips are "all" and the kinds the stream has, for ?kind=.
func streamChips(lang, path, kind string, rows []streamRow) []filterChip {
	has := map[string]bool{}
	for _, r := range rows {
		has[r.Kind] = true
	}
	var keys []string
	for _, k := range streamKinds {
		if has[k] {
			keys = append(keys, k)
		}
	}
	return chips(lang, path, "kind", kind, keys, func(k string) string { return i18n.T(lang, "palette.kind."+k) })
}

// streamKind reads ?kind= when it names a stream kind.
func streamKind(r *http.Request) string {
	k := r.URL.Query().Get("kind")
	for _, s := range streamKinds {
		if s == k {
			return k
		}
	}
	return ""
}

// --- /stream (derive: the home stream as its own page) ----------------------

type streamPage struct {
	page
	Stream     []streamDay
	Chips      []filterChip
	StreamKind string
	Older      string // the next page's URL, empty on the last
	Newest     string // the first page's URL, on later pages
}

// streamPage serves /{lang}/stream: 30 rows a page, ?kind= filters in the
// query (so pages stay full), ?before= continues after a row. Only the
// first page is cached at the edge (a query string bypasses KV).
func (h *Handlers) streamPage(w http.ResponseWriter, r *http.Request, lang string) {
	kind := streamKind(r)
	before := parseCursor(r.URL.Query().Get("before"))
	rows, err := streamRows(r.Context(), h.db, lang, kind, before, streamSize+1)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	more := len(rows) > streamSize
	rows = rows[:min(streamSize, len(rows))]
	p := streamPage{page: h.newPage(lang, "", i18n.T(lang, "stream.title")+" · halid.org", "/stream", nil), StreamKind: kind}
	p.Head.Description = i18n.T(lang, "stream.sub")
	if before.ID != "" || kind != "" {
		p.Head.Noindex = true
	}
	p.Stream = stream(lang, "", rows)
	path := "/" + lang + "/stream"
	// Chips list every stream kind: a page holds only some of them.
	p.Chips = chips(lang, path, "kind", kind, streamKinds, func(k string) string { return i18n.T(lang, "palette.kind."+k) })
	q := url.Values{}
	if kind != "" {
		q.Set("kind", kind)
	}
	if before.ID != "" {
		p.Newest = path
		if kind != "" {
			p.Newest += "?" + q.Encode()
		}
	}
	if more {
		last := rows[len(rows)-1]
		q.Set("before", streamCursor{At: at(last), ID: last.ID}.String())
		p.Older = path + "?" + q.Encode()
	}
	h.render(w, r, "stream", p)
}

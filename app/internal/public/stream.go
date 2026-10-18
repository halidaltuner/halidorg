package public

import (
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/store"
)

// streamKinds are the kinds in the home stream, in chip order
// (design/Home.dc.html; projects and homelab arrive in Phase 4).
var streamKinds = []string{"post", "note", "snippet", "link", "fav"}

// streamSize is how many entries the home stream shows.
const streamSize = 30

type streamRow struct {
	Kind        string  `json:"kind"`
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
}

// streamRows reads the newest published public entries of the stream kinds
// in lang, with what each row's meta needs.
func streamRows(ctx context.Context, db store.DB, lang string) ([]streamRow, error) {
	res, err := db.Query(ctx, `SELECT e.kind, e.lang, e.slug, e.title, e.summary, e.published_at,
  p.reading_minutes, l.url, l.domain, f.fav_type,
  CASE e.kind WHEN 'snippet' THEN json_extract(r.data_json, '$.files[0].language') END AS file_lang,
  CASE WHEN e.kind = 'note' AND e.title = '' THEN r.body_md END AS body_md
FROM entries e JOIN revisions r ON r.id = e.published_revision_id
LEFT JOIN posts p ON p.entry_id = e.id LEFT JOIN links l ON l.entry_id = e.id LEFT JOIN favs f ON f.entry_id = e.id
WHERE e.lang = ? AND e.status = 'published' AND e.visibility = 'public'
  AND e.kind IN ('post', 'note', 'snippet', 'link', 'fav')
ORDER BY e.published_at DESC, e.id DESC LIMIT ?`, lang, streamSize)
	if err != nil {
		return nil, err
	}
	return store.Scan[streamRow](res)
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
	switch r.Kind {
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
func streamChips(lang, kind string, rows []streamRow) []filterChip {
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
	return chips(lang, "/"+lang+"/", "kind", kind, keys, func(k string) string { return i18n.T(lang, "palette.kind."+k) })
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

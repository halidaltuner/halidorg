package search

import (
	"strings"
	"unicode"

	"halid.org/engine/internal/store"
)

// Doc is one row of search_fts. Text fields are raw; Upsert folds them.
type Doc struct {
	EntryID    string
	Kind       string
	Lang       string
	Status     string
	Visibility string
	Slug       string
	Title      string
	Summary    string
	Body       string // plain text, see render.Output.Text
	Tags       []string
}

// Upsert replaces the entry's row. FTS5 has no upsert, so it is a delete
// plus an insert; run both in one Batch on the SEARCH database.
func (d Doc) Upsert() []store.Stmt {
	return []store.Stmt{
		Delete(d.EntryID),
		{
			SQL: `INSERT INTO search_fts (entry_id, kind, lang, status, visibility, slug, title, summary, body, tags)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			Args: []any{d.EntryID, d.Kind, d.Lang, d.Status, d.Visibility, d.Slug,
				Fold(d.Title), Fold(d.Summary), Fold(d.Body), Fold(strings.Join(d.Tags, " "))},
		},
	}
}

// Delete removes the entry's row.
func Delete(entryID string) store.Stmt {
	return store.Stmt{SQL: `DELETE FROM search_fts WHERE entry_id = ?`, Args: []any{entryID}}
}

// kindPrefixes are the command palette filters (design/README.md).
var kindPrefixes = map[string]string{
	"p":  "post",
	"n":  "note",
	"s":  "snippet",
	"l":  "link",
	"f":  "fav",
	"pr": "project",
	"h":  "homelab", // homelab builds: projects indexed under their own kind
}

// Query is a parsed user query.
type Query struct {
	Match string // FTS5 MATCH expression; empty means nothing to search
	Kind  string // from a prefix filter, empty for all kinds
}

// Parse turns "pr: go htm" into Kind "project" and Match `"go" "htm"*`.
// Tokens are folded and stripped to letters and digits, every token must
// match (implicit AND), and the last one matches as a prefix so results
// follow typing. User input never reaches FTS5 syntax unquoted.
func Parse(raw string) Query {
	var q Query
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, ":"); i > 0 {
		if k, ok := kindPrefixes[strings.ToLower(s[:i])]; ok {
			q.Kind = k
			s = s[i+1:]
		}
	}
	var toks []string
	for _, f := range strings.FieldsFunc(Fold(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		toks = append(toks, `"`+f+`"`)
	}
	if len(toks) == 0 {
		return q
	}
	toks[len(toks)-1] += "*"
	q.Match = strings.Join(toks, " ")
	return q
}

// Filter limits a search.
type Filter struct {
	Lang   string // empty for both languages
	Drafts bool   // admin search: include drafts, scheduled and private rows
	Limit  int    // default 20, max 50
}

// Hit is one search result row.
type Hit struct {
	EntryID string  `json:"entry_id"`
	Kind    string  `json:"kind"`
	Lang    string  `json:"lang"`
	Slug    string  `json:"slug"`
	Status  string  `json:"status"`
	Rank    float64 `json:"rank"`
}

// Stmt builds the search statement, or ok=false when there is nothing to
// match. bm25 weights follow the column order of search_fts; unindexed
// columns get 0, then title > tags > summary > body.
func (q Query) Stmt(f Filter) (store.Stmt, bool) {
	if q.Match == "" {
		return store.Stmt{}, false
	}
	limit := f.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 50 {
		limit = 50
	}
	var b strings.Builder
	args := []any{q.Match}
	b.WriteString(`SELECT entry_id, kind, lang, slug, status,
  bm25(search_fts, 0, 0, 0, 0, 0, 0, 10.0, 4.0, 1.0, 6.0) AS rank
FROM search_fts WHERE search_fts MATCH ?`)
	if q.Kind != "" {
		b.WriteString(` AND kind = ?`)
		args = append(args, q.Kind)
	}
	if f.Lang != "" {
		b.WriteString(` AND lang = ?`)
		args = append(args, f.Lang)
	}
	if !f.Drafts {
		b.WriteString(` AND status = 'published' AND visibility = 'public'`)
	}
	b.WriteString(` ORDER BY rank LIMIT ?`)
	args = append(args, limit)
	return store.Stmt{SQL: b.String(), Args: args}, true
}

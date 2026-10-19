package content

import (
	"context"
	"html"
	"regexp"
	"strings"

	"halid.org/engine/internal/store"
)

// Link is one internal link found in rendered HTML: the page it points at
// and the link text.
type Link struct {
	Lang, Section, Slug string
	Text                string
}

var (
	anchorRE  = regexp.MustCompile(`(?is)<a\s[^>]*href="([^"]+)"[^>]*>(.*?)</a>`)
	tagRE     = regexp.MustCompile(`<[^>]+>`)
	sitePathR = regexp.MustCompile(`^/(tr|en)/(posts|notes|snippets|projects|presentations|garden|runbooks|glossary)/([a-z0-9-]{1,80})/?(?:[?#].*)?$`)
)

// ExtractLinks returns the internal entry links in body, once per target, in
// order. origin is the site origin ("https://halid.org"); absolute links on
// it count as internal.
func ExtractLinks(body, origin string) []Link {
	seen := map[string]bool{}
	var out []Link
	for _, m := range anchorRE.FindAllStringSubmatch(body, -1) {
		href := html.UnescapeString(m[1])
		if origin != "" {
			href = strings.TrimPrefix(href, origin)
		}
		p := sitePathR.FindStringSubmatch(href)
		if p == nil {
			continue
		}
		key := p[1] + "/" + p[2] + "/" + p[3]
		if seen[key] {
			continue
		}
		seen[key] = true
		text := strings.Join(strings.Fields(html.UnescapeString(tagRE.ReplaceAllString(m[2], ""))), " ")
		if r := []rune(text); len(r) > 140 {
			text = string(r[:140])
		}
		out = append(out, Link{Lang: p[1], Section: p[2], Slug: p[3], Text: text})
	}
	return out
}

// sectionKind maps a URL section to entries.kind.
var sectionKind = map[string]string{"posts": "post", "notes": "note", "snippets": "snippet", "projects": "project",
	"presentations": "presentation", "garden": "garden", "runbooks": "runbook", "glossary": "term"}

// PageKinds are the kinds with a page of their own, in SQL: the ones that
// show backlinks and related entries, and can be related.
const PageKinds = `('post', 'note', 'snippet', 'project', 'presentation', 'garden', 'runbook', 'term')`

// RelatedEntry is a published entry sharing tags with another.
type RelatedEntry struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Lang    string `json:"lang"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Summary string `json:"summary"`
	Shared  int    `json:"shared"`
	// PublishedAt names an entry with no title and no summary (a note).
	PublishedAt *int64 `json:"published_at"`
}

// Related lists up to n published public entries in lang sharing the most
// tags with entryID, newest first among equals. Title falls back to the
// summary (notes have none).
func Related(ctx context.Context, db store.DB, entryID, lang string, n int) ([]RelatedEntry, error) {
	res, err := db.Query(ctx, `SELECT e.id, e.kind, e.lang, e.slug, CASE WHEN e.title = '' THEN e.summary ELSE e.title END AS title, e.summary, e.published_at, count(*) AS shared
FROM entry_tags mine JOIN entry_tags other ON other.tag_id = mine.tag_id AND other.entry_id <> mine.entry_id
JOIN entries e ON e.id = other.entry_id
WHERE mine.entry_id = ? AND e.lang = ? AND e.status = 'published' AND e.visibility = 'public' AND e.kind IN `+PageKinds+`
GROUP BY e.id ORDER BY shared DESC, e.published_at DESC, e.id LIMIT ?`, entryID, lang, n)
	if err != nil {
		return nil, err
	}
	return store.Scan[RelatedEntry](res)
}

// SetLinksStmts replaces the outgoing links of entryID. Targets are looked
// up by kind, language and slug; links to entries that do not exist are
// dropped by the INSERT ... SELECT.
func SetLinksStmts(entryID string, links []Link) []store.Stmt {
	stmts := []store.Stmt{{SQL: `DELETE FROM links_graph WHERE from_entry_id = ?`, Args: []any{entryID}}}
	for _, l := range links {
		kind, ok := sectionKind[l.Section]
		if !ok {
			continue
		}
		stmts = append(stmts, store.Stmt{
			SQL: `INSERT OR IGNORE INTO links_graph (from_entry_id, to_entry_id, context)
SELECT ?, id, ? FROM entries WHERE kind = ? AND lang = ? AND slug = ? AND id <> ?`,
			Args: []any{entryID, l.Text, kind, l.Lang, l.Slug, entryID},
		})
	}
	return stmts
}

// LinkTargets returns the ids an entry links to now.
func LinkTargets(ctx context.Context, db store.DB, entryID string) ([]string, error) {
	res, err := db.Query(ctx, `SELECT to_entry_id FROM links_graph WHERE from_entry_id = ?`, entryID)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[struct {
		ID string `json:"to_entry_id"`
	}](res)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	return ids, nil
}

// Backlink is an entry that links to another.
type Backlink struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Lang    string `json:"lang"`
	Slug    string `json:"slug"`
	Title   string `json:"title"`
	Context string `json:"context"`
	Status  string `json:"status"`
	// PublishedAt names an entry with no title and no summary (a note).
	PublishedAt *int64 `json:"published_at"`
}

// Backlinks lists the entries linking to entryID. publicOnly keeps published
// public ones, for public pages; admin counts every source.
func Backlinks(ctx context.Context, db store.DB, entryID string, publicOnly bool) ([]Backlink, error) {
	q := `SELECT e.id, e.kind, e.lang, e.slug, CASE WHEN e.title = '' THEN e.summary ELSE e.title END AS title, l.context, e.status, e.published_at
FROM links_graph l JOIN entries e ON e.id = l.from_entry_id
WHERE l.to_entry_id = ?`
	if publicOnly {
		q += ` AND e.status = 'published' AND e.visibility = 'public'`
	}
	q += ` ORDER BY e.published_at DESC, e.id`
	res, err := db.Query(ctx, q, entryID)
	if err != nil {
		return nil, err
	}
	return store.Scan[Backlink](res)
}

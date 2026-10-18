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
	sitePathR = regexp.MustCompile(`^/(tr|en)/(posts)/([a-z0-9-]{1,80})/?(?:[?#].*)?$`)
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
var sectionKind = map[string]string{"posts": "post"}

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
}

// Backlinks lists the entries linking to entryID. publicOnly keeps published
// public ones, for public pages; admin counts every source.
func Backlinks(ctx context.Context, db store.DB, entryID string, publicOnly bool) ([]Backlink, error) {
	q := `SELECT e.id, e.kind, e.lang, e.slug, e.title, l.context, e.status
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

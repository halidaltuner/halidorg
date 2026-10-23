// Package syndication keeps the POSSE copies of an entry (Phase 8.3): the
// URLs of its posts on X, LinkedIn and elsewhere, pasted in the editor and
// linked from the entry's page as u-syndication. Nothing is posted from
// here; the editor builds share links that open each network's composer.
package syndication

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store"
)

// Max is how many copies one entry keeps.
const Max = 10

// Link is one copy.
type Link struct {
	Network string `json:"network"`
	URL     string `json:"url"`
}

// networks maps hosts to the names the page shows.
var networks = map[string]string{
	"x.com": "x", "twitter.com": "x", "linkedin.com": "linkedin", "bsky.app": "bluesky",
	"mastodon.social": "mastodon", "fosstodon.org": "mastodon", "hachyderm.io": "mastodon",
}

// Network names the site a URL is on: a known network, else its host.
func Network(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	if n, ok := networks[host]; ok {
		return n
	}
	if strings.HasSuffix(host, ".linkedin.com") {
		return "linkedin"
	}
	return host
}

// labels are how the page names known networks.
var labels = map[string]string{"x": "X", "linkedin": "LinkedIn", "bluesky": "Bluesky", "mastodon": "Mastodon"}

// Label is the name the page shows for a network.
func Label(network string) string {
	if l, ok := labels[network]; ok {
		return l
	}
	return network
}

// Share is the editor's share links for a published entry: each opens the
// network's composer with the entry's URL; X also takes the text.
type Share struct {
	URL, Text, X, LinkedIn string
}

// NewShare builds the share links for permalink and text.
func NewShare(permalink, text string) Share {
	return Share{URL: permalink, Text: text,
		X:        XIntent(text, permalink),
		LinkedIn: "https://www.linkedin.com/sharing/share-offsite/?url=" + escape(permalink)}
}

// XIntent is X's composer with text and a link.
func XIntent(text, permalink string) string {
	return "https://x.com/intent/tweet?text=" + escape(text) + "&url=" + escape(permalink)
}

// escape is a query value with spaces as %20, as encodeURIComponent writes
// it (share.js), not +.
func escape(s string) string { return strings.ReplaceAll(url.QueryEscape(s), "+", "%20") }

// Parse reads the editor field: one URL per line, blank lines ignored,
// duplicates once. It returns a user-facing error naming the first line
// that is not an absolute http(s) URL.
func Parse(field string) ([]string, error) {
	var out []string
	seen := map[string]bool{}
	for _, line := range strings.Split(strings.ReplaceAll(field, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		u, err := url.Parse(line)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || len(line) > 2048 {
			return nil, fmt.Errorf("%q", line)
		}
		if !seen[line] {
			seen[line] = true
			out = append(out, line)
		}
	}
	if len(out) > Max {
		return nil, fmt.Errorf("max %d", Max)
	}
	return out, nil
}

// Repo stores syndication links.
type Repo struct{ db store.DB }

// NewRepo returns a repo on the main database.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

// List returns the entry's copies in the order they were entered.
func (r *Repo) List(ctx context.Context, entryID string) ([]Link, error) {
	res, err := r.db.Query(ctx, `SELECT network, url FROM syndications WHERE entry_id = ? ORDER BY sort, posted_at`, entryID)
	if err != nil {
		return nil, err
	}
	return store.Scan[Link](res)
}

// Set replaces the entry's copies with urls and reports whether anything
// changed. URLs kept keep their posted_at.
func (r *Repo) Set(ctx context.Context, entryID string, urls []string) (bool, error) {
	have, err := r.List(ctx, entryID)
	if err != nil {
		return false, err
	}
	same := len(have) == len(urls)
	for i := 0; same && i < len(urls); i++ {
		same = have[i].URL == urls[i]
	}
	if same {
		return false, nil
	}
	now := content.Clock().Unix()
	stmts := []store.Stmt{{SQL: `DELETE FROM syndications WHERE entry_id = ? AND url NOT IN (SELECT value FROM json_each(?))`,
		Args: []any{entryID, jsonList(urls)}}}
	for i, u := range urls {
		stmts = append(stmts, store.Stmt{SQL: `INSERT INTO syndications (entry_id, network, url, posted_at, sort) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (entry_id, url) DO UPDATE SET sort = excluded.sort, network = excluded.network`, Args: []any{entryID, Network(u), u, now, i}})
	}
	_, err = r.db.Batch(ctx, stmts...)
	return err == nil, err
}

func jsonList(s []string) string {
	if s == nil {
		s = []string{}
	}
	b, _ := json.Marshal(s)
	return string(b)
}

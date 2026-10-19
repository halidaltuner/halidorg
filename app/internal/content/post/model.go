// Package post is the long-form article kind (CONTENT_MODEL.md §2 post),
// and the long-form kinds written in the same editor: garden pages and
// runbooks (Phase 5.5). Each has a posts row; garden and runbook add their
// own.
package post

import (
	"math"
	"strings"

	"halid.org/engine/internal/content"
)

// Kind is the entries.kind value of posts.
const Kind = "post"

// Long-form kinds besides posts.
const (
	Garden  = "garden"
	Runbook = "runbook"
)

// Kinds are the long-form kinds, in editor order, and the URL segment of
// each.
var Kinds = []string{Kind, Garden, Runbook}

var sections = map[string]string{Kind: "posts", Garden: "garden", Runbook: "runbooks"}

// Section is the URL segment of a long-form kind ("posts", "garden",
// "runbooks"), or "".
func Section(kind string) string { return sections[kind] }

// Maturities are a garden page's stages; Risks a runbook's.
var (
	Maturities = []string{"seedling", "budding", "evergreen"}
	Risks      = []string{"low", "med", "high"}
)

// Post is an entry plus the posts row, and the garden or runbook row.
type Post struct {
	content.Entry
	ReadingMinutes  int    `json:"reading_minutes"`
	CanonicalURL    string `json:"canonical_url"`
	Mermaid         bool   `json:"mermaid"`
	CommentsEnabled bool   `json:"comments_enabled"`

	// Garden pages.
	Maturity     string `json:"maturity"`
	LastTendedAt *int64 `json:"last_tended_at"`
	// Runbooks.
	Platform      string `json:"platform"`
	Prerequisites string `json:"prerequisites"`
	VerifiedOn    *int64 `json:"verified_on"`
	Risk          string `json:"risk"`
}

// Path is the public path of p.
func (p Post) Path() string { return "/" + p.Lang + "/" + Section(p.Kind) + "/" + p.Slug }

// Prereqs are a runbook's prerequisites, one per non-empty line.
func (p Post) Prereqs() []string {
	var out []string
	for _, l := range strings.Split(p.Prerequisites, "\n") {
		if l = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "- ")); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// wordsPerMinute sits between typical English and Turkish technical reading
// speeds; code is excluded from the word count by the renderer.
const wordsPerMinute = 220

// ReadingMinutes is the reading time for a word count, at least 1.
func ReadingMinutes(words int) int {
	if words <= 0 {
		return 1
	}
	return int(math.Ceil(float64(words) / wordsPerMinute))
}

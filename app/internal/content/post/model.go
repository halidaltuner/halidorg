// Package post is the long-form article kind (CONTENT_MODEL.md §2 post).
package post

import (
	"math"

	"halid.org/engine/internal/content"
)

// Kind is the entries.kind value.
const Kind = "post"

// Post is an entry plus the posts row.
type Post struct {
	content.Entry
	ReadingMinutes  int    `json:"reading_minutes"`
	CanonicalURL    string `json:"canonical_url"`
	Mermaid         bool   `json:"mermaid"`
	CommentsEnabled bool   `json:"comments_enabled"`
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

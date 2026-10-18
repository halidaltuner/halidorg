// Package fav is the recommendations kind: tools, repos, videos, books and
// more, with an optional 1-5 rating (CONTENT_MODEL.md §2 fav). Favs have no
// page of their own; they live on /{lang}/favs.
package fav

import "halid.org/engine/internal/content"

// Kind is the entries.kind value.
const Kind = "fav"

// Types lists fav_type values in display order.
var Types = []string{"tool", "repo", "video", "product", "article", "book", "place", "other"}

// Fav is an entry plus the favs row. Entry.Summary is why it is a fav.
type Fav struct {
	content.Entry
	Type       string  `json:"fav_type"`
	Author     string  `json:"author"`
	URL        string  `json:"url"`
	Rating     *int    `json:"rating"`
	CategoryID *string `json:"category_id"`
}

// Draft is the input for Create and Save; Markdown is an optional longer note.
type Draft struct {
	Fav
	Markdown string
	Tags     []string
}

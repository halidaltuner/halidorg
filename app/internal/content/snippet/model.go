// Package snippet is the code kind: one or more files shown as tabs, with a
// Markdown description (CONTENT_MODEL.md §2 snippet).
package snippet

import "halid.org/engine/internal/content"

// Kind is the entries.kind value.
const Kind = "snippet"

// File is one tab of a snippet.
type File struct {
	Filename string `json:"filename"`
	Language string `json:"language"`
	Code     string `json:"code"`
	Sort     int    `json:"sort"`
}

// Snippet is an entry plus its snippets row and files.
type Snippet struct {
	content.Entry
	CategoryID *string `json:"category_id"`
	GistURL    string  `json:"gist_url"`
	Files      []File  `json:"-"`
	// HTML is the description of the published revision (GetPublished only).
	HTML string `json:"-"`
}

// Item is one row of the public snippet list.
type Item struct {
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Lang        string `json:"lang"` // differs from the page's on a fallback row
	PublishedAt *int64 `json:"published_at"`
	Language    string `json:"language"` // first file's language
}

// Draft is the input for Create and Save: Markdown is the description.
type Draft struct {
	Snippet
	Markdown string
	Tags     []string
}

// Package note is the short-form kind: a TIL or a quick thought, title
// optional (CONTENT_MODEL.md §2 note).
package note

import "halid.org/engine/internal/content"

// Kind is the entries.kind value.
const Kind = "note"

// Note is an entry plus the notes row.
type Note struct {
	content.Entry
	SourceURL string `json:"source_url"`
	TIL       bool   `json:"til"`
	// HTML is the body of the published revision (GetPublished and List).
	HTML string `json:"-"`
}

// Draft is the input for Create and Save.
type Draft struct {
	Note
	Markdown string
	Tags     []string
}

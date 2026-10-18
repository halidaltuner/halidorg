// Package link is the bookmark kind: a URL with a title and my comment,
// grouped by category, read or unread (CONTENT_MODEL.md §2 link). Links have
// no page of their own; they live on /{lang}/links.
package link

import "halid.org/engine/internal/content"

// Kind is the entries.kind value.
const Kind = "link"

// Read statuses.
const (
	Unread = "unread"
	Read   = "read"
)

// Link is an entry plus the links row. Entry.Title is the link title,
// Entry.Summary is my comment.
type Link struct {
	content.Entry
	URL         string  `json:"url"`
	Domain      string  `json:"domain"`
	FaviconURL  string  `json:"favicon_url"`
	CategoryID  *string `json:"category_id"`
	Via         string  `json:"via"`
	ArchivedURL string  `json:"archived_url"`
	ReadStatus  string  `json:"read_status"`
}

// Draft is the input for Create and Save; Markdown is an optional longer note.
type Draft struct {
	Link
	Markdown string
	Tags     []string
}

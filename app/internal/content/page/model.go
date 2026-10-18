// Package page is the page kind: About and Now, one of each per language
// (Phase 5.1, CONTENT_MODEL.md §2). The entry's slug is the template; the
// body is the About bio, and the sections live in the revision's data_json,
// so they go live on publish like the text. Every publish is kept as a
// version for /now/archive.
package page

import "halid.org/engine/internal/content"

// Kind is the entries.kind of pages.
const Kind = "page"

// Templates.
const (
	About = "about"
	Now   = "now"
)

// Templates lists them in admin order.
var Templates = []string{About, Now}

// Link is a button: label and URL (https, mailto: or a site path).
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
}

// SkillGroup is one About skill tile.
type SkillGroup struct {
	Group string   `json:"group"`
	Tone  string   `json:"tone"`
	Items []string `json:"items"`
}

// Job is one experience timeline row; From and To are free text ("2023",
// "now").
type Job struct {
	From string `json:"from"`
	To   string `json:"to"`
	Role string `json:"role"`
	Org  string `json:"org"`
	Note string `json:"note"`
	Tone string `json:"tone"`
}

// Build is one Now "building" card.
type Build struct {
	Title string `json:"title"`
	Note  string `json:"note"`
	Stage string `json:"stage"`
	Pct   int    `json:"pct"`
	Tone  string `json:"tone"`
	Href  string `json:"href"`
	Cover string `json:"cover"` // media id
}

// Book is one Now "reading" cover.
type Book struct {
	Title  string `json:"title"`
	Author string `json:"author"`
	Cover  string `json:"cover"` // media id
	Pct    int    `json:"pct"`
	Tone   string `json:"tone"`
}

// Learning is one Now "learning" card; Level is 1-10.
type Learning struct {
	Title string `json:"title"`
	Note  string `json:"note"`
	Level int    `json:"level"`
	Hours int    `json:"hours"`
	Tone  string `json:"tone"`
}

// Data is a page's data_json; each template uses its own fields.
type Data struct {
	// about
	Name       string       `json:"name,omitempty"`
	Portrait   string       `json:"portrait,omitempty"` // media id
	CV         string       `json:"cv,omitempty"`       // media id of a PDF
	CTAs       []Link       `json:"ctas,omitempty"`
	Skills     []SkillGroup `json:"skills,omitempty"`
	Experience []Job        `json:"experience,omitempty"`
	// now
	Place    string     `json:"place,omitempty"`
	Building []Build    `json:"building,omitempty"`
	Reading  []Book     `json:"reading,omitempty"`
	Learning []Learning `json:"learning,omitempty"`
	NotDoing []string   `json:"not_doing,omitempty"`
}

// Page is a page entry with its template and data.
type Page struct {
	content.Entry
	Template string `json:"template"`
	Data     Data   `json:"-"`
	// HTML is the published body (GetPublished only).
	HTML string `json:"-"`
}

// Draft is a page as written.
type Draft struct {
	Page
	Markdown string
	Tags     []string
}

// Version is one published version.
type Version struct {
	RevisionID  string `json:"revision_id"`
	PublishedAt int64  `json:"published_at"`
}

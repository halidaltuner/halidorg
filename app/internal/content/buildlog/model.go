// Package buildlog is the build log kind: dated entries of one project,
// shown as its timeline and at /{lang}/projects/{project}/log/{slug}
// (CONTENT_MODEL.md §2, Phase 4.1).
package buildlog

import "halid.org/engine/internal/content"

// Kind is the entries.kind of build logs.
const Kind = "buildlog"

// Buildlog is one log entry.
type Buildlog struct {
	content.Entry
	ProjectID string   `json:"project_entry_id"`
	LogDate   int64    `json:"log_date"`
	Hours     *float64 `json:"hours_spent"`
	// ProjectSlug and ProjectTitle are read with it, for links and lists.
	ProjectSlug  string `json:"project_slug"`
	ProjectTitle string `json:"project_title"`
}

// Draft is a build log as written.
type Draft struct {
	Buildlog
	Markdown string
	Tags     []string
}

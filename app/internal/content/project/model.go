// Package project is the project kind: software projects and homelab / DIY
// builds share one model and one detail template (CONTENT_MODEL.md §2,
// Phase 4.1). Build logs are their own kind (content/buildlog).
package project

import "halid.org/engine/internal/content"

// Kind is the entries.kind of projects.
const Kind = "project"

// Project types.
const (
	Software = "software"
	Homelab  = "homelab"
)

// Statuses are the project states, from the prototypes.
var Statuses = []string{"active", "maintained", "running", "in_progress", "paused", "done", "idea", "archived"}

// Fact is one fact strip cell, e.g. "nodes: 3".
type Fact struct {
	Key   string `json:"key"`
	Value string `json:"value"`
	Sort  int    `json:"sort"`
}

// Part is one line of a homelab build's parts list. Price is for the whole
// line, in minor units (kuruş, cents).
type Part struct {
	Name     string `json:"name"`
	Qty      int    `json:"qty"`
	Price    *int64 `json:"price"`
	Currency string `json:"currency"`
	URL      string `json:"url"`
	Sort     int    `json:"sort"`
}

// Link is one hero button; the first is the primary one.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Sort  int    `json:"sort"`
}

// Project is a project with its rows.
type Project struct {
	content.Entry
	Type          string  `json:"project_type"`
	ProjectStatus string  `json:"project_status"`
	CategoryID    *string `json:"category_id"`
	Tagline       string  `json:"tagline"`
	KindLine      string  `json:"kind_line"`
	GitHubRepo    string  `json:"github_repo"`
	StartedAt     *int64  `json:"started_at"`
	EndedAt       *int64  `json:"ended_at"`
	Featured      bool    `json:"featured"`
	CostTotal     *int64  `json:"cost_total"`
	Currency      string  `json:"currency"`
	TimeSpent     string  `json:"time_spent"`

	Facts   []Fact   `json:"-"`
	Stack   []string `json:"-"`
	Parts   []Part   `json:"-"`
	Links   []Link   `json:"-"`
	Gallery []string `json:"-"` // media ids, in order

	// HTML is the body of the published revision (GetPublished only).
	HTML string `json:"-"`
}

// Draft is a project as written: the entry, its rows, its Markdown and tags.
type Draft struct {
	Project
	Markdown string
	Tags     []string
}

// Cost is the build's cost in minor units: cost_total when set, else the
// sum of the priced parts in the project currency. ok is false when there
// is nothing to show.
func (p Project) Cost() (amount int64, ok bool) {
	if p.CostTotal != nil {
		return *p.CostTotal, true
	}
	for _, x := range p.Parts {
		if x.Price != nil && x.Currency == p.Currency {
			amount += *x.Price
			ok = true
		}
	}
	return amount, ok
}

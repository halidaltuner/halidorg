// Package presentation is the presentation kind (CONTENT_MODEL.md §2,
// Phase 5.4): talks, workshops and lightning sessions with a self-contained
// HTML deck served from an isolated origin (ADR-007).
package presentation

import (
	"os"
	"strings"
	"time"

	"halid.org/engine/internal/content"
)

// Kind is the entries.kind of presentations.
const Kind = "presentation"

// Types are the presentation types, in filter order.
var Types = []string{"talk", "workshop", "lightning"}

// Slide is one outline row: a slide number and its title.
type Slide struct {
	N     int    `json:"n"`
	Title string `json:"title"`
}

// Link is one row of the "related" list.
type Link struct {
	Label string `json:"label"`
	URL   string `json:"url"`
	Sort  int    `json:"sort"`
}

// Presentation is a presentation with its rows.
type Presentation struct {
	content.Entry
	Type             string  `json:"pres_type"`
	Event            string  `json:"event"`
	City             string  `json:"city"`
	HeldOn           *int64  `json:"held_on"`
	TalkLang         string  `json:"talk_lang"`
	SlideCount       int     `json:"slide_count"`
	DeckKey          *string `json:"deck_r2_key"`
	DeckVersion      *string `json:"deck_version"`
	RecordingURL     *string `json:"recording_url"`
	RecordingMinutes *int    `json:"recording_minutes"`

	Outline []Slide `json:"-"`
	Links   []Link  `json:"-"`

	// HTML is the body of the published revision (GetPublished only).
	HTML string `json:"-"`
}

// Draft is a presentation as written: the entry, its rows, its Markdown
// and tags.
type Draft struct {
	Presentation
	Markdown string
	Tags     []string
}

// DecksOrigin is the isolated origin decks are served from (DECKS_ORIGIN,
// e.g. https://decks.halid.org), or "" when it is not configured.
func DecksOrigin() string { return strings.TrimRight(os.Getenv("DECKS_ORIGIN"), "/") }

// DeckKey is where an entry's deck lives in R2.
func DeckKey(id string) string { return "decks/" + id + "/index.html" }

// DeckURL is the deck's address on the decks origin, versioned so a new
// upload is not served from a cache; "" without a deck or an origin.
func (p Presentation) DeckURL() string {
	o := DecksOrigin()
	if o == "" || p.DeckKey == nil {
		return ""
	}
	u := o + "/" + p.ID + "/"
	if p.DeckVersion != nil {
		u += "?v=" + *p.DeckVersion
	}
	return u
}

// DownloadURL serves the same deck as an attachment named after the slug.
func (p Presentation) DownloadURL() string {
	o := DecksOrigin()
	if o == "" || p.DeckKey == nil {
		return ""
	}
	u := o + "/" + p.ID + "/" + p.Slug + ".html"
	if p.DeckVersion != nil {
		u += "?v=" + *p.DeckVersion
	}
	return u
}

// Day writes the day it was given for a date field ("2026-09-21").
func Day(v *int64) string {
	if v == nil || *v <= 0 {
		return ""
	}
	return time.Unix(*v, 0).UTC().Format("2006-01-02")
}

// day reads a date field as midnight, unix UTC; -1 when it is not a date,
// which Validate reports.
func day(s string) int64 {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return -1
	}
	return t.Unix()
}

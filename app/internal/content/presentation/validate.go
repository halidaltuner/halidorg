package presentation

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
)

// Validate returns nil or a content.ValidationError with every problem.
func Validate(p Presentation) error {
	v := content.Validate(p.Entry, false)
	if p.Kind != Kind {
		v["kind"] = "must be presentation"
	}
	if !slices.Contains(Types, p.Type) {
		v["pres_type"] = "talk, workshop or lightning"
	}
	if p.TalkLang != "tr" && p.TalkLang != "en" {
		v["talk_lang"] = "tr or en"
	}
	for f, s := range map[string]struct {
		val string
		max int
	}{"event": {p.Event, 120}, "city": {p.City, 80}} {
		if utf8.RuneCountInString(s.val) > s.max {
			v[f] = fmt.Sprintf("at most %d characters", s.max)
		}
	}
	if p.HeldOn != nil && *p.HeldOn <= 0 {
		v["held_on"] = "a date"
	}
	if p.SlideCount < 0 || p.SlideCount > 2000 {
		v["slide_count"] = "0 to 2000"
	}
	if p.RecordingURL != nil && !content.HTTPSURL(*p.RecordingURL) {
		v["recording_url"] = "must be an absolute https URL"
	}
	if p.RecordingMinutes != nil && (*p.RecordingMinutes < 1 || *p.RecordingMinutes > 1000) {
		v["recording_minutes"] = "1 to 1000"
	}
	for i, s := range p.Outline {
		switch {
		case s.N < 1 || p.SlideCount > 0 && s.N > p.SlideCount:
			v[fmt.Sprintf("outline.%d.n", i)] = "a slide of the deck"
		case s.Title == "" || utf8.RuneCountInString(s.Title) > 120:
			v[fmt.Sprintf("outline.%d.title", i)] = "1 to 120 characters"
		}
	}
	for i, l := range p.Links {
		switch {
		case l.Label == "" || utf8.RuneCountInString(l.Label) > 120:
			v[fmt.Sprintf("links.%d.label", i)] = "1 to 120 characters"
		case !linkURL(l.URL):
			v[fmt.Sprintf("links.%d.url", i)] = "an https URL or a path here"
		}
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

// linkURL accepts another site over https or a page here.
func linkURL(u string) bool {
	return content.HTTPSURL(u) || strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") && !strings.ContainsAny(u, " \\")
}

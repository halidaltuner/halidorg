package page

import (
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
)

// LinkURL reports whether s is a URL a page may link to: https, mailto: or
// a path on this site.
func LinkURL(s string) bool {
	switch {
	case strings.HasPrefix(s, "mailto:"):
		return len(s) > len("mailto:") && !strings.ContainsAny(s, " \t\n<>\"")
	case strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//"):
		return !strings.ContainsAny(s, " \t\n<>\"\\")
	}
	return content.HTTPSURL(s)
}

// Validate returns nil or a content.ValidationError with every problem. The
// slug must be the template: that keeps one page of each per language.
func Validate(p Page) error {
	v := content.Validate(p.Entry, false)
	if p.Kind != Kind {
		v["kind"] = "must be page"
	}
	if !slices.Contains(Templates, p.Template) {
		v["template"] = "about or now"
	} else if p.Slug != p.Template {
		v["slug"] = "must be " + p.Template
	}
	d := p.Data
	long := func(field, s string, max int) {
		if utf8.RuneCountInString(s) > max {
			v[field] = fmt.Sprintf("at most %d characters", max)
		}
	}
	tone := func(field, t string) {
		if t != "" && !slices.Contains(content.Tones, t) {
			v[field] = "unknown tone"
		}
	}
	long("name", d.Name, 40)
	long("place", d.Place, 60)
	for i, l := range d.CTAs {
		if l.Label == "" || !LinkURL(l.URL) {
			v[fmt.Sprintf("ctas.%d.url", i)] = "a label and an https, mailto: or /site URL"
		}
	}
	for i, s := range d.Skills {
		f := fmt.Sprintf("skills.%d.group", i)
		long(f, s.Group, 40)
		tone(f, s.Tone)
		if s.Group == "" || len(s.Items) == 0 {
			v[f] = "a group and at least one item"
		}
	}
	for i, j := range d.Experience {
		f := fmt.Sprintf("experience.%d.role", i)
		tone(f, j.Tone)
		if j.Role == "" || j.From == "" {
			v[f] = "a role and a start"
		}
		long(f, j.Note, 200)
	}
	for i, b := range d.Building {
		f := fmt.Sprintf("building.%d.title", i)
		tone(f, b.Tone)
		if b.Title == "" {
			v[f] = "a title"
		}
		if b.Pct < 0 || b.Pct > 100 {
			v[f] = "progress is 0 to 100"
		}
		if b.Href != "" && !LinkURL(b.Href) {
			v[f] = "the link must be https or a /site path"
		}
	}
	for i, b := range d.Reading {
		f := fmt.Sprintf("reading.%d.title", i)
		tone(f, b.Tone)
		if b.Title == "" {
			v[f] = "a title"
		}
		if b.Pct < 0 || b.Pct > 100 {
			v[f] = "progress is 0 to 100"
		}
	}
	for i, l := range d.Learning {
		f := fmt.Sprintf("learning.%d.level", i)
		tone(f, l.Tone)
		if l.Title == "" {
			v[f] = "a title"
		}
		if l.Level < 1 || l.Level > 10 {
			v[f] = "level is 1 to 10"
		}
		if l.Hours < 0 {
			v[f] = "hours cannot be negative"
		}
	}
	for i, s := range d.NotDoing {
		long(fmt.Sprintf("not_doing.%d.text", i), s, 160)
	}
	if len(v) > 0 {
		return v
	}
	return nil
}

// LevelKey is the i18n key of a learning level: beginner up to 3,
// intermediate up to 6, advanced up to 8, then expert.
func LevelKey(level int) string {
	switch {
	case level <= 3:
		return "now.level.beginner"
	case level <= 6:
		return "now.level.intermediate"
	case level <= 8:
		return "now.level.advanced"
	}
	return "now.level.expert"
}

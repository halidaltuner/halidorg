package project

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
)

var (
	currencyRE = regexp.MustCompile(`^[A-Z]{3}$`)
	repoRE     = regexp.MustCompile(`^[A-Za-z0-9-]{1,39}/[A-Za-z0-9._-]{1,100}$`)
)

// Validate returns nil or a content.ValidationError with every problem.
// Software has a stack and no parts; homelab has parts, no stack, and needs
// a homelab category (checked against the table in the repo).
func Validate(p Project) error {
	v := content.Validate(p.Entry, false)
	if p.Kind != Kind {
		v["kind"] = "must be project"
	}
	if p.Type != Software && p.Type != Homelab {
		v["project_type"] = "software or homelab"
	}
	if !slices.Contains(Statuses, p.ProjectStatus) {
		v["project_status"] = "unknown status"
	}
	if p.Type == Homelab && p.CategoryID == nil {
		v["category"] = "a homelab build needs a category"
	}
	if p.Type == Software && len(p.Parts) > 0 {
		v["parts"] = "only homelab builds have parts"
	}
	if p.Type == Homelab && len(p.Stack) > 0 {
		v["stack"] = "only software projects have a stack"
	}
	for f, s := range map[string]struct {
		val string
		max int
	}{"tagline": {p.Tagline, 200}, "kind_line": {p.KindLine, 60}, "time_spent": {p.TimeSpent, 40}} {
		if utf8.RuneCountInString(s.val) > s.max {
			v[f] = fmt.Sprintf("at most %d characters", s.max)
		}
	}
	if p.GitHubRepo != "" && !repoRE.MatchString(p.GitHubRepo) {
		v["github_repo"] = "owner/name"
	}
	if !currencyRE.MatchString(p.Currency) {
		v["currency"] = "three capital letters, e.g. TRY"
	}
	if p.CostTotal != nil && *p.CostTotal < 0 {
		v["cost_total"] = "an amount, e.g. 27000 or 149,90"
	}
	if p.StartedAt != nil && p.EndedAt != nil && *p.EndedAt < *p.StartedAt {
		v["ended_at"] = "before the start"
	}
	for i, f := range p.Facts {
		if n := utf8.RuneCountInString(f.Key); n == 0 || n > 40 {
			v[fmt.Sprintf("facts.%d.key", i)] = "1 to 40 characters"
		}
	}
	for i, s := range p.Stack {
		if n := utf8.RuneCountInString(s.Name); n == 0 || n > 60 {
			v[fmt.Sprintf("stack.%d.name", i)] = "1 to 60 characters"
		} else if utf8.RuneCountInString(s.Role) > 40 {
			v[fmt.Sprintf("stack.%d.role", i)] = "at most 40 characters"
		}
	}
	for i, x := range p.Parts {
		switch {
		case x.Name == "" || utf8.RuneCountInString(x.Name) > 120:
			v[fmt.Sprintf("parts.%d.name", i)] = "1 to 120 characters"
		case x.Qty < 1:
			v[fmt.Sprintf("parts.%d.qty", i)] = "at least 1"
		case x.Price != nil && *x.Price < 0:
			v[fmt.Sprintf("parts.%d.price", i)] = "an amount, e.g. 27000 or 149,90"
		case !currencyRE.MatchString(x.Currency):
			v[fmt.Sprintf("parts.%d.currency", i)] = "three capital letters"
		case x.URL != "" && !content.HTTPSURL(x.URL):
			v[fmt.Sprintf("parts.%d.url", i)] = "must be an absolute https URL"
		}
	}
	for i, l := range p.Links {
		switch {
		case l.Label == "" || utf8.RuneCountInString(l.Label) > 40:
			v[fmt.Sprintf("links.%d.label", i)] = "1 to 40 characters"
		case !linkURL(l.URL):
			v[fmt.Sprintf("links.%d.url", i)] = "an https URL, a path or an #anchor"
		}
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

// linkURL accepts what a hero button may point at: another site over
// https, a page here, or an anchor on the project page (#log).
func linkURL(u string) bool {
	switch {
	case content.HTTPSURL(u):
		return true
	case strings.HasPrefix(u, "/") && !strings.HasPrefix(u, "//") && !strings.ContainsAny(u, " \\"):
		return true
	case strings.HasPrefix(u, "#") && len(u) > 1 && !strings.ContainsAny(u, " \\"):
		return true
	}
	return false
}

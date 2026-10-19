package post

import (
	"net/url"
	"slices"
	"unicode/utf8"

	"halid.org/engine/internal/content"
)

// Validate returns nil or a content.ValidationError with every problem.
func Validate(p Post) error {
	v := content.Validate(p.Entry, false)
	if !slices.Contains(Kinds, p.Kind) {
		v["kind"] = "must be post, garden or runbook"
	}
	if p.CanonicalURL != "" {
		u, err := url.Parse(p.CanonicalURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			v["canonical_url"] = "must be an absolute https URL"
		}
	}
	switch p.Kind {
	case Garden:
		if !slices.Contains(Maturities, p.Maturity) {
			v["maturity"] = "seedling, budding or evergreen"
		}
	case Runbook:
		if !slices.Contains(Risks, p.Risk) {
			v["risk"] = "low, med or high"
		}
		if utf8.RuneCountInString(p.Platform) > 80 {
			v["platform"] = "at most 80 characters"
		}
		if utf8.RuneCountInString(p.Prerequisites) > 2000 {
			v["prerequisites"] = "at most 2000 characters"
		}
		if p.VerifiedOn != nil && *p.VerifiedOn <= 0 {
			v["verified_on"] = "a date"
		}
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

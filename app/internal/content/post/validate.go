package post

import (
	"net/url"

	"halid.org/engine/internal/content"
)

// Validate returns nil or a content.ValidationError with every problem.
func Validate(p Post) error {
	v := content.Validate(p.Entry, false)
	if p.Kind != Kind {
		v["kind"] = "must be post"
	}
	if p.CanonicalURL != "" {
		u, err := url.Parse(p.CanonicalURL)
		if err != nil || u.Scheme != "https" || u.Host == "" {
			v["canonical_url"] = "must be an absolute https URL"
		}
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

package fav

import (
	"slices"

	"halid.org/engine/internal/content"
)

// Validate returns nil or a content.ValidationError with every problem.
func Validate(f Fav) error {
	v := content.Validate(f.Entry, false)
	if f.Kind != Kind {
		v["kind"] = "must be fav"
	}
	if !slices.Contains(Types, f.Type) {
		v["fav_type"] = "unknown type"
	}
	if f.URL != "" && !content.HTTPSURL(f.URL) {
		v["url"] = "must be an absolute https URL"
	}
	if f.Rating != nil && (*f.Rating < 1 || *f.Rating > 5) {
		v["rating"] = "1 to 5"
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

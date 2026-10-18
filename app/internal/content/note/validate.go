package note

import (
	"strings"

	"halid.org/engine/internal/content"
)

// Validate returns nil or a content.ValidationError with every problem. A
// note may publish without a title, but not without text.
func Validate(n Note, body string) error {
	v := content.Validate(n.Entry, true)
	if n.Kind != Kind {
		v["kind"] = "must be note"
	}
	if n.SourceURL != "" && !content.HTTPSURL(n.SourceURL) {
		v["source_url"] = "must be an absolute https URL"
	}
	if (n.Status == content.Published || n.Status == content.Scheduled) && strings.TrimSpace(body) == "" {
		v["body"] = "a note needs text to publish"
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

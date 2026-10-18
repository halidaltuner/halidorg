package link

import (
	"halid.org/engine/internal/content"
)

// Validate returns nil or a content.ValidationError with every problem.
func Validate(l Link) error {
	v := content.Validate(l.Entry, false)
	if l.Kind != Kind {
		v["kind"] = "must be link"
	}
	if !content.HTTPSURL(l.URL) {
		v["url"] = "must be an absolute https URL"
	}
	for f, u := range map[string]string{"archived_url": l.ArchivedURL, "favicon_url": l.FaviconURL} {
		if u != "" && !content.HTTPSURL(u) && !(f == "favicon_url" && len(u) > 0 && u[0] == '/') {
			v[f] = "must be an absolute https URL"
		}
	}
	if l.ReadStatus != Read && l.ReadStatus != Unread {
		v["read_status"] = "read or unread"
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

package snippet

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
)

// MaxCode bounds one file; snippets are short by nature and D1 rows have a limit.
const MaxCode = 200 << 10

// Validate returns nil or a content.ValidationError with every problem.
func Validate(s Snippet) error {
	v := content.Validate(s.Entry, false)
	if s.Kind != Kind {
		v["kind"] = "must be snippet"
	}
	if s.GistURL != "" && !content.HTTPSURL(s.GistURL) {
		v["gist_url"] = "must be an absolute https URL"
	}
	seen := map[string]bool{}
	for i, f := range s.Files {
		name := strings.TrimSpace(f.Filename)
		switch {
		case name == "" || utf8.RuneCountInString(name) > 120 || strings.ContainsAny(name, `/\`):
			v[fmt.Sprintf("files.%d.filename", i)] = "1 to 120 characters, no slashes"
		case seen[name]:
			v[fmt.Sprintf("files.%d.filename", i)] = "used twice in this snippet"
		}
		seen[name] = true
		if len(f.Code) > MaxCode {
			v[fmt.Sprintf("files.%d.code", i)] = "at most 200 KB"
		}
	}
	if (s.Status == content.Published || s.Status == content.Scheduled) && len(s.Files) == 0 {
		v["files"] = "a snippet needs at least one file to publish"
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

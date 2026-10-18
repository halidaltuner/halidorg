package buildlog

import "halid.org/engine/internal/content"

// Validate returns nil or a content.ValidationError with every problem. The
// project itself is checked against the table in the repo.
func Validate(b Buildlog) error {
	v := content.Validate(b.Entry, false)
	if b.Kind != Kind {
		v["kind"] = "must be buildlog"
	}
	if b.ProjectID == "" {
		v["project"] = "a build log belongs to a project"
	}
	if b.LogDate <= 0 {
		v["log_date"] = "a date is required"
	}
	if b.Hours != nil && (*b.Hours < 0 || *b.Hours > 10000) {
		v["hours_spent"] = "0 to 10000"
	}
	if len(v) == 0 {
		return nil
	}
	return v
}

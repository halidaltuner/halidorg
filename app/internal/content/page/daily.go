package page

import (
	"context"
	"encoding/json"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
)

// TypeDaily is the job the cron Worker enqueues once a day: Now says
// "updated N days ago" and certificate statuses follow the date (About
// shows them too), so these cached pages are re-rendered daily.
const TypeDaily = "now.daily"

// RegisterDaily adds the job handler.
func RegisterDaily(j *jobs.Jobs) {
	j.Handle(TypeDaily, func(ctx context.Context, _ json.RawMessage) error {
		var paths []string
		for _, l := range i18n.Supported {
			paths = append(paths, "/"+l+"/now", "/"+l+"/certificates", "/"+l+"/about")
		}
		return jobs.EnqueuePages(ctx, j, paths...)
	})
}

package jobs

import (
	"context"
	"encoding/json"
	"fmt"
)

// TypePagesChanged re-renders a list of pages into KV. Writes that are not
// entry writes enqueue it with the pages they change: a settings save (the
// homelab stats on /{lang}/homelab, Phase 4.3).
const TypePagesChanged = "pages.changed"

// PagesChanged is its payload.
type PagesChanged struct {
	Paths []string `json:"paths"`
}

// EnqueuePages queues a re-render of paths.
func EnqueuePages(ctx context.Context, j *Jobs, paths ...string) error {
	_, err := j.Enqueue(ctx, TypePagesChanged, PagesChanged{Paths: paths})
	return err
}

func (e *EntryJobs) pages(ctx context.Context, raw json.RawMessage) error {
	var p PagesChanged
	if err := json.Unmarshal(raw, &p); err != nil || len(p.Paths) == 0 {
		return Permanent{fmt.Errorf("pages.changed: bad payload: %s", raw)}
	}
	if len(p.Paths) > 50 {
		return Permanent{fmt.Errorf("pages.changed: %d paths", len(p.Paths))}
	}
	for _, path := range p.Paths {
		if len(path) == 0 || path[0] != '/' {
			return Permanent{fmt.Errorf("pages.changed: bad path %q", path)}
		}
	}
	_, err := e.warm.Warm(ctx, p.Paths)
	return err
}

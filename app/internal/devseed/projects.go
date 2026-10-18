package devseed

import (
	"context"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Project samples (Phase 4): a software project, a homelab build with
// parts, and a build log entry, so the project pages have something to show.

func money(v int64) *int64 { return &v }

var homelabCategory = "01K0000000000000000000HM01" // seeded by migrations/0007

var sampleProjects = []project.Draft{
	{Project: project.Project{Entry: content.Entry{Slug: "sample-project-site", Lang: "en", Title: "Sample: this site"},
		Type: project.Software, ProjectStatus: "active", Tagline: "A bilingual engineering notebook with a htmx admin.", Currency: "TRY",
		Facts:    []project.Fact{{Key: "version", Value: "v0.2"}, {Key: "language", Value: "Go"}},
		KindLine: "go · htmx · cloudflare",
		Stack:    []project.Tech{{Name: "Go 1.26", Role: "server"}, {Name: "htmx 2", Role: "admin"}, {Name: "Cloudflare", Role: "edge + storage"}},
		Links:    []project.Link{{Label: "source", URL: "https://github.com/"}}},
		Markdown: "## Goal\n\nA site that is fast without a warm container.\n\n## How\n\nRender on publish into KV.", Tags: []string{"sample"}},
	{Project: project.Project{Entry: content.Entry{Slug: "sample-project-rack", Lang: "en", Title: "Sample: 3-node rack"},
		Type: project.Homelab, ProjectStatus: "running", CategoryID: &homelabCategory, Tagline: "Three mini PCs and a switch in a 10\" rack.",
		Currency: "TRY", TimeSpent: "6 weekends",
		Facts: []project.Fact{{Key: "nodes", Value: "3"}, {Key: "idle", Value: "38 W"}},
		Parts: []project.Part{{Name: "3× mini PC", Qty: 3, Price: money(2700000)}, {Name: "2.5GbE switch", Qty: 1, Price: money(480000)}},
		Links: []project.Link{{Label: "build log", URL: "#log"}}},
		Markdown: "## Goal\n\nQuiet enough for the living room.", Tags: []string{"sample"}},
}

// seedProjects creates the project samples that do not exist yet, with one
// build log entry under the rack, and queues their render jobs.
func seedProjects(ctx context.Context, db store.DB, j *jobs.Jobs) (int, error) {
	n := 0
	projects := project.NewRepo(db)
	ids := map[string]string{}
	for _, d := range sampleProjects {
		id, err := existing(ctx, db, project.Kind, d.Lang, d.Slug)
		if err != nil {
			return n, err
		}
		if id == "" {
			p, err := projects.Create(ctx, d)
			if err != nil {
				return n, err
			}
			if _, err := projects.Publish(ctx, p.ID); err != nil {
				return n, err
			}
			id = p.ID
			n++
		}
		ids[d.Slug] = id
		if j != nil {
			if err := jobs.Enqueue(ctx, j, id, nil); err != nil {
				return n, err
			}
		}
	}
	logs := buildlog.NewRepo(db)
	id, err := existing(ctx, db, buildlog.Kind, "en", "sample-log-brackets")
	if err != nil {
		return n, err
	}
	if id == "" {
		hours := 4.0
		b, err := logs.Create(ctx, buildlog.Draft{Buildlog: buildlog.Buildlog{Entry: content.Entry{Slug: "sample-log-brackets", Lang: "en", Title: "Sample: printed the brackets"},
			ProjectID: ids["sample-project-rack"], LogDate: 1760000000, Hours: &hours}, Markdown: "PLA brackets hold the three nodes."})
		if err != nil {
			return n, err
		}
		if _, err := logs.Publish(ctx, b.ID); err != nil {
			return n, err
		}
		id = b.ID
		n++
	}
	if j != nil {
		return n, jobs.Enqueue(ctx, j, id, nil)
	}
	return n, nil
}

package devseed

import (
	"context"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/page"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Page samples (Phase 5.1): an About and a Now page in English, so both
// pages have something to show. Placeholder text only.
var samplePages = []page.Draft{
	{Page: page.Page{Entry: content.Entry{Lang: "en", Title: "About", Summary: "Sample about page."}, Template: page.About,
		Data: page.Data{Name: "Sample",
			CTAs: []page.Link{{Label: "say hello", URL: "mailto:hello@example.com"}, {Label: "github", URL: "https://github.com/"}},
			Skills: []page.SkillGroup{{Group: "languages", Tone: "cyan", Items: []string{"Go", "SQL", "TypeScript"}},
				{Group: "platform", Tone: "green", Items: []string{"Cloudflare", "Docker"}},
				{Group: "data", Tone: "indigo", Items: []string{"SQLite", "Postgres"}},
				{Group: "hardware", Tone: "orange", Items: []string{"ESP32", "3D printing"}}},
			Experience: []page.Job{{From: "2023", To: "now", Role: "Sample role", Org: "Sample company", Note: "What the work was about.", Tone: "cyan"},
				{From: "2019", To: "2023", Role: "Earlier role", Org: "Another company", Note: "A line about it.", Tone: "indigo"}}}},
		Markdown: "A sample bio: one or two sentences about the work and the site."},
	{Page: page.Page{Entry: content.Entry{Lang: "en", Title: "Now", Summary: "Sample now page."}, Template: page.Now,
		Data: page.Data{Place: "istanbul",
			Building: []page.Build{{Title: "Sample: admin editor", Note: "What is being built right now.", Stage: "in progress", Pct: 65, Tone: "green", Href: "/en/projects/sample-project-site"}},
			Reading:  []page.Book{{Title: "Sample book", Author: "An author", Pct: 40, Tone: "orange"}, {Title: "Another book", Author: "Someone", Pct: 72, Tone: "indigo"}},
			Learning: []page.Learning{{Title: "Sample skill", Note: "Why and how.", Level: 3, Hours: 12, Tone: "indigo"}},
			NotDoing: []string{"New side projects until the current one ships."}}}},
}

// seedPages creates the page samples that do not exist yet and queues their
// render jobs.
func seedPages(ctx context.Context, db store.DB, j *jobs.Jobs) (int, error) {
	n := 0
	r := page.NewRepo(db)
	for _, d := range samplePages {
		id, err := existing(ctx, db, page.Kind, d.Lang, d.Template)
		if err != nil {
			return n, err
		}
		if id == "" {
			p, err := r.Create(ctx, d)
			if err != nil {
				return n, err
			}
			if _, err := r.Publish(ctx, p.ID); err != nil {
				return n, err
			}
			id = p.ID
			n++
		}
		if j != nil {
			if err := jobs.Enqueue(ctx, j, id, nil); err != nil {
				return n, err
			}
		}
	}
	return n, nil
}

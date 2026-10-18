package feeds

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/store/storetest"
)

// Projects have their own feeds, which carry their build logs; projects and
// their build logs are in the sitemap, a log under its project.
func TestProjectFeedAndSitemap(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	ctx := context.Background()
	db := storetest.NewDB(t)
	p, err := project.NewRepo(db).Create(ctx, project.Draft{Project: project.Project{Entry: content.Entry{Slug: "site", Lang: "en", Title: "halid.org"},
		Type: project.Software}, Markdown: "This site."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := project.NewRepo(db).Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	b, err := buildlog.NewRepo(db).Create(ctx, buildlog.Draft{Buildlog: buildlog.Buildlog{Entry: content.Entry{Slug: "first", Lang: "en", Title: "First"},
		ProjectID: p.ID, LogDate: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := buildlog.NewRepo(db).Publish(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(db).Register(mux)
	rec := get(t, mux, "/en/projects/feed.xml")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<link>https://halid.org/en/projects/site</link>") {
		t.Fatalf("feed %d %s", rec.Code, rec.Body.String())
	}
	if f := rec.Body.String(); !strings.Contains(f, "<title>halid.org: First</title>") || !strings.Contains(f, "<link>https://halid.org/en/projects/site/log/first</link>") {
		t.Errorf("build log not in the projects feed: %s", f)
	}
	sm := get(t, mux, "/sitemap.xml").Body.String()
	for _, want := range []string{"<loc>https://halid.org/en/projects</loc>", "<loc>https://halid.org/en/projects/site</loc>",
		"<loc>https://halid.org/en/projects/site/log/first</loc>"} {
		if !strings.Contains(sm, want) {
			t.Errorf("sitemap missing %s", want)
		}
	}
}

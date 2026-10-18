package buildlog_test

import (
	"context"
	"errors"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/store/storetest"
)

func TestBuildlog(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	pr := project.NewRepo(db)
	r := buildlog.NewRepo(db)
	p, err := pr.Create(ctx, project.Draft{Project: project.Project{Entry: content.Entry{Slug: "site", Lang: "en", Title: "halid.org"}, Type: project.Software}})
	if err != nil {
		t.Fatal(err)
	}
	var v content.ValidationError
	_, err = r.Create(ctx, buildlog.Draft{Buildlog: buildlog.Buildlog{Entry: content.Entry{Slug: "x", Lang: "en", Title: "x"}}})
	if !errors.As(err, &v) || v["project"] == "" || v["log_date"] == "" {
		t.Fatalf("no project, no date: %v", err)
	}
	_, err = r.Create(ctx, buildlog.Draft{Buildlog: buildlog.Buildlog{Entry: content.Entry{Slug: "x", Lang: "tr", Title: "x"}, ProjectID: p.ID, LogDate: 100}})
	if !errors.As(err, &v) || v["project"] == "" {
		t.Fatalf("other language accepted: %v", err)
	}
	h := 2.5
	b, err := r.Create(ctx, buildlog.Draft{Buildlog: buildlog.Buildlog{Entry: content.Entry{Slug: "webmentions", Lang: "en", Title: "Webmentions moderation queue"},
		ProjectID: p.ID, LogDate: 1760000000, Hours: &h}, Markdown: "Queue and approve."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Publish(ctx, b.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Create(ctx, buildlog.Draft{Buildlog: buildlog.Buildlog{Entry: content.Entry{Slug: "draft-log", Lang: "en", Title: "later"},
		ProjectID: p.ID, LogDate: 1760100000}}); err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, b.ID)
	if err != nil || got.ProjectSlug != "site" || got.ProjectTitle != "halid.org" || *got.Hours != 2.5 {
		t.Fatalf("get %+v %v", got, err)
	}
	all, _ := r.ForProject(ctx, p.ID, true)
	pub, _ := r.ForProject(ctx, p.ID, false)
	if len(all) != 2 || all[0].Slug != "draft-log" || len(pub) != 1 || pub[0].Slug != "webmentions" {
		t.Fatalf("timeline %d %d", len(all), len(pub))
	}
	if err := pr.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, b.ID); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("build log outlived its project: %v", err)
	}
}

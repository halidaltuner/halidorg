package jobs_test

import (
	"context"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store/storetest"
)

// RefOf reads what the purge rules need for projects and build logs.
func TestRefOfProject(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	cat := "01K0000000000000000000HM01"
	p, err := project.NewRepo(db).Create(ctx, project.Draft{Project: project.Project{Entry: content.Entry{Slug: "rack", Lang: "en", Title: "rack"},
		Type: project.Homelab, CategoryID: &cat}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := buildlog.NewRepo(db).Create(ctx, buildlog.Draft{Buildlog: buildlog.Buildlog{Entry: content.Entry{Slug: "brackets", Lang: "en", Title: "brackets"},
		ProjectID: p.ID, LogDate: 1}})
	if err != nil {
		t.Fatal(err)
	}
	pr, err := jobs.RefOf(ctx, db, p.ID)
	if err != nil || !pr.Homelab || len(pr.Logs) != 1 || pr.Logs[0] != "brackets" {
		t.Fatalf("project ref %+v %v", pr, err)
	}
	br, err := jobs.RefOf(ctx, db, b.ID)
	if err != nil || br.Parent != "rack" || jobs.ItemPath(*br) != "/en/projects/rack/log/brackets" {
		t.Fatalf("buildlog ref %+v %v", br, err)
	}
}

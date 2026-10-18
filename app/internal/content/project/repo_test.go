package project

import (
	"context"
	"errors"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

func ptr[T any](v T) *T { return &v }

const rack = "01K0000000000000000000HM01" // the seeded homelab category

func TestHomelabRowsAndRevisions(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	r := NewRepo(db)

	// The migration seeds the five homelab categories with their tones.
	cats, err := content.Categories(ctx, db, "homelab")
	if err != nil || len(cats) != 5 || cats[0].Slug != "homelab" || *cats[0].Tone != "orange" || cats[2].Slug != "3d-print" {
		t.Fatalf("homelab categories %+v %v", cats, err)
	}

	var v content.ValidationError
	// A homelab build needs a category and cannot have a stack.
	_, err = r.Create(ctx, Draft{Project: Project{Entry: content.Entry{Slug: "rack", Lang: "en", Title: "rack"}, Type: Homelab, Stack: []Tech{{Name: "go"}}}})
	if !errors.As(err, &v) || v["category"] == "" || v["stack"] == "" {
		t.Fatalf("homelab without category / with stack: %v", err)
	}
	// A link category is not a homelab category.
	link := content.Category{Kind: "link", Slug: "net", NameEN: "net"}
	if err := content.CreateCategory(ctx, db, &link); err != nil {
		t.Fatal(err)
	}
	_, err = r.Create(ctx, Draft{Project: Project{Entry: content.Entry{Slug: "rack", Lang: "en", Title: "rack"}, Type: Homelab, CategoryID: &link.ID}})
	if !errors.As(err, &v) || v["category"] == "" {
		t.Fatalf("link category accepted: %v", err)
	}

	p, err := r.Create(ctx, Draft{
		Project: Project{Entry: content.Entry{Slug: "rack", Lang: "en", Title: "3-node Proxmox cluster"}, Type: Homelab,
			CategoryID: ptr(rack), Tagline: " Three mini PCs. ", ProjectStatus: "running", TimeSpent: "6 weekends", Currency: "try",
			Facts: []Fact{{Key: "nodes", Value: "3"}, {}, {Key: "idle", Value: "38 W"}},
			Parts: []Part{{Name: "3× N100 mini PC", Price: ptr[int64](2700000)}, {Name: "UPS", Qty: 1, Price: ptr[int64](390000), Currency: "EUR"}, {}},
			Links: []Link{{Label: "build log", URL: "#log"}, {Label: "grafana", URL: "https://grafana.example/d/x"}}},
		Markdown: "## Goal\n\nQuiet enough for the living room.",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Tagline != "Three mini PCs." || got.Currency != "TRY" || len(got.Facts) != 2 || got.Facts[1].Sort != 1 ||
		len(got.Parts) != 2 || got.Parts[0].Currency != "TRY" || got.Parts[0].Qty != 1 || got.Parts[1].Currency != "EUR" ||
		len(got.Links) != 2 || got.Links[0].URL != "#log" || got.ProjectStatus != "running" {
		t.Fatalf("stored %+v", got)
	}
	// The cost sums the parts in the project currency; cost_total wins.
	if c, ok := got.Cost(); !ok || c != 2700000 {
		t.Errorf("cost %d %v", c, ok)
	}
	got.CostTotal = ptr[int64](4120000)
	if c, _ := got.Cost(); c != 4120000 {
		t.Errorf("cost_total %d", c)
	}

	// Publish, then edit the rows: the public read keeps the published ones.
	if got, err = r.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	got.Facts = []Fact{{Key: "nodes", Value: "4"}}
	got.Gallery = []string{"missing"}
	if _, err := r.Save(ctx, Draft{Project: got, Markdown: "## Goal\n\nFour now."}, content.OriginManual); !errors.As(err, &v) || v["gallery"] == "" {
		t.Fatalf("missing gallery image accepted: %v", err)
	}
	got.Gallery = nil
	if _, err := r.Save(ctx, Draft{Project: got, Markdown: "## Goal\n\nFour now."}, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	pub, err := r.GetPublished(ctx, "en", "rack")
	if err != nil || len(pub.Facts) != 2 || pub.Facts[0].Value != "3" || len(pub.Parts) != 2 {
		t.Fatalf("published rows %+v %v", pub, err)
	}
	cur, _ := r.Get(ctx, p.ID)
	if len(cur.Facts) != 1 || cur.Facts[0].Value != "4" || cur.Status != content.Published {
		t.Fatalf("current rows %+v", cur)
	}

	// Lists: admin sees drafts, the public list only published ones.
	if _, err := r.Create(ctx, Draft{Project: Project{Entry: content.Entry{Slug: "site", Lang: "en", Title: "halid.org"}, Type: Software,
		CategoryID: ptr(rack), Stack: []Tech{{Name: "go", Role: " server "}, {Name: " htmx "}, {}}}}); err != nil {
		t.Fatal(err)
	}
	all, _ := r.List(ctx, ListFilter{Lang: "en", Admin: true})
	pubs, _ := r.List(ctx, ListFilter{Lang: "en"})
	soft, _ := r.List(ctx, ListFilter{Lang: "en", Type: Software, Admin: true})
	if len(all) != 2 || len(pubs) != 1 || len(soft) != 1 || soft[0].CategoryID != nil {
		t.Fatalf("lists %d %d %d %+v", len(all), len(pubs), len(soft), soft)
	}
	sp, _ := r.Get(ctx, soft[0].ID)
	if len(sp.Stack) != 2 || sp.Stack[1].Name != "htmx" || sp.Stack[0].Role != "server" {
		t.Errorf("stack %v", sp.Stack)
	}
}

func TestValidate(t *testing.T) {
	base := Project{Entry: content.Entry{Kind: Kind, Slug: "x", Lang: "en", Title: "x", Status: content.Draft, Visibility: content.Public},
		Type: Software, ProjectStatus: "active", Currency: "TRY"}
	if err := Validate(base); err != nil {
		t.Fatal(err)
	}
	for name, mut := range map[string]func(*Project){
		"project_type":   func(p *Project) { p.Type = "hardware" },
		"project_status": func(p *Project) { p.ProjectStatus = "shipped" },
		"parts":          func(p *Project) { p.Parts = []Part{{Name: "x", Qty: 1, Currency: "TRY"}} },
		"github_repo":    func(p *Project) { p.GitHubRepo = "not a repo" },
		"currency":       func(p *Project) { p.Currency = "TL" },
		"ended_at":       func(p *Project) { p.StartedAt, p.EndedAt = ptr[int64](10), ptr[int64](5) },
		"links.0.url":    func(p *Project) { p.Links = []Link{{Label: "x", URL: "javascript:alert(1)"}} },
		"facts.0.key":    func(p *Project) { p.Facts = []Fact{{Value: "3"}} },
		"tagline":        func(p *Project) { p.Tagline = string(make([]rune, 201)) },
	} {
		p := base
		mut(&p)
		var v content.ValidationError
		if err := Validate(p); !errors.As(err, &v) || v[name] == "" {
			t.Errorf("%s: %v", name, err)
		}
	}
	for _, ok := range []string{"https://github.com/x/y", "/en/posts/x", "#log"} {
		if !linkURL(ok) {
			t.Errorf("link %q refused", ok)
		}
	}
	for _, bad := range []string{"//evil.example", "http://x.example", "#", "/a b"} {
		if linkURL(bad) {
			t.Errorf("link %q accepted", bad)
		}
	}
}

func TestDeleteTakesBuildLogs(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	r := NewRepo(db)
	p, err := r.Create(ctx, Draft{Project: Project{Entry: content.Entry{Slug: "site", Lang: "en", Title: "site"}, Type: Software}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Batch(ctx,
		store.Stmt{SQL: `INSERT INTO entries (id, kind, slug, lang, translation_group_id, title, status, visibility, created_at, updated_at)
VALUES ('L1', 'buildlog', 'first', 'en', 'L1', 'first', 'draft', 'public', 1, 1)`},
		store.Stmt{SQL: `INSERT INTO buildlogs (entry_id, project_entry_id, log_date) VALUES ('L1', ?, 1)`, Args: []any{p.ID}},
	); err != nil {
		t.Fatal(err)
	}
	if err := r.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	res, _ := db.Query(ctx, `SELECT COUNT(*) AS n FROM entries`)
	if rows, _ := store.Scan[struct {
		N int `json:"n"`
	}](res); rows[0].N != 0 {
		t.Errorf("%d entries left", rows[0].N)
	}
}

func TestTechReadsOldRevisions(t *testing.T) {
	var p Project
	RowsOf(&p, `{"stack":["go",{"name":"htmx 2","role":"admin"}]}`)
	if len(p.Stack) != 2 || p.Stack[0].Name != "go" || p.Stack[1].Role != "admin" {
		t.Fatalf("stack %+v", p.Stack)
	}
}

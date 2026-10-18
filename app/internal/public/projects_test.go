package public

import (
	"context"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/store"
)

func seedProjects(t *testing.T, db store.DB) {
	t.Helper()
	ctx := context.Background()
	pr := project.NewRepo(db)
	mk := func(d project.Draft, publish bool) project.Project {
		p, err := pr.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if publish {
			if p, err = pr.Publish(ctx, p.ID); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	cost := int64(2700000)
	homelab := "01K0000000000000000000HM01"
	mk(project.Draft{Project: project.Project{Entry: content.Entry{Slug: "site", Lang: "en", Title: "halid.org"}, Type: project.Software,
		ProjectStatus: "maintained", Tagline: "This site.", KindLine: "go · htmx",
		Facts: []project.Fact{{Key: "started", Value: "2025-11"}},
		Stack: []project.Tech{{Name: "Go 1.26", Role: "server"}, {Name: "htmx 2"}},
		Links: []project.Link{{Label: "github →", URL: "https://github.com/x/y"}, {Label: "changelog", URL: "/en/posts/changelog"}}},
		Markdown: "## Why\n\nBecause."}, true)
	rack := mk(project.Draft{Project: project.Project{Entry: content.Entry{Slug: "rack", Lang: "en", Title: "3-node rack"}, Type: project.Homelab,
		CategoryID: &homelab, ProjectStatus: "running", Tagline: "Quiet.",
		Parts: []project.Part{{Name: "mini PC", Qty: 3, Price: &cost}}}, Markdown: "## Goal\n\nQuiet."}, true)
	mk(project.Draft{Project: project.Project{Entry: content.Entry{Slug: "secret", Lang: "en", Title: "draft project"}, Type: project.Software}}, false)
	lr := buildlog.NewRepo(db)
	for i, s := range []string{"brackets", "switch"} {
		b, err := lr.Create(ctx, buildlog.Draft{Buildlog: buildlog.Buildlog{Entry: content.Entry{Slug: s, Lang: "en", Title: "log " + s},
			ProjectID: rack.ID, LogDate: int64(1760000000 + i*86400)}, Markdown: "Did " + s + "."})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lr.Publish(ctx, b.ID); err != nil {
			t.Fatal(err)
		}
	}
}

func TestProjectsPages(t *testing.T) {
	mux, db := newSite(t)
	seedProjects(t, db)

	code, body := get(t, mux, "/en/projects")
	if code != 200 {
		t.Fatalf("projects %d", code)
	}
	must(t, body,
		`<span class="pj-eyebrow mono">◆ software</span>`,
		`<a class="pj-card" href="/en/projects/site">`,
		`<span class="pj-status st-link mono">maintained</span>`,
		`<div class="pj-pills"><span class="mono">go</span><span class="mono">htmx</span></div>`,
		`<a href="/en/homelab">homelab &amp; diy</a>`,
		`href="/en/projects/feed.xml"`,
	)
	if strings.Contains(body, "/en/projects/rack") || strings.Contains(body, "draft project") {
		t.Error("homelab build or draft on the software list")
	}

	code, body = get(t, mux, "/en/projects/site")
	if code != 200 {
		t.Fatalf("site %d", code)
	}
	must(t, body,
		`<main class="pd tone-green">`, `<a class="pd-back mono" href="/en/projects">← projects</a>`,
		`<span class="pd-status mono">maintained</span><span class="pd-kind mono">go · htmx</span>`,
		`<a class="pd-btn primary" href="https://github.com/x/y">github →</a><a class="pd-btn" href="/en/posts/changelog">changelog</a>`,
		`<dt class="mono">started</dt><dd>2025-11</dd>`,
		`<h3 class="mono">stack</h3>`, `<div class="pd-row"><span>Go 1.26</span><span class="mono">server</span></div>`,
		`<div class="pd-row"><span>htmx 2</span></div>`,
		`<h2 id="why">Why<a class="h-anchor"`,
		`<link rel="canonical" href="https://halid.org/en/projects/site">`,
		`<meta property="og:image" content="https://halid.org/og/en/projects/site.png?v=`,
	)
	if strings.Contains(body, `class="pd-log"`) {
		t.Error("timeline without build logs")
	}

	_, body = get(t, mux, "/en/projects/rack")
	must(t, body,
		`<main class="pd tone-orange">`, `<a class="pd-back mono" href="/en/homelab">← homelab &amp; diy</a>`,
		`<span class="pd-kind mono">homelab</span>`,
		`<dt class="mono">cost</dt><dd>₺27.0k</dd>`,
		`<h3 class="mono">parts</h3>`, `<span>3× mini PC</span><span class="mono">₺27.0k</span>`,
		// newest first
		`<a href="/en/projects/rack/log/switch"><time class="mono" datetime="2025-10-10">2025-10-10</time><span>log switch</span></a>`,
	)
	if i, j := strings.Index(body, "log switch"), strings.Index(body, "log brackets"); i < 0 || j < 0 || i > j {
		t.Error("timeline not newest first")
	}

	// A build log under its project, with the neighbour links.
	code, body = get(t, mux, "/en/projects/rack/log/brackets")
	if code != 200 {
		t.Fatalf("log %d", code)
	}
	must(t, body, `<a class="mono back" href="/en/projects/rack#log">← 3-node rack</a>`, `<h1>log brackets</h1>`, `Did brackets.`,
		`<a class="next" href="/en/projects/rack/log/switch">log switch →`)
	for _, p := range []string{"/en/projects/site/log/brackets", "/en/projects/secret", "/en/projects/nope", "/en/projects/rack/log/nope"} {
		if code, _ := get(t, mux, p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}

	// tr: no tr projects; the en one shows with the banner, noindex.
	code, body = get(t, mux, "/tr/projects/site")
	if code != 200 {
		t.Fatalf("fallback %d", code)
	}
	must(t, body, `Bu proje henüz Türkçeye çevrilmedi.`, `<meta name="robots" content="noindex">`, `lang="en"`)
	_, body = get(t, mux, "/tr/projects")
	must(t, body, `Henüz yayınlanmış proje yok.`)
}

package public

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/github"
	"halid.org/engine/internal/settings"
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
		ProjectStatus: "maintained", Tagline: "This site.", KindLine: "go · htmx", GitHubRepo: "Halid/Site",
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
	if strings.Contains(body, `class="gh"`) || strings.Contains(body, "★") {
		t.Error("GitHub section before the first sync")
	}
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
		`<meta property="og:image" content="https://halid.org/og/en/logs/brackets.png?v=`,
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
	// The tr list shows the en project, tagged, linking to its own page.
	_, body = get(t, mux, "/tr/projects")
	must(t, body, `href="/en/projects/site"`, `<span class="lang-tag mono" title="en">en</span>`)
}

func TestHomelabPageAndTiles(t *testing.T) {
	mux, db := newSite(t)
	seedProjects(t, db)
	ctx := context.Background()
	if err := settings.Set(ctx, db, settings.HomelabKey, settings.Homelab{Nodes: "3", Idle: "38 W"}); err != nil {
		t.Fatal(err)
	}
	// A second build in another category, featured.
	net, cost := "01K0000000000000000000HM05", int64(150000)
	p, err := project.NewRepo(db).Create(ctx, project.Draft{Project: project.Project{Entry: content.Entry{Slug: "router", Lang: "en", Title: "OpenWrt router"},
		Type: project.Homelab, CategoryID: &net, Featured: true, CostTotal: &cost, TimeSpent: "2 evenings", ProjectStatus: "done"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := project.NewRepo(db).Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}

	code, body := get(t, mux, "/en/homelab")
	if code != 200 {
		t.Fatalf("homelab %d", code)
	}
	must(t, body,
		`<main class="hl tone-orange" data-filter="category">`,
		`<div><dd>2</dd><dt class="mono">builds</dt></div><div><dd>3</dd><dt class="mono">nodes</dt></div><div><dd>38 W</dd><dt class="mono">idle W</dt></div><div><dd>–</dd><dt class="mono">uptime</dt></div>`,
		// featured first
		`<a class="hl-featured" href="/en/projects/router">`, `<span class="hl-plate"><span>featured: OpenWrt router</span>`,
		`<a class="chip tone-orange" href="/en/homelab" data-filter-key="" aria-current="true">all</a>`,
		`<a class="chip tone-orange" href="/en/homelab?category=homelab" data-filter-key="homelab">homelab</a>`,
		`<a class="chip tone-green" href="/en/homelab?category=network" data-filter-key="network">network</a>`,
		`<a class="hl-card" href="/en/projects/rack" data-filter-value="homelab">`,
		`<span class="tone-green">network</span><span>done</span>`,
		`<div class="hl-card-foot mono"><span>₺1.5k</span><span>2 evenings</span></div>`,
		`<div class="hl-card-foot mono"><span>₺27.0k</span></div>`,
		`/static/js/filter.js`,
	)
	if strings.Contains(body, "/en/projects/site") {
		t.Error("software project on the homelab page")
	}
	_, body = get(t, mux, "/en/homelab?category=network")
	must(t, body, `data-filtered="network"`, `href="/en/homelab?category=network" data-filter-key="network" aria-current="true"`)
	if strings.Contains(body, `href="/en/projects/rack" data-filter-value`) {
		t.Error("other category in the network filter")
	}
	_, body = get(t, mux, "/tr/homelab")
	must(t, body, `href="/en/projects/rack"`, `<span class="lang-tag mono" title="en">en</span>`)

	// Home: the projects and homelab tiles with live counts.
	_, body = get(t, mux, "/en/")
	must(t, body,
		`<a class="home-tile tone-green" href="/en/projects"><span class="g mono" aria-hidden="true">◆</span><span class="l">projects</span><span class="n mono">1 active</span></a>`,
		`<a class="home-tile tone-orange" href="/en/homelab"><span class="g mono" aria-hidden="true">⌁</span><span class="l">homelab &amp; diy</span><span class="n mono">2 builds</span></a>`,
		`<a class="cta-outline" href="/en/projects">projects →</a>`,
	)
}

func TestProjectsGitHubSection(t *testing.T) {
	mux, db := newSite(t)
	seedProjects(t, db)
	clock := content.Clock
	content.Clock = func() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) }
	t.Cleanup(func() { content.Clock = clock })
	pushed := time.Date(2026, 10, 18, 9, 0, 0, 0, time.UTC)
	created := time.Date(2025, 11, 2, 0, 0, 0, 0, time.UTC)
	snap := github.Snapshot{Complete: true, Total: 1234, Pinned: []string{"R1"},
		Weeks: []github.Week{{Start: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC), Contributions: 30}, {Start: time.Date(2026, 10, 11, 0, 0, 0, 0, time.UTC), Contributions: 10}},
		Repos: []github.Repo{{ID: "R1", FullName: "halid/site", Name: "site", Description: "This site.", URL: "https://github.com/halid/site", Stars: 42,
			Language: "Go", PushedAt: &pushed, CreatedAt: &created, Languages: map[string]int64{"Go": 750, "HTML": 250},
			Releases: []github.Release{{ID: "V1", Tag: "v0.4.0", URL: "https://github.com/halid/site/releases/v0.4.0", Note: "Webmentions.", PublishedAt: created}}}}}
	if err := github.Write(context.Background(), db, snap, time.Date(2026, 10, 18, 11, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	_, body := get(t, mux, "/en/projects")
	must(t, body,
		`<div class="pj-card-foot mono"><span>★ 42</span><span>pushed 3h ago</span>`,
		`<section class="gh">`, `<span class="mono">40 contributions · last 52 weeks</span>`,
		`<span class="w3" style="height:100%" title="30 contributions · week of 2026-10-04"></span>`,
		`<span class="l1" style="width:75%"></span>`, `<i class="l2" aria-hidden="true"></i>HTML 25%`,
		`<a class="gh-repo" href="https://github.com/halid/site"><span class="n mono">site</span><span class="d">This site.</span><span class="m mono">Go · ★ 42</span></a>`,
		`<span class="t mono">site v0.4.0</span><span class="dt mono">2025-11-02</span><span class="nt">Webmentions.</span>`,
	)
}

func TestStreamAndGitHubPages(t *testing.T) {
	mux, db := newSite(t)
	seedProjects(t, db)
	ctx := context.Background()
	created := time.Date(2025, 11, 2, 0, 0, 0, 0, time.UTC)
	rel := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	snap := github.Snapshot{Complete: true, Weeks: []github.Week{{Start: created, Contributions: 3}},
		Repos: []github.Repo{
			{ID: "R1", FullName: "halid/site", Name: "site", URL: "https://github.com/halid/site", CreatedAt: &created,
				Releases: []github.Release{{ID: "V1", Tag: "v0.4.0", URL: "https://github.com/halid/site/releases/v0.4.0", Note: "Webmentions.", PublishedAt: rel}}},
			{ID: "R2", FullName: "halid/tool", Name: "tool", URL: "https://github.com/halid/tool", CreatedAt: &created},
		}}
	if err := github.Write(ctx, db, snap, time.Now()); err != nil {
		t.Fatal(err)
	}

	_, body := get(t, mux, "/en/")
	must(t, body,
		`<a class="chip stream-chip k-homelab" href="/en/?kind=homelab" data-filter-key="homelab">homelab</a>`,
		`<a class="stream-row" href="/en/projects/rack/log/switch" data-filter-value="homelab"><span class="glyph k-homelab mono" aria-hidden="true">⌁</span><span class="stream-title">log switch</span><span class="stream-meta mono">build log</span></a>`,
		`<a class="stream-row" href="/en/projects/rack" data-filter-value="homelab"><span class="glyph k-homelab mono" aria-hidden="true">⌁</span><span class="stream-title">3-node rack</span><span class="stream-meta mono">running</span></a>`,
		`<a class="stream-row" href="/en/projects/site" data-filter-value="project"><span class="glyph k-project mono" aria-hidden="true">◆</span><span class="stream-title">halid.org</span><span class="stream-meta mono">maintained</span></a>`,
		// GitHub rows open the linked project, else GitHub.
		`<a class="stream-row" href="/en/projects/site" data-filter-value="project"><span class="glyph k-project mono" aria-hidden="true">◆</span><span class="stream-title">site v0.4.0</span><span class="stream-meta mono">release</span></a>`,
		`<a class="stream-row" href="https://github.com/halid/tool" data-filter-value="project"><span class="glyph k-project mono" aria-hidden="true">◆</span><span class="stream-title">tool</span><span class="stream-meta mono">new repo</span></a>`,
		`<p class="stream-more"><a class="mono" href="/en/stream">whole stream →</a></p>`,
	)
	if strings.Contains(body, "draft project") {
		t.Error("draft in the stream")
	}

	code, body := get(t, mux, "/en/stream")
	if code != 200 {
		t.Fatalf("stream %d", code)
	}
	must(t, body, `<h1>Stream</h1>`, `<link rel="canonical" href="https://halid.org/en/stream">`,
		`<a class="chip stream-chip k-fav" href="/en/stream?kind=fav">fav</a>`, `log brackets`)
	if strings.Contains(body, "stream-pager") || strings.Contains(body, "noindex") {
		t.Error("pager or noindex on a short first page")
	}
	_, body = get(t, mux, "/en/stream?kind=homelab")
	must(t, body, `aria-current="true">homelab</a>`, `log switch`, `3-node rack`, `<meta name="robots" content="noindex">`)
	if strings.Contains(body, "site v0.4.0") || strings.Contains(body, ">halid.org<") {
		t.Error("project rows under homelab")
	}
	_, body = get(t, mux, "/en/stream?kind=project")
	must(t, body, `site v0.4.0`, `>halid.org<`)

	// Month archive pages: stable paths, whole months, linked in order.
	code, body = get(t, mux, "/en/stream/2025-11")
	if code != 200 {
		t.Fatalf("month %d", code)
	}
	must(t, body, `<span class="stream-month mono">2025-11</span>`, `<link rel="canonical" href="https://halid.org/en/stream/2025-11">`,
		`<span class="stream-title">tool</span>`, `← newer</a>`)
	if strings.Contains(body, "log switch") || strings.Contains(body, `class="older"`) {
		t.Error("another month's rows, or an older page before the first month")
	}
	for _, p := range []string{"/en/stream/2025-13", "/en/stream/1999-01", "/en/stream/x"} {
		if code, _ := get(t, mux, p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}
	months, err := streamMonths(ctx, db, "en")
	if err != nil || len(months) < 2 || months[len(months)-1] != "2025-11" {
		t.Fatalf("months %v %v", months, err)
	}

	code, body = get(t, mux, "/en/github")
	if code != 200 {
		t.Fatalf("github %d", code)
	}
	must(t, body, `<h1>github</h1>`, `<section class="gh">`, `<h2>activity</h2>`,
		`<div class="stream-date mono">2026-10</div>`,
		`<a class="stream-row" href="https://github.com/halid/site/releases/v0.4.0"><span class="glyph k-project mono" aria-hidden="true">◆</span><span class="stream-title">site v0.4.0 <span class="gh-note">Webmentions.</span></span><span class="stream-meta mono">release · 2026-10-09</span></a>`,
		`<span class="stream-meta mono">new repo · 2025-11-02</span>`)
	if strings.Contains(body, `<h2>github</h2>`) || strings.Contains(body, `href="/en/github">github</a>`) {
		t.Error("section heading repeats the page title")
	}
	_, body = get(t, mux, "/en/projects")
	must(t, body, `<h2><a href="/en/github">github</a></h2>`)
}

func TestGitHubPageBeforeSync(t *testing.T) {
	mux, _ := newSite(t)
	code, body := get(t, mux, "/tr/github")
	if code != 200 {
		t.Fatalf("github %d", code)
	}
	must(t, body, `GitHub verisi henüz senkronlanmadı.`)
	_, body = get(t, mux, "/tr/stream")
	must(t, body, `Henüz yayınlanmış bir şey yok.`)
}

func TestStreamFirstPageLinksOlderMonth(t *testing.T) {
	mux, db := newSite(t)
	created := time.Date(2025, 1, 2, 0, 0, 0, 0, time.UTC)
	repo := github.Repo{ID: "R1", FullName: "halid/site", Name: "site", URL: "https://github.com/halid/site", CreatedAt: &created}
	// 35 releases, one a day from 2026-03-01: the 31st newest falls in March.
	for i := 0; i < 35; i++ {
		at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC).AddDate(0, 0, i)
		repo.Releases = append(repo.Releases, github.Release{ID: fmt.Sprintf("V%d", i), Tag: fmt.Sprintf("v%d", i),
			URL: "https://github.com/halid/site/releases/x", PublishedAt: at})
	}
	if err := github.Write(context.Background(), db, github.Snapshot{Complete: true, Repos: []github.Repo{repo}}, time.Now()); err != nil {
		t.Fatal(err)
	}
	_, body := get(t, mux, "/en/stream")
	if n := strings.Count(body, `class="stream-row"`); n != 30 {
		t.Errorf("first page %d rows", n)
	}
	must(t, body, `<a class="older" href="/en/stream/2026-03">older →</a>`)
	_, body = get(t, mux, "/en/stream/2026-03?kind=project")
	must(t, body, `<meta name="robots" content="noindex">`, `href="/en/stream/2025-01?kind=project">older →</a>`, `href="/en/stream/2026-04?kind=project">← newer</a>`)
}

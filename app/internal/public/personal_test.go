package public

import (
	"context"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	pages "halid.org/engine/internal/content/page"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/store/storetest"
)

func TestAboutAndNow(t *testing.T) {
	mux, db := newSite(t)
	ctx := context.Background()
	for _, p := range []string{"/en/about", "/en/now", "/en/now/archive"} {
		if code, _ := get(t, mux, p); code != 404 {
			t.Errorf("%s before publishing: %d", p, code)
		}
	}
	seedProjects(t, db)
	m := media.NewRepo(db, storetest.NewBlob())
	cv, err := m.Upload(ctx, "cv.pdf", []byte("%PDF-1.7\n"))
	if err != nil {
		t.Fatal(err)
	}
	r := pages.NewRepo(db)
	about, err := r.Create(ctx, pages.Draft{Page: pages.Page{Entry: content.Entry{Lang: "en", Title: "About"}, Template: pages.About,
		Data: pages.Data{Name: "Halid", CV: cv.ID, CTAs: []pages.Link{{Label: "say hello", URL: "mailto:me@halid.org"}, {Label: "github", URL: "https://github.com/x"}},
			Skills:     []pages.SkillGroup{{Group: "languages", Tone: "cyan", Items: []string{"Go", "SQL"}}},
			Experience: []pages.Job{{From: "2023", To: "now", Role: "Platform Engineer", Org: "Acme", Note: "Kubernetes.", Tone: "indigo"}}}},
		Markdown: "Backend engineer in **Istanbul**."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Publish(ctx, about.ID); err != nil {
		t.Fatal(err)
	}
	code, body := get(t, mux, "/en/about")
	if code != 200 {
		t.Fatalf("about %d", code)
	}
	must(t, body,
		`<h1>Hi, I’m <span class="ab-name mono">Halid</span>.</h1>`, `<strong>Istanbul</strong>`,
		`<a class="cta-primary" href="mailto:me@halid.org">say hello</a><a class="cta-outline" href="/`+cv.R2Key+`" download>cv.pdf</a><a class="cta-outline" href="https://github.com/x">github</a>`,
		`<div class="ab-skill tone-cyan"><span class="h mono">languages</span><div class="ab-pills"><span>Go</span><span>SQL</span></div></div>`,
		`<span class="when mono">2023 – now</span><span class="role">Platform Engineer <span class="org">· Acme</span></span>`,
		`<a class="ab-card tone-green" href="/en/projects">`, `<a class="ab-card tone-orange" href="/en/homelab">`,
		`<link rel="canonical" href="https://halid.org/en/about">`,
	)
	if strings.Contains(body, `href="/en/now"><span>`) || strings.Contains(body, "what I’m doing") {
		t.Error("now card before Now is published")
	}

	clock := content.Clock
	t.Cleanup(func() { content.Clock = clock })
	at := func(d string) {
		day, _ := time.Parse("2006-01-02", d)
		content.Clock = func() time.Time { return day.Add(9 * time.Hour) }
	}
	at("2026-10-01")
	now, err := r.Create(ctx, pages.Draft{Page: pages.Page{Entry: content.Entry{Lang: "en", Title: "Now"}, Template: pages.Now,
		Data: pages.Data{Place: "istanbul", Building: []pages.Build{{Title: "admin editor", Stage: "in progress", Pct: 65, Tone: "green", Href: "/en/projects/site"}},
			Reading:  []pages.Book{{Title: "DDIA", Author: "Kleppmann", Pct: 72, Tone: "orange"}},
			Learning: []pages.Learning{{Title: "Zig", Note: "comptime", Level: 3, Hours: 26, Tone: "indigo"}},
			NotDoing: []string{"new side projects"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if now, err = r.Publish(ctx, now.ID); err != nil {
		t.Fatal(err)
	}
	at("2026-10-13")
	d := pages.Draft{Page: now}
	d.Data.Place = "ankara"
	if _, err := r.Save(ctx, d, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Publish(ctx, now.ID); err != nil {
		t.Fatal(err)
	}
	at("2026-10-17")
	_, body = get(t, mux, "/en/now")
	must(t, body,
		`<p class="nw-updated mono">updated 4 days ago</p>`, `<p class="nw-meta mono">2026-10-13 · ankara</p>`,
		`<a class="mono" href="/en/now/archive">1 earlier version →</a>`,
		`<a class="nw-build" href="/en/projects/site">`, `<span>in progress</span><span>65%</span></div><div class="bar"><span style="width:65%"></span>`,
		`<div class="nw-book tone-orange">`, `<span class="by">Kleppmann</span>`,
		`<span class="lv mono">beginner</span>`, `<span class="on"></span><span class="on"></span><span class="on"></span><span></span>`, `26h logged`,
		`<span class="x mono">✕ not doing</span><span class="i">new side projects</span>`,
	)
	if n := strings.Count(body, `<span class="on"></span>`); n != 3 {
		t.Errorf("%d level cells on", n)
	}
	_, body = get(t, mux, "/en/about")
	must(t, body, `<a class="ab-card tone-indigo" href="/en/now">`)

	code, body = get(t, mux, "/en/now/archive")
	if code != 200 {
		t.Fatalf("archive %d", code)
	}
	must(t, body, `<a href="/en/now/archive/2026-10-01"><time class="mono" datetime="2026-10-01">2026-10-01</time><span>istanbul</span></a>`)
	if strings.Contains(body, "2026-10-13") {
		t.Error("the live version in the archive")
	}
	code, body = get(t, mux, "/en/now/archive/2026-10-01")
	if code != 200 {
		t.Fatalf("version %d", code)
	}
	must(t, body, `<p class="nw-updated mono">version of 2026-10-01</p>`, `2026-10-01 · istanbul`, `<meta name="robots" content="noindex">`,
		`<a class="mono" href="/en/now">current page →</a>`)
	for _, p := range []string{"/en/now/archive/2026-10-13", "/en/now/archive/2026-10-02", "/en/now/archive/x", "/tr/now"} {
		if code, _ := get(t, mux, p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}
	if got := daysAgo("tr", time.Date(2026, 10, 16, 23, 0, 0, 0, time.UTC), time.Date(2026, 10, 17, 1, 0, 0, 0, time.UTC)); got != "dün güncellendi" {
		t.Errorf("tr yesterday %q", got)
	}
}

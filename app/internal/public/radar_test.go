package public

import (
	"context"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/radar"
	"halid.org/engine/internal/store"
)

// seedRadar freezes two editions: 2026-04 with Go (adopt), Zig (hold) and
// TypeScript (adopt); 2026-10 where Zig moves to assess, TypeScript to
// trial and leaves nothing, Gleam joins and Webpack is left out.
func seedRadar(t *testing.T, db store.DB) {
	t.Helper()
	ctx := context.Background()
	now := time.Unix(1_790_000_000, 0)
	old := content.Clock
	content.Clock = func() time.Time { now = now.Add(time.Hour); return now }
	t.Cleanup(func() { content.Clock = old })
	r := radar.NewRepo(db)
	add := func(name, q, ring, en string) radar.Item {
		i, err := r.Create(ctx, radar.Item{Name: name, Quadrant: q, Ring: ring, NoteEN: en, NoteTR: "tr: " + en})
		if err != nil {
			t.Fatal(err)
		}
		return i
	}
	add("Go", "languages", "adopt", "Default for anything server-side.")
	zig := add("Zig", "languages", "hold", "Not yet.")
	ts := add("TypeScript", "languages", "adopt", "Less of it with htmx.")
	wp := add("Webpack", "tools", "hold", "Avoid.")
	if _, err := r.Freeze(ctx, "2026-04"); err != nil {
		t.Fatal(err)
	}
	zig.Ring, zig.NoteEN = "assess", "Build system finally clicked."
	ts.Ring = "trial"
	wp.Off = true
	for _, i := range []radar.Item{zig, ts, wp} {
		if _, err := r.Save(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	add("Gleam", "languages", "assess", "Watching the BEAM story.")
	if _, err := r.Freeze(ctx, "2026-10"); err != nil {
		t.Fatal(err)
	}
}

func TestRadarPages(t *testing.T) {
	mux, db := newSite(t)
	code, body := get(t, mux, "/en/radar")
	if code != 200 {
		t.Fatalf("empty radar %d", code)
	}
	must(t, body, "No radar edition published yet.", `<title>tech radar · halid.org</title>`)
	if code, _ := get(t, mux, "/en/radar/2026-10"); code != 404 {
		t.Errorf("edition before any freeze: %d", code)
	}

	seedRadar(t, db)
	code, body = get(t, mux, "/en/radar")
	if code != 200 {
		t.Fatalf("radar %d", code)
	}
	must(t, body,
		`<a href="/en/radar" aria-current="page">2026-10</a><a href="/en/radar/2026-04">2026-04</a>`,
		`<title id="rdr-svg-t">Tech radar, edition 2026-10: 4 items, all in the list beside it.</title>`,
		`<circle cx="250" cy="250" r="60" class="rdr-c r-adopt"/>`,
		`<text x="254.0" y="246" class="rdr-rl r-adopt">adopt</text>`, `<text x="434.0" y="246" class="rdr-rl r-hold">hold</text>`,
		`<text x="490" y="18" text-anchor="end" class="rdr-ql">languages</text>`,
		// Listed by ring: Go (adopt), TypeScript (trial), Gleam and Zig (assess).
		`<a class="rdr-row" href="/en/radar/go" data-n="1"><span class="rdr-n mono">1</span><span class="rdr-text"><span class="rdr-name">Go</span><span class="rdr-note">Default for anything server-side.</span></span><span class="rdr-tag mono"></span></a>`,
		`data-n="2"><span class="rdr-n mono">2</span><span class="rdr-text"><span class="rdr-name">TypeScript</span><span class="rdr-note">Less of it with htmx.</span></span><span class="rdr-tag mono">↓ from adopt</span>`,
		`<span class="rdr-name">Gleam</span><span class="rdr-note">Watching the BEAM story.</span></span><span class="rdr-tag mono">new</span>`,
		`<span class="rdr-name">Zig</span><span class="rdr-note">Build system finally clicked.</span></span><span class="rdr-tag mono">↑ from hold</span>`,
		`<h2 class="mono" id="ring-gone">left in this edition</h2>`, `<p>Webpack</p>`,
		`/static/js/radar.js`, `href="/en/radar/feed.xml"`, `<link rel="canonical" href="https://halid.org/en/radar">`,
	)
	// New blips are triangles, moved ones have a trail, the rest circles.
	if strings.Count(body, `<polygon points="0,-11 10,7 -10,7" class="rdr-mark"/>`) != 1 || strings.Count(body, `class="rdr-trail"`) != 2 ||
		strings.Count(body, `<circle r="8" class="rdr-mark"/>`) != 3 {
		t.Error("blip shapes")
	}
	if strings.Contains(body, `<h2 class="mono" id="ring-hold">`) {
		t.Error("an empty ring is listed")
	}
	// The SVG is the same on every render.
	_, again := get(t, mux, "/en/radar")
	if again != body {
		t.Error("blips moved between renders")
	}

	// The older edition: no new or moved marks (it is the first), its own canonical.
	code, body = get(t, mux, "/en/radar/2026-04")
	if code != 200 {
		t.Fatalf("2026-04 %d", code)
	}
	must(t, body, `<a href="/en/radar">2026-10</a><a href="/en/radar/2026-04" aria-current="page">2026-04</a>`,
		`<link rel="canonical" href="https://halid.org/en/radar/2026-04">`, `<span class="rdr-name">Webpack</span>`, `<title>tech radar · 2026-04 · halid.org</title>`)
	if strings.Contains(body, "rdr-trail") || strings.Contains(body, "<polygon") {
		t.Error("the first edition marks blips")
	}
	_, body = get(t, mux, "/en/radar/2026-10")
	must(t, body, `<link rel="canonical" href="https://halid.org/en/radar">`)
	if code, _ := get(t, mux, "/en/radar/2025-01"); code != 404 {
		t.Errorf("unknown edition %d", code)
	}

	// An item: its ring now and across editions; notes where they changed.
	code, body = get(t, mux, "/en/radar/zig")
	if code != 200 {
		t.Fatalf("item %d", code)
	}
	must(t, body, `<span class="eyebrow mono">◎ // languages</span>`, `<h1>Zig</h1>`,
		`<span class="rdr-chip r-assess">assess</span> in the 2026-10 edition`, `<p class="rdr-lead">Build system finally clicked.</p>`,
		`<li><a class="mono" href="/en/radar">2026-10</a><span class="rdr-chip r-assess">assess</span><span class="rdr-tag mono">↑ from hold</span><p>Build system finally clicked.</p></li>`,
		`<li><a class="mono" href="/en/radar/2026-04">2026-04</a><span class="rdr-chip r-hold">hold</span><p>Not yet.</p></li>`,
		`hreflang="tr" href="https://halid.org/tr/radar/zig"`, `og:image`)
	_, body = get(t, mux, "/tr/radar/zig")
	must(t, body, `<p class="rdr-lead">tr: Not yet.</p>`, `<span class="rdr-chip r-assess">incele</span>`)
	_, body = get(t, mux, "/en/radar/webpack")
	must(t, body, "Not on the current radar.", `<li class="is-off"><a class="mono" href="/en/radar">2026-10</a><span class="rdr-tag mono">left the radar</span></li>`)
	if code, _ := get(t, mux, "/en/radar/nope"); code != 404 {
		t.Errorf("unknown item %d", code)
	}

	// The stream has a row per edition.
	_, body = get(t, mux, "/en/stream")
	must(t, body, `href="/en/radar"`, `tech radar 2026-10`, `1 new · 2 moved`, `tech radar 2026-04`, `href="/en/radar/2026-04"`)
	_, body = get(t, mux, "/en/stream?kind=radar")
	if strings.Contains(body, "glyph k-post") || !strings.Contains(body, "glyph k-radar") {
		t.Error("radar filter kept other rows")
	}
	// The header links the radar.
	must(t, body, `href="/en/radar"`)
}

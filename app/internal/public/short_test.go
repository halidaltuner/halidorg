package public

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/store"
)

func seedShort(t *testing.T, db store.DB) {
	t.Helper()
	seed(t, db) // advances content.Clock, so the short entries sort after the posts
	ctx := context.Background()
	sr := snippet.NewRepo(db)
	mkSnippet := func(slug, lang, title, md string, files ...snippet.File) {
		s, err := sr.Create(ctx, snippet.Draft{Snippet: snippet.Snippet{Entry: content.Entry{Slug: slug, Lang: lang, Title: title}, Files: files},
			Markdown: md, Tags: []string{"go"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sr.Publish(ctx, s.ID); err != nil {
			t.Fatal(err)
		}
	}
	mkSnippet("graceful", "en", "go: graceful shutdown", "Stop on **SIGTERM**.",
		snippet.File{Filename: "main.go", Code: "package main\n\n// wait\nfunc main() {}\n"},
		snippet.File{Filename: "run it.sh", Code: "go run .\n"})
	mkSnippet("prune", "en", "sh: prune images", "", snippet.File{Filename: "prune.sh", Code: "docker image prune -af\n"})

	nr := note.NewRepo(db)
	mkNote := func(slug, lang, title, md string, til bool) {
		n, err := nr.Create(ctx, note.Draft{Note: note.Note{Entry: content.Entry{Slug: slug, Lang: lang, Title: title},
			SourceURL: "https://go.dev/blog/", TIL: til}, Markdown: md, Tags: []string{"go"}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := nr.Publish(ctx, n.ID); err != nil {
			t.Fatal(err)
		}
	}
	mkNote("loopvar", "en", "", "Go 1.22 gives each loop iteration its own variable.", true)
	mkNote("htmx-swap", "en", "htmx swaps", "`hx-swap=outerHTML` keeps the target.", false)
}

func TestSnippetsPage(t *testing.T) {
	mux, db := newSite(t)
	seedShort(t, db)

	// The list page shows the newest snippet.
	code, body := get(t, mux, "/en/snippets")
	if code != 200 {
		t.Fatalf("snippets %d", code)
	}
	must(t, body,
		`<h1>snippets</h1>`,
		`<a href="/en/snippets/prune" aria-current="page">`,
		`<a href="/en/snippets/graceful"><span class="snip-list-title">go: graceful shutdown</span><span class="snip-list-meta mono">`,
		` · go</span>`,
		`<h2>sh: prune images</h2>`,
		`<link rel="canonical" href="https://halid.org/en/snippets">`,
		`href="/en/snippets/feed.xml"`,
		`/static/js/snippet.js`,
		`<a href="/en/snippets" data-s="snippets" aria-current="page">`,
	)

	code, body = get(t, mux, "/en/snippets/graceful")
	if code != 200 {
		t.Fatalf("snippet %d", code)
	}
	must(t, body,
		`<a href="/en/snippets/graceful" aria-current="page">`,
		`<h2>go: graceful shutdown</h2>`,
		`<strong>SIGTERM</strong>`,
		` · #go</span>`,
		`aria-selected="true" data-lang="go" data-raw="/en/snippets/graceful/raw/main.go">main.go</a>`,
		`aria-selected="false" data-lang="bash" data-raw="/en/snippets/graceful/raw/run%20it.sh">run it.sh</a>`,
		`<span class="snip-lang">go</span>`,
		`<a class="snip-raw" href="/en/snippets/graceful/raw/main.go">raw</a>`,
		`<span class="line"><span class="ln">1</span>`,
		`<span class="ln">4</span>`,
		`<link rel="canonical" href="https://halid.org/en/snippets/graceful">`,
		`<meta property="og:image" content="https://halid.org/og/en/snippets/graceful.png?v=`,
	)
	if strings.Contains(body, `<span class="ln">5</span>`) {
		t.Error("trailing newline numbered as a line")
	}

	// Raw files: text/plain, cacheable, names with spaces.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/en/snippets/graceful/raw/run%20it.sh", nil))
	if rec.Code != 200 || rec.Body.String() != "go run .\n" || rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.Contains(rec.Header().Get("Cache-Control"), "public") {
		t.Errorf("raw: %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	for _, p := range []string{"/en/snippets/graceful/raw/missing.go", "/tr/snippets/graceful/raw/main.go", "/en/snippets/nope"} {
		if code, _ := get(t, mux, p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}

	// tr: no tr snippets, so the list is empty; the en one shows with the banner.
	code, body = get(t, mux, "/tr/snippets/graceful")
	if code != 200 {
		t.Fatalf("fallback %d", code)
	}
	must(t, body, `<section class="snip-view" lang="en">`, `Bu snippet henüz Türkçeye çevrilmedi.`,
		`<meta name="robots" content="noindex">`, `<link rel="canonical" href="https://halid.org/en/snippets/graceful">`)
	if strings.Contains(body, `class="snip-list"`) {
		t.Error("en snippets in the tr list")
	}
	_, body = get(t, mux, "/tr/snippets")
	must(t, body, `Henüz yayınlanmış snippet yok.`)
}

func TestNotesPages(t *testing.T) {
	mux, db := newSite(t)
	seedShort(t, db)

	code, body := get(t, mux, "/en/notes")
	if code != 200 {
		t.Fatalf("notes %d", code)
	}
	must(t, body,
		`<h1>Notes</h1>`,
		`<article class="note-card tone-indigo">`,
		`<a class="note-time" href="/en/notes/loopvar"><time datetime="`,
		`<span class="tag-pill">TIL</span>`,
		`Go 1.22 gives each loop iteration its own variable.`,
		`<p class="note-title">htmx swaps</p>`,
		`<code>hx-swap=outerHTML</code>`,
		`<span class="tag-pill">#go</span>`,
		`source: go.dev ↗`,
		`href="/en/notes/feed.xml"`,
	)
	if i, j := strings.Index(body, "htmx swaps"), strings.Index(body, "loop iteration"); i < 0 || j < 0 || i > j {
		t.Error("notes not newest first")
	}

	code, body = get(t, mux, "/en/notes/loopvar")
	if code != 200 {
		t.Fatalf("note %d", code)
	}
	must(t, body, `← all notes`, `<link rel="canonical" href="https://halid.org/en/notes/loopvar">`,
		`<meta property="og:image" content="https://halid.org/og/en/notes/loopvar.png?v=`, `<title>note · `)

	code, body = get(t, mux, "/tr/notes/htmx-swap")
	if code != 200 {
		t.Fatalf("fallback %d", code)
	}
	must(t, body, `Bu not henüz Türkçeye çevrilmedi.`, `<div class="note-list" lang="en">`, `<meta name="robots" content="noindex">`)
	if code, _ := get(t, mux, "/en/notes/nope"); code != 404 {
		t.Errorf("missing note: %d", code)
	}
	_, body = get(t, mux, "/tr/notes")
	must(t, body, `Henüz yayınlanmış not yok.`)
}

func TestHomeStreamKinds(t *testing.T) {
	mux, db := newSite(t)
	seedShort(t, db)
	if _, err := link.NewRepo(db).Create(context.Background(), link.Draft{}); err == nil {
		t.Fatal("empty link accepted")
	}
	code, body := get(t, mux, "/en/")
	if code != 200 {
		t.Fatalf("home %d", code)
	}
	must(t, body,
		`<section class="home-section stream" data-filter="kind">`,
		`<a class="chip stream-chip k-all" href="/en/" data-filter-key="" aria-current="true">all</a>`,
		`href="/en/?kind=note" data-filter-key="note">note</a>`,
		`href="/en/?kind=snippet" data-filter-key="snippet">snippet</a>`,
		// An untitled note shows its first line; its meta is the time.
		`<a class="stream-row" href="/en/notes/loopvar" data-filter-value="note"><span class="glyph k-note mono" aria-hidden="true">~</span><span class="stream-title">Go 1.22 gives each loop iteration its own variable.</span>`,
		`<a class="stream-row" href="/en/snippets/graceful" data-filter-value="snippet"><span class="glyph k-snippet mono" aria-hidden="true">{}</span><span class="stream-title">go: graceful shutdown</span><span class="stream-meta mono">go</span>`,
		`/static/js/filter.js`,
	)
	if strings.Contains(body, `data-filter-key="fav"`) {
		t.Error("chip for a kind without entries")
	}
	_, body = get(t, mux, "/en/?kind=note")
	must(t, body, `data-filtered="note"`, `href="/en/?kind=note" data-filter-key="note" aria-current="true"`)
	if strings.Contains(body, `data-filter-value="post"`) || strings.Contains(body, `data-filter-value="snippet"`) {
		t.Error("other kinds in the note filter")
	}
}

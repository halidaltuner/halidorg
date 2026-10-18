package public

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/store"
)

func seedShort(t *testing.T, db store.DB) (snippet.Snippet, note.Note) {
	t.Helper()
	ctx := context.Background()
	snips := snippet.NewRepo(db)
	mkS := func(slug, title string, files []snippet.File, tags ...string) snippet.Snippet {
		s, err := snips.Create(ctx, snippet.Draft{Snippet: snippet.Snippet{Entry: content.Entry{Slug: slug, Lang: "en", Title: title}, Files: files},
			Markdown: "Stop on **SIGTERM**.", Tags: tags})
		if err != nil {
			t.Fatal(err)
		}
		if s, err = snips.Publish(ctx, s.ID); err != nil {
			t.Fatal(err)
		}
		return s
	}
	mkS("prune", "sh: prune images", []snippet.File{{Filename: "prune.sh", Code: "docker image prune -af"}})
	sn := mkS("shutdown", "go: graceful shutdown", []snippet.File{
		{Filename: "main.go", Code: "// stop on signal\nctx, stop := signal.NotifyContext(ctx)"},
		{Filename: "a b.sql", Code: "SELECT 1 < 2;"},
	}, "go")

	notes := note.NewRepo(db)
	mkN := func(d note.Draft) note.Note {
		n, err := notes.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if n, err = notes.Publish(ctx, n.ID); err != nil {
			t.Fatal(err)
		}
		return n
	}
	n := mkN(note.Draft{Note: note.Note{Entry: content.Entry{Slug: "til-fold", Lang: "en", Summary: "Dotless i folds"}, TIL: true, SourceURL: "https://www.unicode.org/x"},
		Markdown: "Turkish **ı** folds to i.", Tags: []string{"go"}})
	mkN(note.Draft{Note: note.Note{Entry: content.Entry{Slug: "titled", Lang: "en", Title: "A titled note"}}, Markdown: "Body."})
	return sn, n
}

func TestSnippetPages(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db)
	seedShort(t, db)

	code, body := get(t, mux, "/en/snippets")
	if code != 200 {
		t.Fatalf("snippets %d", code)
	}
	// The newest snippet is open; the list links both.
	must(t, body, "<h1>snippets</h1>", `<a href="/en/snippets/shutdown" aria-current="page">`, `<a href="/en/snippets/prune">`,
		"<h2>go: graceful shutdown</h2>", `<strong>SIGTERM</strong>`, `aria-controls="f1"`, `data-raw="/en/snippets/shutdown/raw/a%20b.sql"`,
		`<span class="ln">1</span>`, `<span class="c1">// stop on signal</span>`, "SELECT", "&lt;", `/static/js/code.js`,
		`<link rel="canonical" href="https://halid.org/en/snippets">`, `data-s="snippets" aria-current="page"`)

	code, body = get(t, mux, "/en/snippets/prune")
	if code != 200 {
		t.Fatalf("snippet %d", code)
	}
	must(t, body, "<title>sh: prune images · halid.org</title>", `<a href="/en/snippets/prune" aria-current="page">`,
		`<meta property="og:image" content="https://halid.org/og/en/snippets/prune.png?v=`, `<link rel="canonical" href="https://halid.org/en/snippets/prune">`,
		`href="/lang/tr?next=%2Ftr%2Fsnippets"`)
	if strings.Contains(body, `rel="alternate" hreflang="tr"`) {
		t.Error("a one-language snippet claims a tr alternate")
	}

	for path, want := range map[string]int{"/en/snippets/nope": 404, "/tr/snippets/prune": 404, "/en/snippets/prune/raw/x.sh": 404, "/en/snippets/Bad_Slug": 404} {
		if code, _ := get(t, mux, path); code != want {
			t.Errorf("%s = %d, want %d", path, code, want)
		}
	}
	code, body = get(t, mux, "/tr/snippets")
	if code != 200 || !strings.Contains(body, "Henüz yayınlanmış snippet yok.") {
		t.Errorf("empty tr snippets %d", code)
	}
}

func TestSnippetRaw(t *testing.T) {
	mux, db := newSite(t)
	sn, _ := seedShort(t, db)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/en/snippets/shutdown/raw/a%20b.sql", nil))
	if rec.Code != 200 || rec.Body.String() != "SELECT 1 < 2;" || rec.Header().Get("Content-Type") != "text/plain; charset=utf-8" ||
		rec.Header().Get("X-Content-Type-Options") != "nosniff" || !strings.HasPrefix(rec.Header().Get("Cache-Control"), "public") {
		t.Fatalf("raw %d %q %v", rec.Code, rec.Body.String(), rec.Header())
	}
	// An unpublished edit does not reach the raw file.
	snips := snippet.NewRepo(db)
	cur, _ := snips.Get(context.Background(), sn.ID)
	cur.Files[1].Code = "DROP TABLE x;"
	if _, err := snips.Save(context.Background(), snippet.Draft{Snippet: cur, Markdown: "x"}, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	if _, body := get(t, mux, "/en/snippets/shutdown/raw/a%20b.sql"); body != "SELECT 1 < 2;" {
		t.Errorf("raw serves the draft: %q", body)
	}
}

func TestNotePages(t *testing.T) {
	mux, db := newSite(t)
	seedShort(t, db)
	code, body := get(t, mux, "/en/notes")
	if code != 200 {
		t.Fatalf("notes %d", code)
	}
	must(t, body, `title="halid.org · notes" href="/en/notes/feed.xml"`, "<h1>notes</h1>", `<a href="/en/notes/til-fold"><time>`, "today I learned", "<strong>ı</strong>",
		`<h2><a href="/en/notes/titled">A titled note</a></h2>`, "source: unicode.org ↗", `href="/en/tags/go"`)
	if strings.Contains(body, "<h2><a href=\"/en/notes/til-fold\">") {
		t.Error("an untitled note got a heading")
	}

	code, body = get(t, mux, "/en/notes/til-fold")
	if code != 200 {
		t.Fatalf("note %d", code)
	}
	must(t, body, "<title>Dotless i folds · halid.org</title>", `<article class="post note" lang="en">`, `<meta property="og:type" content="article">`,
		`<meta property="og:image" content="https://halid.org/og/en/notes/til-fold.png?v=`, `<meta name="description" content="Dotless i folds">`,
		`href="/lang/tr?next=%2Ftr%2Fnotes"`)
	if strings.Contains(body, "<h1>") {
		t.Error("untitled note page has an h1 title")
	}
	if code, _ := get(t, mux, "/tr/notes/til-fold"); code != 404 {
		t.Errorf("tr note %d", code)
	}
}

func TestTagPageShortKinds(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db)
	seedShort(t, db)
	code, body := get(t, mux, "/en/tags/go")
	if code != 200 {
		t.Fatalf("tag %d", code)
	}
	must(t, body, "/en/posts/htmx-admin", "more with this tag", `<a class="stream-row" href="/en/notes/til-fold"><span class="glyph tone-indigo mono"`,
		`<a class="stream-row" href="/en/snippets/shutdown"><span class="glyph tone-amber mono"`)
	// A tag with only short-form entries still has a page.
	mux2, db2 := newSite(t)
	seedShort(t, db2)
	if code, body := get(t, mux2, "/en/tags/go"); code != 200 || strings.Contains(body, "No posts here yet") {
		t.Errorf("short-only tag %d", code)
	}
}

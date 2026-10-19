package public

import (
	"context"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/content/term"
)

// TestConnections: every detail page shows its backlinks and up to three
// entries in its language sharing the most tags; posts always show the
// backlinks block, other kinds only when it has something.
func TestConnections(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db) // htmx-admin #htmx #go, ghost #meta, ghosttan-ayrilmak (tr) #meta
	ctx := context.Background()

	notes := note.NewRepo(db)
	pubNote := func(slug, lang, summary string, tags ...string) note.Note {
		n, err := notes.Create(ctx, note.Draft{Note: note.Note{Entry: content.Entry{Slug: slug, Lang: lang, Summary: summary}}, Markdown: "Body.", Tags: tags})
		if err != nil {
			t.Fatal(err)
		}
		if n, err = notes.Publish(ctx, n.ID); err != nil {
			t.Fatal(err)
		}
		return n
	}
	both := pubNote("hx-go", "en", "htmx from Go", "htmx", "go")
	pubNote("go-only", "en", "Go only", "go")
	pubNote("tr-go", "tr", "Türkçe Go", "go")
	lonely := pubNote("lonely", "en", "Nothing shared", "solo")
	pubNote("bare", "en", "", "meta") // no title, no summary

	terms := term.NewRepo(db)
	ulid, err := terms.Create(ctx, term.Draft{Term: term.Term{Entry: content.Entry{Slug: "ulid", Lang: "en", Title: "ULID"}, ShortDef: "A sortable ID."}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := terms.Publish(ctx, ulid.ID); err != nil {
		t.Fatal(err)
	}
	// The job records links from published bodies; here the note links to the term.
	if _, err := db.Batch(ctx, content.SetLinksStmts(both.ID, []content.Link{{Lang: "en", Section: "glossary", Slug: "ulid", Text: "ULIDs"}})...); err != nil {
		t.Fatal(err)
	}

	// The post: two shared tags rank first, then one; no tr entries.
	_, body := get(t, mux, "/en/posts/htmx-admin")
	must(t, body, `<section class="backlinks" id="backlinks">`, `No pages link here yet.`,
		`<section class="backlinks related" id="related">`,
		`<a class="related-row" href="/en/notes/hx-go"><span class="glyph k-note mono" aria-hidden="true">~</span><span class="related-text"><span class="related-title">htmx from Go</span></span></a>`)
	if i, j := strings.Index(body, "/en/notes/hx-go"), strings.Index(body, "/en/notes/go-only"); i < 0 || j < i {
		t.Errorf("related order: hx-go %d, go-only %d", i, j)
	}
	if strings.Contains(body, "/tr/notes/tr-go") || strings.Contains(body, "/en/notes/lonely") {
		t.Error("related lists another language or an unrelated entry")
	}

	// An entry with no title and no summary goes by its kind and day.
	_, body = get(t, mux, "/en/posts/ghost")
	must(t, body, `<a class="related-row" href="/en/notes/bare"><span class="glyph k-note mono" aria-hidden="true">~</span><span class="related-text"><span class="related-title">note · 2026-`)

	// The term: a backlink from the note, no related block (no tags).
	_, body = get(t, mux, "/en/glossary/ulid")
	must(t, body, `<a href="/en/notes/hx-go" title="ULIDs">~ htmx from Go</a>`)
	if strings.Contains(body, `id="related"`) {
		t.Error("related block without tags")
	}

	// A note with nothing in it shows no block at all; one with tag-mates does.
	_, body = get(t, mux, "/en/notes/lonely")
	if strings.Contains(body, `class="backlinks`) {
		t.Errorf("empty connections on %s", lonely.Slug)
	}
	_, body = get(t, mux, "/en/notes/go-only")
	must(t, body, `href="/en/posts/htmx-admin"`, `<span class="related-title">Notes on an htmx-first admin</span><span class="related-sum">summary of Notes on an htmx-first admin</span>`)
	if strings.Contains(body, `id="backlinks"`) {
		t.Error("empty backlinks block on a note")
	}
}

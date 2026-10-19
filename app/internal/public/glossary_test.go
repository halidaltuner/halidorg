package public

import (
	"context"
	"errors"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/book"
	pages "halid.org/engine/internal/content/page"
	"halid.org/engine/internal/content/term"
)

func TestGlossary(t *testing.T) {
	mux, db := newSite(t)
	ctx := context.Background()
	r := term.NewRepo(db)
	var v content.ValidationError
	if _, err := r.Create(ctx, term.Draft{Term: term.Term{Entry: content.Entry{Slug: "x", Lang: "en"}}}); !errors.As(err, &v) || v["title"] == "" {
		t.Fatalf("term without a title: %v", err)
	}
	for _, d := range []term.Draft{
		{Term: term.Term{Entry: content.Entry{Slug: "kv", Lang: "en", Title: "KV"}, Aliases: []string{" key-value store ", "", "kv", "Key-value store"},
			ShortDef: "Cloudflare's key-value store at the edge."}, Markdown: "Eventually **consistent**."},
		{Term: term.Term{Entry: content.Entry{Slug: "isr", Lang: "en", Title: "incremental static regeneration"}}, Markdown: "x"},
		{Term: term.Term{Entry: content.Entry{Slug: "d1", Lang: "en", Title: "D1"}, ShortDef: "SQLite at the edge."}, Markdown: "y"},
	} {
		x, err := r.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Publish(ctx, x.ID); err != nil {
			t.Fatal(err)
		}
	}
	_, body := get(t, mux, "/en/glossary")
	must(t, body, `<nav class="gl-letters mono" aria-label="letters"><a href="#l-D">D</a><a href="#l-I">I</a><a href="#l-K">K</a></nav>`,
		`<div id="kv"><dt><a href="/en/glossary/kv">KV</a> <span class="gl-aka mono">key-value store</span></dt><dd>Cloudflare&#39;s key-value store at the edge.</dd></div>`,
		`href="/en/glossary/feed.xml"`)
	code, body := get(t, mux, "/en/glossary/kv")
	if code != 200 {
		t.Fatalf("term %d", code)
	}
	must(t, body, `<h1>KV</h1>`, `<p class="gl-aka mono">also: key-value store</p>`, `<p class="gl-def">Cloudflare&#39;s key-value store at the edge.</p>`,
		`<strong>consistent</strong>`, `<meta name="description" content="Cloudflare&#39;s key-value store at the edge.">`)
	if code, _ := get(t, mux, "/en/glossary/missing"); code != 404 {
		t.Errorf("missing term %d", code)
	}
	if letterOf("tr", "iz") != "İ" || letterOf("en", "iz") != "I" || letterOf("en", "3d") != "#" {
		t.Error("letters")
	}
}

func TestReadingListAndNow(t *testing.T) {
	mux, db := newSite(t)
	ctx := context.Background()
	r := book.NewRepo(db)
	five, three := 5, 3
	start, end := int64(1788220800), int64(1789948800)
	var v content.ValidationError
	if _, err := r.Create(ctx, book.Draft{Book: book.Book{Entry: content.Entry{Slug: "x", Lang: "en", Title: "x"}, State: "skimming", Progress: 120}}); !errors.As(err, &v) ||
		v["book_status"] == "" || v["progress_pct"] == "" {
		t.Fatalf("bad book: %v", err)
	}
	for _, d := range []book.Draft{
		{Book: book.Book{Entry: content.Entry{Slug: "ddia", Lang: "en", Title: "Designing Data-Intensive Applications", Summary: "Still the best map."},
			Author: "Martin Kleppmann", State: "reading", Progress: 72, StartedAt: &start}},
		{Book: book.Book{Entry: content.Entry{Slug: "sre", Lang: "en", Title: "Site Reliability Engineering"}, Author: "Google", State: "done", Rating: &five,
			StartedAt: &start, FinishedAt: &end}},
		{Book: book.Book{Entry: content.Entry{Slug: "later", Lang: "en", Title: "A book for later"}, State: "want", Rating: &three}},
	} {
		x, err := r.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Publish(ctx, x.ID); err != nil {
			t.Fatal(err)
		}
	}
	if got, _ := r.List(ctx, "en", "done"); len(got) != 1 || got[0].Progress != 100 {
		t.Errorf("a read book is at 100%%: %+v", got)
	}
	_, body := get(t, mux, "/en/reading")
	must(t, body, `<h2 class="rd-head mono" id="rd-reading">reading · 1</h2>`, `<li class="rd-book tone-amber" id="ddia">`,
		`<span class="rd-author">Martin Kleppmann</span>`, `<p>Still the best map.</p>`, `<span>since 2026-09-01</span>`,
		`aria-valuenow="72"`, `<span class="rd-stars" aria-label="5">★★★★★</span>`, `<span>2026-09-21</span>`)
	if i, j, k := strings.Index(body, "rd-reading"), strings.Index(body, "rd-want"), strings.Index(body, "rd-done"); !(i < j && j < k) {
		t.Errorf("groups out of order: %d %d %d", i, j, k)
	}

	// Now shows the books being read from the list, not the typed ones.
	pr := pages.NewRepo(db)
	now, err := pr.Create(ctx, pages.Draft{Page: pages.Page{Entry: content.Entry{Lang: "en", Title: "Now"}, Template: pages.Now,
		Data: pages.Data{Reading: []pages.Book{{Title: "Typed in the form", Pct: 10}}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pr.Publish(ctx, now.ID); err != nil {
		t.Fatal(err)
	}
	_, body = get(t, mux, "/en/now")
	must(t, body, "Designing Data-Intensive Applications", "72%")
	if strings.Contains(body, "Typed in the form") || strings.Contains(body, "Site Reliability") {
		t.Error("Now shows typed or finished books while the list has one being read")
	}
}

package public

import (
	"context"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/fav"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/store"
)

func seedCuration(t *testing.T, db store.DB) {
	t.Helper()
	seed(t, db)
	ctx := context.Background()
	cat := func(slug, en string, sort int, parent *string) *string {
		c := content.Category{Kind: link.Kind, Slug: slug, NameEN: en, NameTR: en + " tr", Sort: sort, ParentID: parent}
		if slug == "architecture" {
			tone := "green"
			c.Tone = &tone
		}
		if err := content.CreateCategory(ctx, db, &c); err != nil {
			t.Fatal(err)
		}
		return &c.ID
	}
	arch := cat("architecture", "architecture", 1, nil)
	pg := cat("postgres", "postgres", 2, nil)
	books := cat("arch-books", "books", 3, arch)

	lr := link.NewRepo(db)
	mkLink := func(slug, title, url, comment string, c *string, read bool) {
		l, err := lr.Create(ctx, link.Draft{Link: link.Link{Entry: content.Entry{Slug: slug, Lang: "en", Title: title, Summary: comment},
			URL: url, CategoryID: c}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := lr.Publish(ctx, l.ID); err != nil {
			t.Fatal(err)
		}
		if read {
			if err := lr.SetRead(ctx, l.ID, true); err != nil {
				t.Fatal(err)
			}
		}
	}
	mkLink("grug", "The Grug Brained Developer", "https://grugbrain.dev/", "Complexity is the enemy.", arch, true)
	mkLink("boring", "Choose Boring Technology", "https://mcfunley.com/choose-boring-technology", "Innovation tokens.", arch, false)
	mkLink("hypermedia", "Hypermedia Systems", "https://hypermedia.systems/", "", books, true)
	mkLink("fts", "Postgres full-text search", "https://rachbelaid.com/fts", "Ranking and weights.", pg, false)
	mkLink("loose", "Uncategorized thing", "https://example.com/x", "", nil, false)
	if _, err := db.Query(ctx, `UPDATE links SET favicon_url = '/favicons/grugbrain.dev.ico' WHERE domain = 'grugbrain.dev'`); err != nil {
		t.Fatal(err)
	}

	fr := fav.NewRepo(db)
	mkFav := func(slug, typ, title, author, url string, rating int) {
		d := fav.Draft{Fav: fav.Fav{Entry: content.Entry{Slug: slug, Lang: "en", Title: title}, Type: typ, Author: author, URL: url}}
		if rating > 0 {
			d.Rating = &rating
		}
		f, err := fr.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fr.Publish(ctx, f.ID); err != nil {
			t.Fatal(err)
		}
	}
	mkFav("ddia", "book", "Designing Data-Intensive Applications", "Martin Kleppmann", "", 5)
	mkFav("pgcli", "tool", "pgcli", "postgres client", "https://www.pgcli.com/", 4)
}

func TestLinksPage(t *testing.T) {
	mux, db := newSite(t)
	seedCuration(t, db)
	code, body := get(t, mux, "/en/links")
	if code != 200 {
		t.Fatalf("links %d", code)
	}
	must(t, body,
		`<h1>links</h1>`,
		`<main class="cur-page lk-page" data-filter="status">`,
		`<a class="chip tone-navy" href="/en/links" data-filter-key="" aria-current="true">all</a>`,
		`href="/en/links?status=unread" data-filter-key="unread">unread</a>`,
		`<h2 class="mono"><span class="cat-dot tone-green" aria-hidden="true"></span><a href="/en/awesome/architecture">architecture</a> · <span data-filter-count>2</span>`,
		`<h2 class="mono">other · <span data-filter-count>1</span>`,
		`<img class="lk-icon" src="/favicons/grugbrain.dev.ico"`,
		`<span class="lk-icon lk-initial mono" aria-hidden="true">m</span>`,
		`<a class="lk-title" href="https://grugbrain.dev/">The Grug Brained Developer</a><span class="lk-domain mono">grugbrain.dev</span>`,
		`<p>Complexity is the enemy.</p>`,
		`<span class="lk-status mono is-read" data-link-id=`,
		`>other · <span data-filter-count>1</span>`,
		`/static/js/filter.js`, `/static/js/links.js`, `href="/en/links/feed.xml"`,
		`<a class="lk-awesome" href="/en/awesome">all awesome lists →</a>`,
		`<a class="lk-awesome mono" href="/en/awesome/architecture">awesome →</a>`,
	)
	if strings.Count(body, `class="lk-awesome mono"`) != 3 {
		t.Error("every categorized group links its awesome list, other does not")
	}
	// Category order: architecture (sort 1), postgres, books; other last.
	order := []string{">architecture</a>", ">postgres</a>", ">books</a>", ">other ·"}
	last := -1
	for _, o := range order {
		i := strings.Index(body, o)
		if i < last {
			t.Errorf("%s out of order", o)
		}
		last = i
	}

	// No-JS filter: the server filters and marks the page.
	_, body = get(t, mux, "/en/links?status=unread")
	must(t, body, `data-filtered="unread"`, `Choose Boring Technology`, `href="/en/links?status=unread" data-filter-key="unread" aria-current="true"`)
	if strings.Contains(body, "Grug") {
		t.Error("read link in the unread filter")
	}
	_, body = get(t, mux, "/tr/links")
	must(t, body, `Burada henüz bağlantı yok.`)
}

func TestFavsPage(t *testing.T) {
	mux, db := newSite(t)
	seedCuration(t, db)
	code, body := get(t, mux, "/en/favs")
	if code != 200 {
		t.Fatalf("favs %d", code)
	}
	must(t, body,
		`<h1>favs</h1>`,
		`href="/en/favs?type=tool" data-filter-key="tool">tool</a>`,
		`href="/en/favs?type=book" data-filter-key="book">book</a>`,
		`<a class="fv-card" id="pgcli" href="https://www.pgcli.com/" data-filter-value="tool">`,
		`<article class="fv-card" id="ddia" data-filter-value="book">`,
		`<div class="fv-author">Martin Kleppmann</div>`,
		`<div class="fv-rating" role="img" aria-label="4 of 5"><span class="on"></span><span class="on"></span><span class="on"></span><span class="on"></span><span></span></div>`,
	)
	if strings.Contains(body, `data-filter-key="video"`) {
		t.Error("chip for a type without favs")
	}
	_, body = get(t, mux, "/en/favs?type=book")
	must(t, body, `data-filtered="book"`, `ddia`)
	if strings.Contains(body, `id="pgcli"`) {
		t.Error("tool in the book filter")
	}
}

func TestAwesomePage(t *testing.T) {
	mux, db := newSite(t)
	seedCuration(t, db)
	code, body := get(t, mux, "/en/awesome/architecture")
	if code != 200 {
		t.Fatalf("awesome %d", code)
	}
	must(t, body,
		`<header class="list-head tone-green">`,
		`<h1>awesome architecture</h1>`,
		`<a href="#architecture">general</a><a href="#arch-books">books</a>`,
		`<li><a href="https://grugbrain.dev/">The Grug Brained Developer</a> <span class="mono">grugbrain.dev</span><span class="aw-comment">Complexity is the enemy.</span></li>`,
		`<section class="aw-section" id="arch-books">`,
		`Hypermedia Systems`,
		`<link rel="canonical" href="https://halid.org/en/awesome/architecture">`,
		`<nav class="aw-back mono"><a href="/en/links">← links</a><a href="/en/awesome">all awesome lists</a></nav>`,
	)
	if strings.Contains(body, "Postgres full-text") {
		t.Error("other category on the page")
	}
	// A child category has its own page; empty and unknown ones 404.
	if code, body := get(t, mux, "/en/awesome/arch-books"); code != 200 || !strings.Contains(body, "Hypermedia Systems") {
		t.Errorf("child page %d", code)
	}
	// No tr links: the tr page lists the en ones, tagged (Phase 7.3).
	if code, body := get(t, mux, "/tr/awesome/architecture"); code != 200 || !strings.Contains(body, `<span class="lang-tag mono" title="en">en</span>`) {
		t.Errorf("tr fallback page %d", code)
	}
	for _, p := range []string{"/en/awesome/nope", "/en/awesome/Bad_Slug"} {
		if code, _ := get(t, mux, p); code != 404 {
			t.Errorf("%s: %d", p, code)
		}
	}
}

func TestAwesomeIndex(t *testing.T) {
	mux, db := newSite(t)
	seedCuration(t, db)
	code, body := get(t, mux, "/en/awesome")
	if code != 200 {
		t.Fatalf("index %d", code)
	}
	must(t, body,
		`<h1>awesome</h1>`,
		// architecture counts its own 2 links and the 1 in books.
		`<a href="/en/awesome/architecture"><span class="cat-dot tone-green" aria-hidden="true"></span>awesome architecture</a><span class="mono">3 links</span>`,
		`<a href="/en/awesome/arch-books"><span class="cat-dot tone-green" aria-hidden="true"></span>awesome books</a><span class="mono">1 link</span>`,
		`<div class="aw-index-parent mono">part of <a href="/en/awesome/architecture">architecture</a></div>`,
		`<a href="/en/awesome/postgres"><span class="cat-dot tone-navy" aria-hidden="true"></span>awesome postgres</a><span class="mono">1 link</span>`,
		`<link rel="canonical" href="https://halid.org/en/awesome">`,
	)
	if i, j := strings.Index(body, "awesome architecture"), strings.Index(body, "awesome books"); i < 0 || j < i {
		t.Error("lists not in category order")
	}
	_, body = get(t, mux, "/tr/awesome")
	must(t, body, `awesome architecture tr</a><span class="mono">3 bağlantı</span>`)
}

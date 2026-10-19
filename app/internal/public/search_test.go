package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/search"
	"halid.org/engine/internal/store/storetest"
)

// TestSearchPage: /search is the palette without JS; results in the page
// language come first, unpublished entries never show, Turkish folds.
func TestSearchPage(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	db := storetest.NewDB(t)
	sdb := storetest.NewSearchDB(t)
	h, err := New(db)
	if err != nil {
		t.Fatal(err)
	}
	h.WithSearch(sdb)
	mux := http.NewServeMux()
	h.Register(mux)
	ctx := context.Background()
	r := post.NewRepo(db)
	add := func(slug, lang, title string, publish bool) {
		p, err := r.Create(ctx, post.Draft{Post: post.Post{Entry: content.Entry{Slug: slug, Lang: lang, Title: title}}, Markdown: "Body."})
		if err != nil {
			t.Fatal(err)
		}
		if publish {
			if p, err = r.Publish(ctx, p.ID); err != nil {
				t.Fatal(err)
			}
		}
		doc := search.Doc{EntryID: p.ID, Kind: "post", Lang: lang, Status: p.Status, Visibility: p.Visibility, Slug: slug, Title: title}
		if _, err := sdb.Batch(ctx, doc.Upsert()...); err != nil {
			t.Fatal(err)
		}
	}
	add("isik", "tr", "Işık ve gölge", true)
	add("light", "en", "Işık: light in Turkish", true)
	add("draft", "tr", "Işık taslağı", false)

	code, body := get(t, mux, "/tr/search")
	if code != 200 {
		t.Fatalf("form %d", code)
	}
	must(t, body, `<form class="search-form" method="get" action="/tr/search" role="search">`, `<meta name="robots" content="noindex">`)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/tr/search?q=ISIK", nil))
	body = rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("results %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
	must(t, body, `value="ISIK"`,
		`<a class="stream-row" href="/tr/posts/isik"><span class="glyph k-post mono" aria-hidden="true">¶</span><span class="stream-title">Işık ve gölge</span><span class="stream-meta mono">yazı</span></a>`,
		`href="/en/posts/light"`, `yazı · en`)
	if strings.Index(body, "/tr/posts/isik") > strings.Index(body, "/en/posts/light") {
		t.Error("the page language is not first")
	}
	if strings.Contains(body, "taslağı") {
		t.Error("a draft is in the results")
	}
	_, body = get(t, mux, "/en/search?q=nothing+here")
	must(t, body, `No results for “nothing here”.`)
}

package public

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/search"
	"halid.org/engine/internal/store"
)

func TestPaletteSearch(t *testing.T) {
	mux, db, sdb := newSiteSearch(t)
	seed(t, db)
	ctx := context.Background()
	// Index what seed published, plus a draft that must stay hidden.
	res, _ := db.Query(ctx, `SELECT id, kind, lang, slug, title, status, visibility FROM entries`)
	rows, _ := store.Scan[struct {
		ID, Kind, Lang, Slug, Title, Status, Visibility string
	}](res)
	for _, r := range rows {
		d := search.Doc{EntryID: r.ID, Kind: r.Kind, Lang: r.Lang, Status: r.Status, Visibility: r.Visibility, Slug: r.Slug, Title: r.Title}
		if _, err := sdb.Batch(ctx, d.Upsert()...); err != nil {
			t.Fatal(err)
		}
	}
	draft := search.Doc{EntryID: "D1", Kind: "post", Lang: "en", Status: content.Draft, Visibility: content.Public, Slug: "ghost-draft", Title: "Ghost draft"}
	_, _ = sdb.Batch(ctx, draft.Upsert()...)

	get := func(q string) string {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", q, nil))
		if rec.Header().Get("Cache-Control") != "private, max-age=30" {
			t.Errorf("%s: Cache-Control %q", q, rec.Header().Get("Cache-Control"))
		}
		return rec.Body.String()
	}
	body := get("/x/search?q=ghost&lang=tr")
	if !strings.Contains(body, `href="/tr/posts/ghosttan-ayrilmak"`) || !strings.Contains(body, `href="/en/posts/ghost" role="option" lang="en"`) {
		t.Fatalf("results:\n%s", body)
	}
	if strings.Index(body, "/tr/posts/") > strings.Index(body, "/en/posts/") {
		t.Error("page-language results should come first")
	}
	if strings.Contains(body, "Ghost draft") {
		t.Error("draft leaked into public search")
	}
	if body := get("/x/search?q=p:htmx&lang=en"); !strings.Contains(body, "Notes on an htmx-first admin") || !strings.Contains(body, ">post<") {
		t.Errorf("prefix search:\n%s", body)
	}
	if body := get("/x/search?q=&lang=en"); body != "" {
		t.Errorf("empty query: %q", body)
	}
}

package public

import (
	"context"
	"strings"
	"testing"
)

func TestSeriesPages(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db)
	ctx := context.Background()
	if _, err := db.Query(ctx, `UPDATE series SET summary_en = 'A site, start to finish.' WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	_, body := get(t, mux, "/en/series")
	must(t, body, `<a class="sr-card" href="/en/series/kromatik">`, `<span>2 parts</span><span class="sr-st st-active">ongoing</span>`,
		`<h2>building kromatik</h2>`, `A site, start to finish.`)
	code, body := get(t, mux, "/en/series/kromatik")
	if code != 200 {
		t.Fatalf("series %d", code)
	}
	must(t, body, `<h1>building kromatik</h1>`,
		`<a href="/en/posts/ghost"><span class="sr-n mono">01</span><span class="sr-body"><span class="sr-title"><span aria-hidden="true">¶</span> Leaving Ghost</span>`,
		`<a href="/en/posts/htmx-admin"><span class="sr-n mono">02</span>`)
	// No Turkish parts: no Turkish page, and the index leaves it out.
	if code, _ := get(t, mux, "/tr/series/kromatik"); code != 404 {
		t.Errorf("tr series without parts: %d", code)
	}
	if _, body := get(t, mux, "/tr/series"); strings.Contains(body, "kromatik") {
		t.Error("tr index lists a series without tr parts")
	}
	// Archived series leave the site.
	if _, err := db.Query(ctx, `UPDATE series SET status = 'archived' WHERE id = 's1'`); err != nil {
		t.Fatal(err)
	}
	if code, _ := get(t, mux, "/en/series/kromatik"); code != 404 {
		t.Errorf("archived series %d", code)
	}
}

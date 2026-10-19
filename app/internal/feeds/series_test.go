package feeds

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/store/storetest"
)

func TestSeriesInSitemap(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	ctx := context.Background()
	db := storetest.NewDB(t)
	if _, err := db.Query(ctx, `INSERT INTO series (id, slug, title_en) VALUES ('s1', 'kromatik', 'kromatik'), ('s2', 'old', 'old')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Query(ctx, `UPDATE series SET status = 'archived' WHERE id = 's2'`); err != nil {
		t.Fatal(err)
	}
	r := post.NewRepo(db)
	for _, s := range []string{"s1", "s2"} {
		id := s
		p, err := r.Create(ctx, post.Draft{Post: post.Post{Entry: content.Entry{Slug: "part-" + s, Lang: "en", Title: "Part", SeriesID: &id}}, Markdown: "x"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := r.Publish(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
	}
	mux := http.NewServeMux()
	New(db).Register(mux)
	sm := get(t, mux, "/sitemap.xml").Body.String()
	if !strings.Contains(sm, "<loc>https://halid.org/en/series/kromatik</loc>") || !strings.Contains(sm, "<loc>https://halid.org/tr/series</loc>") ||
		strings.Contains(sm, "/series/old") || strings.Contains(sm, "/tr/series/kromatik") {
		t.Errorf("sitemap series:\n%s", sm)
	}
}

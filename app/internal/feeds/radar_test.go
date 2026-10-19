package feeds

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"halid.org/engine/internal/content/radar"
	"halid.org/engine/internal/store/storetest"
)

func TestRadarFeedAndSitemap(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	ctx := context.Background()
	db := storetest.NewDB(t)
	r := radar.NewRepo(db)
	zig, err := r.Create(ctx, radar.Item{Name: "Zig", Quadrant: "languages", Ring: "hold", NoteEN: "Not yet."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Freeze(ctx, "2026-04"); err != nil {
		t.Fatal(err)
	}
	zig.Ring = "assess"
	if _, err := r.Save(ctx, zig); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Freeze(ctx, "2026-10"); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(db).Register(mux)
	sm := get(t, mux, "/sitemap.xml").Body.String()
	for _, want := range []string{"<loc>https://halid.org/en/radar</loc>", "<loc>https://halid.org/tr/radar/zig</loc>", "<loc>https://halid.org/en/radar/2026-04</loc>"} {
		if !strings.Contains(sm, want) {
			t.Errorf("sitemap: no %s", want)
		}
	}
	if strings.Contains(sm, "/en/radar/2026-10<") {
		t.Error("the latest edition is listed twice")
	}
	rec := get(t, mux, "/en/radar/feed.xml")
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<link>https://halid.org/en/radar/zig</link>") || !strings.Contains(rec.Body.String(), "halid.org · tech radar") {
		t.Errorf("radar feed %d:\n%s", rec.Code, rec.Body.String())
	}
}

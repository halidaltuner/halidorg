package radar_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content/radar"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store/storetest"
)

func TestRadarAdmin(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	ctx := context.Background()
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	h, err := radar.NewAdmin(s.DB, admin.NewUI(s.DB), a, jobs.New(s.DB, s.Queue))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	fresh, _ := a.TestSession(ctx, "me@halid.org", time.Minute)
	do := func(method, target string, form url.Values, htmx bool) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		if htmx {
			req.Header.Set("HX-Request", "true")
		}
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Accept-Language", "en")
		req.AddCookie(fresh)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	must := func(body string, wants ...string) {
		t.Helper()
		for _, w := range wants {
			if !strings.Contains(body, w) {
				t.Errorf("missing %q", w)
			}
		}
	}

	body := do("GET", "/admin/radar", nil, false).Body.String()
	must(body, "No radar items yet.", "no edition yet", `<option value="languages" selected>languages</option>`)

	if rec := do("POST", "/admin/radar", url.Values{"name": {""}, "quadrant": {"languages"}, "ring": {"adopt"}}, false); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "The name must be 1 to 60 characters.") {
		t.Errorf("empty name: %d", rec.Code)
	}
	for _, it := range [][]string{{"Go", "languages", "adopt", "Default for anything server-side."}, {"Zig", "languages", "hold", ""}} {
		if rec := do("POST", "/admin/radar", url.Values{"name": {it[0]}, "quadrant": {it[1]}, "ring": {it[2]}, "note_en": {it[3]}}, false); rec.Code != http.StatusSeeOther {
			t.Fatalf("create %s: %d %s", it[0], rec.Code, rec.Body.String())
		}
	}
	repo := radar.NewRepo(s.DB)
	items, _ := repo.Items(ctx)
	if len(items) != 2 {
		t.Fatalf("items %+v", items)
	}
	zig := items[1]
	body = do("GET", "/admin/radar", nil, false).Body.String()
	must(body, `<tr id="r-`+zig.Group+`">`, `value="Default for anything server-side."`, "next edition: 2 new, 0 moved, 0 dropped",
		`hx-post="/admin/radar/`+zig.Group+`"`)

	// Freezing shows the diff and needs a valid label.
	body = do("GET", "/admin/radar/freeze", nil, false).Body.String()
	must(body, "First edition", `<span class="rdr-name">Go</span>`, `name="label" value="20`)
	if rec := do("POST", "/admin/radar/freeze", url.Values{"label": {"2026-13"}}, false); rec.Code != http.StatusUnprocessableEntity ||
		!strings.Contains(rec.Body.String(), "The label must be YYYY-MM") {
		t.Errorf("bad label: %d", rec.Code)
	}
	sent := len(s.Queue.(*storetest.Queue).Sent)
	if rec := do("POST", "/admin/radar/freeze", url.Values{"label": {"2026-04"}}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("freeze %d %s", rec.Code, rec.Body.String())
	}
	if n := len(s.Queue.(*storetest.Queue).Sent) - sent; n != 5 {
		t.Errorf("freeze enqueued %d jobs, want 5 (two items in two languages, the pages)", n)
	}
	body = do("GET", "/admin/radar?notice=frozen", nil, false).Body.String()
	must(body, "Edition frozen and published.", "latest edition: 2026-04", "no changes since the latest edition", `<a href="/en/radar/zig">zig</a>`,
		`<a href="/en/radar/2026-04">`)

	// A row saves in place with htmx; a bad value stays in the row.
	rec := do("POST", "/admin/radar/"+zig.Group, url.Values{"name": {"Zig"}, "quadrant": {"languages"}, "ring": {"assess"}, "note_en": {"Promoted."}}, true)
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Body.String(), `<tr id="r-`+zig.Group+`">`) || !strings.Contains(rec.Body.String(), "Saved") {
		t.Errorf("htmx save: %d %s", rec.Code, rec.Body.String())
	}
	rec = do("POST", "/admin/radar/"+zig.Group, url.Values{"name": {"Zig"}, "quadrant": {"languages"}, "ring": {"never"}}, true)
	if !strings.Contains(rec.Body.String(), "Pick a ring.") {
		t.Errorf("htmx error row: %s", rec.Body.String())
	}
	body = do("GET", "/admin/radar/freeze", nil, false).Body.String()
	must(body, "Since 2026-04", `<li class="rdr-moved"><span class="rdr-mark mono">moved</span><span class="rdr-name">Zig</span><span class="rdr-move mono">languages · hold → assess</span></li>`,
		"unchanged: 1")
	// Nothing public moved yet: the entry still has the frozen note.
	ids, _ := repo.EntryIDs(ctx, zig.Group)
	res, _ := s.DB.Query(ctx, `SELECT summary FROM entries WHERE id = ?`, ids["en"])
	if string(res.Rows[0]) != `{"summary":""}` {
		t.Errorf("draft edit reached the entry: %s", res.Rows[0])
	}

	// Deleting needs a recent sign-in and re-renders the item's pages.
	sent = len(s.Queue.(*storetest.Queue).Sent)
	if rec := do("POST", "/admin/radar/"+zig.Group+"/delete", url.Values{}, false); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/radar?notice=deleted" {
		t.Fatalf("delete %d %s", rec.Code, rec.Header().Get("Location"))
	}
	if n := len(s.Queue.(*storetest.Queue).Sent) - sent; n != 2 {
		t.Errorf("delete enqueued %d jobs, want 2", n)
	}
	if rec := do("POST", "/admin/radar/"+zig.Group, url.Values{"name": {"Zig"}}, false); rec.Code != http.StatusNotFound {
		t.Errorf("save after delete: %d", rec.Code)
	}
}

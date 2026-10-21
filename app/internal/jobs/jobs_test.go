package jobs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
	"halid.org/engine/internal/ui"
)

func TestPaths(t *testing.T) {
	now := &Ref{Kind: "post", Lang: "en", Slug: "htmx", Tags: []string{"go", "htmx"}, Listed: true}
	prev := &Ref{Kind: "post", Lang: "en", Slug: "htmx-old", Tags: []string{"go", "admin", "Bad Tag"}}
	got := Paths(Change{Now: now, Prev: prev, Siblings: []Ref{{Kind: "post", Lang: "tr", Slug: "htmx-tr"}, {Kind: "uses", Lang: "en", Slug: "x"}}})
	want := []string{
		"/en/", "/en/atom.xml", "/en/feed.json", "/en/feed.xml", "/en/posts", "/en/posts/htmx", "/en/posts/htmx-old",
		"/en/stream", "/en/tags/admin", "/en/tags/admin/feed.xml", "/en/tags/go", "/en/tags/go/feed.xml",
		"/en/tags/htmx", "/en/tags/htmx/feed.xml", "/sitemap.xml", "/tr/posts", "/tr/posts/htmx", "/tr/posts/htmx-old", "/tr/posts/htmx-tr",
		"/tr/tags/admin", "/tr/tags/go", "/tr/tags/htmx", // the tr lists show it while untranslated
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Paths =\n%v\nwant\n%v", got, want)
	}
	// Deleted entry: Prev only.
	if got := Paths(Change{Prev: &Ref{Kind: "post", Lang: "tr", Slug: "x"}}); !contains(got, "/tr/posts/x") || !contains(got, "/sitemap.xml") {
		t.Errorf("delete paths = %v", got)
	}
	// A note: its page and fallback, the notes list and the notes feeds, the
	// stream, not the post feeds.
	notePaths := Paths(Change{Now: &Ref{Kind: "note", Lang: "en", Slug: "til", Listed: true}})
	if want := []string{"/en/", "/en/notes", "/en/notes/atom.xml", "/en/notes/feed.json", "/en/notes/feed.xml", "/en/notes/til", "/en/stream", "/sitemap.xml", "/tr/notes", "/tr/notes/til"}; !reflect.DeepEqual(notePaths, want) {
		t.Errorf("note paths =\n%v\nwant\n%v", notePaths, want)
	}
	// A link has no page of its own: the list and its feeds.
	linkPaths := Paths(Change{Now: &Ref{Kind: "link", Lang: "tr", Slug: "grug", Listed: true}})
	if want := []string{"/en/links", "/sitemap.xml", "/tr/", "/tr/links", "/tr/links/atom.xml", "/tr/links/feed.json", "/tr/links/feed.xml", "/tr/stream"}; !reflect.DeepEqual(linkPaths, want) {
		t.Errorf("link paths =\n%v\nwant\n%v", linkPaths, want)
	}
	if got := Paths(Change{Now: &Ref{Kind: "fav", Lang: "en", Slug: "ddia"}}); !contains(got, "/en/favs") || contains(got, "/en/favs/ddia") {
		t.Errorf("fav paths = %v", got)
	}
	// A homelab project: its page, the projects list and feeds, the homelab
	// page; a slug change moves its build logs too.
	proj := Paths(Change{Now: &Ref{Kind: "project", Lang: "en", Slug: "rack", Listed: true, Homelab: true, Logs: []string{"brackets"}},
		Prev: &Ref{Kind: "project", Lang: "en", Slug: "rack-old", Listed: true, Homelab: true, Logs: []string{"brackets"}}})
	if want := []string{"/en/", "/en/about", "/en/homelab", "/en/projects", "/en/projects/atom.xml", "/en/projects/feed.json", "/en/projects/feed.xml",
		"/en/projects/rack", "/en/projects/rack-old", "/en/projects/rack-old/log/brackets", "/en/projects/rack/log/brackets",
		"/en/stream", "/sitemap.xml", "/tr/homelab", "/tr/projects", "/tr/projects/rack", "/tr/projects/rack-old"}; !reflect.DeepEqual(proj, want) {
		t.Errorf("project paths =\n%v\nwant\n%v", proj, want)
	}
	if got := Paths(Change{Now: &Ref{Kind: "project", Lang: "en", Slug: "site", Listed: true}}); contains(got, "/en/homelab") {
		t.Errorf("software project on the homelab page: %v", got)
	}
	// A presentation: its page, the list and its feeds, About (its card).
	pres := Paths(Change{Now: &Ref{Kind: "presentation", Lang: "tr", Slug: "htmx", Listed: true}})
	for _, want := range []string{"/tr/presentations/htmx", "/tr/presentations", "/tr/presentations/feed.xml", "/tr/presentations/atom.xml",
		"/tr/presentations/feed.json", "/tr/about", "/tr/", "/sitemap.xml"} {
		if !contains(pres, want) {
			t.Errorf("presentation paths %v: no %s", pres, want)
		}
	}
	// A build log: its page under the project, the project page, the
	// projects feeds, the stream.
	logPaths := Paths(Change{Now: &Ref{Kind: "buildlog", Lang: "en", Slug: "brackets", Parent: "rack", Listed: true}})
	if want := []string{"/en/", "/en/projects/atom.xml", "/en/projects/feed.json", "/en/projects/feed.xml", "/en/projects/rack", "/en/projects/rack/log/brackets", "/en/stream"}; !reflect.DeepEqual(logPaths, want) {
		t.Errorf("buildlog paths =\n%v\nwant\n%v", logPaths, want)
	}
	// The month it was published in (now and before) has a stream archive page.
	moved := Paths(Change{Now: &Ref{Kind: "note", Lang: "en", Slug: "til", Listed: true, Month: "2026-10"}, Prev: &Ref{Kind: "note", Lang: "en", Slug: "til", Listed: true, Month: "2026-09"}})
	if !contains(moved, "/en/stream/2026-10") || !contains(moved, "/en/stream/2026-09") {
		t.Errorf("month paths %v", moved)
	}
	// Now: its page, the other language's, the archive list and the day of
	// its latest version, About (the now card), the sitemap.
	nowPaths := Paths(Change{Now: &Ref{Kind: "page", Lang: "en", Slug: "now", Listed: true, Version: "2026-10-13"},
		Prev: &Ref{Kind: "page", Lang: "en", Slug: "now", Listed: true, Version: "2026-10-01"}})
	if want := []string{"/en/about", "/en/now", "/en/now/archive", "/en/now/archive/2026-10-01", "/en/now/archive/2026-10-13", "/sitemap.xml", "/tr/now"}; !reflect.DeepEqual(nowPaths, want) {
		t.Errorf("now paths =\n%v\nwant\n%v", nowPaths, want)
	}
	// A runbook (Phase 5.5): its page and the runbooks list with its feeds.
	rb := Paths(Change{Now: &Ref{Kind: "runbook", Lang: "en", Slug: "zfs", Listed: true}})
	for _, want := range []string{"/en/runbooks/zfs", "/en/runbooks", "/en/runbooks/feed.xml", "/tr/runbooks/zfs"} {
		if !contains(rb, want) {
			t.Errorf("runbook paths %v: no %s", rb, want)
		}
	}
	// A part of a series: the series index and page, and the other parts
	// (their nav shows its title).
	sp := Paths(Change{Now: &Ref{Kind: "post", Lang: "en", Slug: "two", Listed: true, Series: "kromatik", Parts: []string{"post:one", "garden:three"}}})
	for _, want := range []string{"/en/series", "/en/series/kromatik", "/en/posts/one", "/en/garden/three"} {
		if !contains(sp, want) {
			t.Errorf("series part paths %v: no %s", sp, want)
		}
	}
	// A book: the reading list and its feeds, and Now (the books being read).
	bk := Paths(Change{Now: &Ref{Kind: "book", Lang: "tr", Slug: "ddia", Listed: true}})
	for _, want := range []string{"/tr/reading", "/tr/reading/feed.xml", "/tr/now"} {
		if !contains(bk, want) {
			t.Errorf("book paths %v: no %s", bk, want)
		}
	}
	if contains(bk, "/tr/reading/ddia") {
		t.Errorf("book with a page of its own: %v", bk)
	}
	// Kinds without pages contribute nothing.
	if got := Paths(Change{Now: &Ref{Kind: "uses", Lang: "en", Slug: "x"}}); len(got) != 0 {
		t.Errorf("unknown kind paths = %v", got)
	}
	// A radar item: its page and the radar with its feeds.
	rd := Paths(Change{Now: &Ref{Kind: "radar_item", Lang: "en", Slug: "zig", Listed: true}})
	for _, want := range []string{"/en/radar/zig", "/en/radar", "/en/radar/feed.xml", "/sitemap.xml"} {
		if !contains(rd, want) {
			t.Errorf("radar paths %v: no %s", rd, want)
		}
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

func TestCacheKeyMatchesWorker(t *testing.T) {
	// Same cases as worker/test/cache.test.ts.
	cases := map[string]string{
		"/en/": "page:en:/en/", "/tr/posts/x": "page:tr:/tr/posts/x", "/": "page:-:/", "/de/": "page:-:/de/",
		"/sitemap.xml": "page:-:/sitemap.xml",
	}
	for in, want := range cases {
		if got := CacheKey(in); got != want {
			t.Errorf("CacheKey(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCacheable(t *testing.T) {
	h := func(kv ...string) http.Header {
		out := http.Header{}
		for i := 0; i < len(kv); i += 2 {
			out.Set(kv[i], kv[i+1])
		}
		return out
	}
	if !Cacheable(200, h("Content-Type", "text/html; charset=utf-8", "Cache-Control", "public, max-age=0")) {
		t.Error("public html")
	}
	if !Cacheable(200, h("Content-Type", "application/rss+xml", "Cache-Control", "public")) {
		t.Error("rss")
	}
	for _, bad := range []http.Header{
		h("Content-Type", "text/html", "Cache-Control", "private"),
		h("Content-Type", "text/html", "Cache-Control", "public, no-store"),
		h("Content-Type", "text/html"),
		h("Content-Type", "image/png", "Cache-Control", "public"),
		h("Content-Type", "text/html", "Cache-Control", "public", "Set-Cookie", "a=b"),
	} {
		if Cacheable(200, bad) {
			t.Errorf("cacheable: %v", bad)
		}
	}
	if Cacheable(404, h("Content-Type", "text/html", "Cache-Control", "public")) {
		t.Error("404 cacheable")
	}
}

func TestWarmStoresAndDeletes(t *testing.T) {
	kv := storetest.NewKV()
	_ = kv.Put(context.Background(), "page:en:/en/gone", []byte("old"), store.KVPut{})
	mux := http.NewServeMux()
	mux.HandleFunc("GET /en/ok", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=0")
		w.Header().Set("ETag", `"abc"`)
		_, _ = w.Write([]byte("<p>ok</p>"))
	})
	// The Worker's header names the live deploy (ui.DeployHeader).
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Deploy-Sha", "113540ed11ad344faed75867d95c05eff1a8ffb2")
	ui.DeployHeader(http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), req)
	w := NewWarmer(mux, kv)
	res, err := w.Warm(context.Background(), []string{"/en/ok", "/en/gone"})
	if err != nil {
		t.Fatal(err)
	}
	if !res[0].Stored || res[1].Stored {
		t.Errorf("results = %+v", res)
	}
	if string(kv.Vals["page:en:/en/ok"]) != "<p>ok</p>" {
		t.Errorf("stored = %q", kv.Vals["page:en:/en/ok"])
	}
	if m := kv.Meta["page:en:/en/ok"].(PageMeta); m.ETag != `"abc"` || !strings.HasPrefix(m.ContentType, "text/html") || m.StoredAt == 0 || m.Deploy != "113540ed11ad344faed75867d95c05eff1a8ffb2" {
		t.Errorf("meta = %+v", m)
	}
	if _, ok := kv.Vals["page:en:/en/gone"]; ok {
		t.Error("404 path not deleted")
	}
}

func setup(t *testing.T) (store.Stores, *Jobs, *storetest.Queue) {
	t.Helper()
	s := storetest.Stores(t)
	q := s.Queue.(*storetest.Queue)
	return s, New(s.DB, q), q
}

func runStatus(t *testing.T, db store.DB, id string) (string, int, string) {
	t.Helper()
	res, err := db.Query(context.Background(), `SELECT status, attempt, error FROM job_runs WHERE id = ?`, id)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := store.Scan[struct {
		Status  string `json:"status"`
		Attempt int    `json:"attempt"`
		Error   string `json:"error"`
	}](res)
	if len(rows) != 1 {
		t.Fatalf("run %s not found", id)
	}
	return rows[0].Status, rows[0].Attempt, rows[0].Error
}

func deliver(j *Jobs, msg json.RawMessage, attempt string, header bool) int {
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs", bytes.NewReader(msg))
	if header {
		req.Header.Set(InternalHeader, "jobs")
	}
	req.Header.Set("X-Job-Attempt", attempt)
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	j.Register(mux)
	mux.ServeHTTP(rec, req)
	return rec.Code
}

func TestEnqueueDeliverAndRetry(t *testing.T) {
	s, j, q := setup(t)
	calls := 0
	j.Handle("flaky", func(context.Context, json.RawMessage) error {
		calls++
		if calls < 3 {
			return errors.New("boom")
		}
		return nil
	})
	id, err := j.Enqueue(context.Background(), "flaky", map[string]int{"n": 1})
	if err != nil {
		t.Fatal(err)
	}
	if st, _, _ := runStatus(t, s.DB, id); st != "queued" || len(q.Sent) != 1 {
		t.Fatalf("after enqueue: %s, sent %d", st, len(q.Sent))
	}
	msg := q.Sent[0]

	if code := deliver(j, msg, "1", false); code != http.StatusNotFound {
		t.Errorf("without internal header: %d", code)
	}
	if code := deliver(j, msg, "1", true); code != 500 {
		t.Errorf("attempt 1: %d", code)
	}
	if st, a, e := runStatus(t, s.DB, id); st != "failed" || a != 1 || e != "boom" {
		t.Errorf("after attempt 1: %s %d %q", st, a, e)
	}
	_ = deliver(j, msg, "2", true)
	if code := deliver(j, msg, "3", true); code != 200 {
		t.Errorf("attempt 3: %d", code)
	}
	if st, a, e := runStatus(t, s.DB, id); st != "done" || a != 3 || e != "" {
		t.Errorf("after success: %s %d %q", st, a, e)
	}
}

func TestDeadAfterMaxAndUnknownType(t *testing.T) {
	s, j, q := setup(t)
	j.Handle("always", func(context.Context, json.RawMessage) error { return errors.New("no") })
	id, _ := j.Enqueue(context.Background(), "always", nil)
	if code := deliver(j, q.Sent[0], "4", true); code != 500 {
		t.Errorf("last attempt code %d", code)
	}
	if st, _, _ := runStatus(t, s.DB, id); st != "dead" {
		t.Errorf("status = %s, want dead", st)
	}
	failed, err := j.Failed(context.Background(), 0)
	if err != nil || len(failed) != 1 || failed[0].ID != id {
		t.Fatalf("Failed = %+v %v", failed, err)
	}
	if err := j.Retry(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	if st, a, _ := runStatus(t, s.DB, id); st != "queued" || a != 0 || len(q.Sent) != 2 {
		t.Errorf("after retry: %s %d sent %d", st, a, len(q.Sent))
	}

	// Unknown type: dead at once, acked so the Worker does not retry.
	id2, _ := j.Enqueue(context.Background(), "nope", nil)
	if code := deliver(j, q.Sent[2], "1", true); code != 200 {
		t.Errorf("unknown type code %d", code)
	}
	if st, _, e := runStatus(t, s.DB, id2); st != "dead" || !strings.Contains(e, "unknown job type") {
		t.Errorf("unknown type: %s %q", st, e)
	}
}

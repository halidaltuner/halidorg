package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/search"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

// signedIn returns the admin mux, its stores and a session cookie.
func signedIn(t *testing.T) (*http.ServeMux, store.Stores, *http.Cookie) {
	t.Helper()
	mux, s, a := newSite(t, auth.Config{SessionSecret: strings.Repeat("k", 32)})
	c, err := a.TestSession(context.Background(), "me@halid.org", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return mux, s, c
}

// do sends a same-origin request, as a browser would from an admin page.
func do(mux *http.ServeMux, c *http.Cookie, method, target string, form url.Values, htmx bool) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Accept-Language", "en")
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec
}

func entry(t *testing.T, db store.DB, slug, title, status string, at int64) string {
	t.Helper()
	e := content.Entry{Kind: "post", Slug: slug, Lang: "en", Title: title, Status: status, Visibility: content.Public}
	if status == content.Published {
		e.Status = content.Draft // the CHECK wants the published pointer first
	}
	if status == content.Published || status == content.Scheduled {
		e.PublishedAt = &at
	}
	stmts := content.InsertEntry(&e, content.Body{MD: "x"}, content.OriginManual)
	stmts = append(stmts, store.Stmt{SQL: `INSERT INTO posts (entry_id) VALUES (?)`, Args: []any{e.ID}})
	if status == content.Published {
		stmts = append(stmts, store.Stmt{SQL: `UPDATE entries SET status = 'published', published_revision_id = current_revision_id WHERE id = ?`, Args: []any{e.ID}})
	}
	if _, err := db.Batch(context.Background(), stmts...); err != nil {
		t.Fatal(err)
	}
	return e.ID
}

func exec(t *testing.T, db store.DB, sql string, args ...any) {
	t.Helper()
	if _, err := db.Query(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func TestDashboard(t *testing.T) {
	mux, s, c := signedIn(t)
	now := time.Now().Unix()
	draft := entry(t, s.DB, "bilingual-fts-3", "Bilingual FTS, part 3", content.Draft, 0)
	entry(t, s.DB, "radar-notes", "Radar notes", content.Scheduled, now+86400)
	entry(t, s.DB, "htmx-admin", "Notes on an htmx-first admin", content.Published, now-3600)
	exec(t, s.DB, `INSERT INTO job_runs (id, type, status, attempt, error, created_at) VALUES ('01JOBFAILED000000000000001', 'entry.changed', 'dead', 4, 'timeout', ?)`, now)
	exec(t, s.DB, `INSERT INTO not_found_log (path, hits, last_seen_at) VALUES ('/2019/05/docker-notes/', 38, ?), ('/old/', 3, ?)`, now, now-30*86400)

	rec := do(mux, c, "GET", "/admin", nil, false)
	if rec.Code != 200 {
		t.Fatalf("dashboard %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<a class="adm-nav-i" href="/admin" aria-current="page"><span>Dashboard</span>`,
		`href="/admin/posts"><span>posts</span><span class="n">3</span>`,
		`<a class="adm-nav-i sub" href="/admin/notes"><span>notes</span>`, `<a class="adm-nav-i sub" href="/admin/projects"><span>projects</span>`, `<a class="adm-nav-i" href="/admin/radar"><span>Radar</span>`, `<span class="adm-nav-i is-off" aria-disabled="true"><span>Jobs</span>`,
		`<span class="adm-env">staging</span>`,
		`last deploy a3f91c2`,
		`href="/admin/posts/` + draft + `"`, "Bilingual FTS, part 3",
		"Radar notes", `<span class="n ok">1 this week</span>`, "en · tr ✕",
		"entry.changed #000001", "dead · attempt 4/4 · timeout",
		"/2019/05/docker-notes/",
		`/static/vendor/htmx.min.js`,
		`<a class="adm-btn" href="/admin/quick">+ quick capture</a>`,
		`<a class="bm-chip mono" href="javascript:`, `data-bookmarklet`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("dashboard missing %q", want)
		}
	}
	if strings.Contains(body, "/old/") {
		t.Error("404 older than 7 days is listed")
	}
	if !strings.Contains(body, "webmentions to moderate") || !strings.Contains(body, "queue empty") {
		t.Error("webmentions panel missing or not empty")
	}
}

func TestModerateWebmention(t *testing.T) {
	mux, s, c := signedIn(t)
	id := entry(t, s.DB, "htmx", "Notes on htmx", content.Published, 100)
	for i, st := range []string{"pending", "pending"} {
		exec(t, s.DB, `INSERT INTO webmentions (id, target_entry_id, target_url, source_url, author, content, type, status, received_at, updated_at)
VALUES (?, ?, 'https://halid.org/en/posts/htmx', ?, 'Ada', 'Nice post', 'reply', ?, ?, ?)`,
			"01WM00000000000000000000"+string(rune('A'+i))+"0", id, "https://blog.example/p/"+string(rune('a'+i)), st, 10+i, 10+i)
	}
	body := do(mux, c, "GET", "/admin", nil, false).Body.String()
	for _, want := range []string{`<span class="n muted" id="wm-n" hx-swap-oob="true">2</span>`, "blog.example</a> · reply", "→ Notes on htmx", "Ada: Nice post",
		`hx-post="/admin/webmentions/01WM00000000000000000000A0/approve"`} {
		if !strings.Contains(body, want) {
			t.Errorf("panel missing %q", want)
		}
	}

	// htmx approve: the row goes, the count comes back out of band, and the
	// post's pages are re-rendered.
	rec := do(mux, c, "POST", "/admin/webmentions/01WM00000000000000000000A0/approve", url.Values{}, true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `id="wm-n" hx-swap-oob="true">1</span>`) {
		t.Fatalf("approve %d %s", rec.Code, rec.Body.String())
	}
	sent := s.Queue.(*storetest.Queue).Sent
	if len(sent) != 1 || !strings.Contains(string(sent[0]), `/en/posts/htmx`) {
		t.Fatalf("approve queued %+v", sent)
	}
	// Plain reject: back to the dashboard, nothing to re-render.
	if rec := do(mux, c, "POST", "/admin/webmentions/01WM00000000000000000000B0/reject", url.Values{}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("reject %d", rec.Code)
	}
	if n := len(s.Queue.(*storetest.Queue).Sent); n != 1 {
		t.Errorf("reject queued a re-render (%d messages)", n)
	}
	res, _ := s.DB.Query(context.Background(), `SELECT status FROM webmentions ORDER BY id`)
	rows, _ := store.Scan[struct {
		Status string `json:"status"`
	}](res)
	if len(rows) != 2 || rows[0].Status != "approved" || rows[1].Status != "rejected" {
		t.Errorf("statuses %+v", rows)
	}
	if rec := do(mux, c, "POST", "/admin/webmentions/nope/approve", url.Values{}, false); rec.Code != 404 {
		t.Errorf("unknown webmention: %d", rec.Code)
	}
}

func TestRetryJob(t *testing.T) {
	mux, s, c := signedIn(t)
	exec(t, s.DB, `INSERT INTO job_runs (id, type, payload, status, attempt, error, created_at) VALUES ('01JOBFAILED000000000000002', 'entry.changed', '{"entry_id":"x"}', 'failed', 2, 'boom', 1)`)
	rec := do(mux, c, "POST", "/admin/jobs/01JOBFAILED000000000000002/retry", url.Values{}, true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "queued · retrying") {
		t.Fatalf("retry %d %s", rec.Code, rec.Body.String())
	}
	if n := len(s.Queue.(*storetest.Queue).Sent); n != 1 {
		t.Errorf("queued %d messages", n)
	}
	if rec := do(mux, c, "POST", "/admin/jobs/nope/retry", url.Values{}, false); rec.Code != 404 {
		t.Errorf("unknown job: %d", rec.Code)
	}
}

func TestCrossSitePostIsRejected(t *testing.T) {
	mux, _, c := signedIn(t)
	req := httptest.NewRequest("POST", "/admin/redirects", strings.NewReader("from=/a&to=/b"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.AddCookie(c)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("cross-site post: %d", rec.Code)
	}
}

func TestAddRedirect(t *testing.T) {
	mux, s, c := signedIn(t)
	exec(t, s.DB, `INSERT INTO not_found_log (path, hits, last_seen_at) VALUES ('/tag/golang/', 21, ?)`, time.Now().Unix())

	// Plain form: 303 back to the dashboard; the 404 row is gone.
	rec := do(mux, c, "POST", "/admin/redirects", url.Values{"from": {"https://halid.org/tag/golang/"}, "to": {"/en/tags/go"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("redirect post %d %s", rec.Code, rec.Body.String())
	}
	res, _ := s.DB.Query(context.Background(), `SELECT to_path, code FROM redirects WHERE from_path = '/tag/golang/'`)
	rows, _ := store.Scan[struct {
		To   string `json:"to_path"`
		Code int    `json:"code"`
	}](res)
	if len(rows) != 1 || rows[0].To != "/en/tags/go" || rows[0].Code != 301 {
		t.Fatalf("redirect row %+v", rows)
	}
	if body := do(mux, c, "GET", "/admin", nil, false).Body.String(); strings.Contains(body, "<td>/tag/golang/</td>") {
		t.Error("redirected path still listed as 404")
	}

	// Tombstone via htmx: the panel comes back with the saved note.
	rec = do(mux, c, "POST", "/admin/redirects", url.Values{"from": {"/gone/"}, "gone": {"1"}}, true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "/gone/ → 410") {
		t.Fatalf("410 via htmx: %d %s", rec.Code, rec.Body.String())
	}

	// Validation: errors render, nothing is stored.
	for _, f := range []url.Values{
		{"from": {"/admin/x"}, "to": {"/"}},
		{"from": {"/a"}, "to": {"https://evil.example/"}},
		{"from": {"/a"}, "to": {"/a"}},
		{"from": {"no-slash"}, "to": {"/b"}},
	} {
		rec := do(mux, c, "POST", "/admin/redirects", f, false)
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), `class="adm-error"`) {
			t.Errorf("%v: %d", f, rec.Code)
		}
	}
}

func TestAdminSearchIncludesDrafts(t *testing.T) {
	mux, s, c := signedIn(t)
	id := entry(t, s.DB, "ozel-taslak", "Özel taslak", content.Draft, 0)
	doc := search.Doc{EntryID: id, Kind: "post", Lang: "en", Status: content.Draft, Visibility: content.Public, Slug: "ozel-taslak", Title: "Özel taslak", Body: "gizli"}
	if _, err := s.Search.Batch(context.Background(), doc.Upsert()...); err != nil {
		t.Fatal(err)
	}
	rec := do(mux, c, "GET", "/admin/search?q=ozel", nil, true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Özel taslak") || !strings.Contains(rec.Body.String(), "en · draft") {
		t.Fatalf("htmx search %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "<html") {
		t.Error("htmx search returned a full page")
	}
	rec = do(mux, c, "GET", "/admin/search?q=nothing-here", nil, false)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "No results.") || !strings.Contains(rec.Body.String(), `value="nothing-here"`) {
		t.Fatalf("full search page %d", rec.Code)
	}
}

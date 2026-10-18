package github_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/github"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

type fakeFetch struct{ token string }

func (f *fakeFetch) Fetch(context.Context) (github.Snapshot, error) {
	created := time.Date(2025, 11, 2, 0, 0, 0, 0, time.UTC)
	return github.Snapshot{Complete: true, Pinned: []string{"R1"}, Weeks: []github.Week{{Start: created, Contributions: 3}},
		Repos: []github.Repo{{ID: "R1", FullName: "halid/site", Name: "site", URL: "https://github.com/halid/site", Stars: 42, CreatedAt: &created}}}, nil
}

// deliver runs a job the way the Worker delivers it: POST /internal/jobs.
func deliver(t *testing.T, mux *http.ServeMux, id, token string) {
	t.Helper()
	body, _ := json.Marshal(jobs.Message{ID: id, Type: github.TypeSync, Payload: json.RawMessage(`{}`)})
	req := httptest.NewRequest("POST", "/internal/jobs", strings.NewReader(string(body)))
	req.Header.Set(jobs.InternalHeader, "jobs")
	if token != "" {
		req.Header.Set(github.TokenHeader, token)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("delivery %d %s", rec.Code, rec.Body.String())
	}
}

func TestSyncJobAndAdmin(t *testing.T) {
	ctx := context.Background()
	s := storetest.Stores(t)
	j := jobs.New(s.DB, s.Queue)
	var used []string
	github.NewSync(s.DB, "", j).WithClient(func(tok string) github.Fetcher { used = append(used, tok); return &fakeFetch{tok} }).Register(j)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	adm, err := github.NewAdmin(s.DB, admin.NewUI(s.DB), a, j)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	j.Register(mux)
	adm.Register(mux)
	c, _ := a.TestSession(ctx, "me@halid.org", time.Hour)
	do := func(method, target string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Accept-Language", "en")
		req.AddCookie(c)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	// "run now" queues the job; delivered without a token, it ends dead and says why.
	if rec := do("POST", "/admin/github/sync", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("run now %d", rec.Code)
	}
	first := sent(t, s, 0).ID
	deliver(t, mux, first, "")
	body := do("GET", "/admin/github", nil).Body.String()
	for _, want := range []string{"The last sync found no token", `<span class="adm-st s-archived">dead</span>`, "No repos yet."} {
		if !strings.Contains(body, want) {
			t.Errorf("admin missing %q", want)
		}
	}
	if _, err := s.DB.Query(ctx, `UPDATE job_runs SET created_at = created_at - 60`); err != nil { // the next run is later
		t.Fatal(err)
	}
	// With the token on the delivery the sync writes and queues the page refresh.
	if _, err := j.Enqueue(ctx, github.TypeSync, struct{}{}); err != nil {
		t.Fatal(err)
	}
	second := sent(t, s, -1).ID
	deliver(t, mux, second, "tok-from-worker")
	if len(used) != 1 || used[0] != "tok-from-worker" {
		t.Fatalf("token used %v", used)
	}
	last := sent(t, s, -1)
	if last.Type != jobs.TypePagesChanged || !strings.Contains(string(last.Payload), "/en/projects") {
		t.Errorf("refresh %+v", last)
	}
	// The current month's stream archive page, not one from long ago.
	if p := string(last.Payload); !strings.Contains(p, "/tr/stream/"+time.Now().UTC().Format("2006-01")) || strings.Contains(p, "/stream/2025-11") {
		t.Errorf("month pages %s", p)
	}
	body = do("GET", "/admin/github", nil).Body.String()
	for _, want := range []string{`>halid/site</a>`, "★ 42", `<span class="adm-st s-published">done</span>`, `pinned</span> unpin`} {
		if !strings.Contains(body, want) {
			t.Errorf("admin missing %q", want)
		}
	}
	if strings.Contains(body, "found no token") {
		t.Error("old no-token warning after a good sync")
	}
	// Unpin, then link the repo to a project.
	if rec := do("POST", "/admin/github/repos/R1/pin", url.Values{"pinned": {"0"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("unpin %d", rec.Code)
	}
	if n := count(t, s.DB, `SELECT COUNT(*) AS n FROM gh_repos WHERE pinned = 1`); n != 0 {
		t.Error("still pinned")
	}
	if _, err := s.DB.Batch(ctx,
		store.Stmt{SQL: `INSERT INTO entries (id, kind, slug, lang, translation_group_id, title, status, visibility, created_at, updated_at)
VALUES ('P1', 'project', 'site', 'en', 'P1', 'halid.org', 'draft', 'public', 1, 1)`},
		store.Stmt{SQL: `INSERT INTO projects (entry_id, project_type) VALUES ('P1', 'software')`}); err != nil {
		t.Fatal(err)
	}
	if rec := do("POST", "/admin/github/repos/R1/link", url.Values{"project": {"P1"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("link %d", rec.Code)
	}
	if n := count(t, s.DB, `SELECT COUNT(*) AS n FROM projects WHERE github_repo = 'halid/site'`); n != 1 {
		t.Error("not linked")
	}
	if !strings.Contains(do("GET", "/admin/github", nil).Body.String(), `<option value="P1" selected>halid.org · en</option>`) {
		t.Error("link not shown")
	}
}

// sent decodes a queued message; i < 0 counts from the end.
func sent(t *testing.T, s store.Stores, i int) jobs.Message {
	t.Helper()
	q := s.Queue.(*storetest.Queue).Sent
	if i < 0 {
		i += len(q)
	}
	var m jobs.Message
	if err := json.Unmarshal(q[i], &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func count(t *testing.T, db store.DB, sql string) int {
	t.Helper()
	res, err := db.Query(context.Background(), sql)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := store.Scan[struct {
		N int `json:"n"`
	}](res)
	return rows[0].N
}

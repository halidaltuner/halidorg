package github

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

const page1 = `{"data":{"viewer":{"login":"halid",
 "pinnedItems":{"nodes":[{"id":"R2"},{"id":"R1"}]},
 "contributionsCollection":{"contributionCalendar":{"totalContributions":60,"weeks":[
   {"firstDay":"2026-10-04","contributionDays":[{"contributionCount":10},{"contributionCount":20}]},
   {"firstDay":"2026-10-11","contributionDays":[{"contributionCount":0}]},
   {"firstDay":"2026-09-27","contributionDays":[{"contributionCount":15},{"contributionCount":15}]}]}},
 "repositories":{"pageInfo":{"hasNextPage":true,"endCursor":"c1"},"nodes":[
   {"id":"R1","nameWithOwner":"halid/site","name":"site","description":"This site.","url":"https://github.com/halid/site",
    "stargazerCount":42,"forkCount":1,"isArchived":false,"isFork":false,"pushedAt":"2026-10-18T10:00:00Z","createdAt":"2025-11-02T00:00:00Z",
    "primaryLanguage":{"name":"Go"},"repositoryTopics":{"nodes":[{"topic":{"name":"htmx"}}]},
    "languages":{"edges":[{"size":800,"node":{"name":"Go"}},{"size":200,"node":{"name":"HTML"}}]},
    "releases":{"nodes":[{"id":"V1","tagName":"v0.4.0","name":null,"url":"https://github.com/halid/site/releases/v0.4.0","publishedAt":"2026-10-09T00:00:00Z","description":"Webmentions.\nMore."},
                         {"id":"V0","tagName":"v0.5.0-draft","url":"https://github.com/halid/site/releases/x","publishedAt":null}]}}]}}}}`

const page2 = `{"data":{"viewer":{"login":"halid","pinnedItems":{"nodes":[]},"contributionsCollection":null,
 "repositories":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[
   {"id":"R2","nameWithOwner":"halid/dotfiles","name":"dotfiles","description":null,"url":"https://github.com/halid/dotfiles",
    "stargazerCount":29,"forkCount":0,"isArchived":false,"isFork":false,"pushedAt":"2026-09-01T00:00:00Z","createdAt":"2020-01-01T00:00:00Z",
    "primaryLanguage":{"name":"Shell"},"repositoryTopics":{"nodes":[]},"languages":{"edges":[{"size":1000,"node":{"name":"Shell"}}]},
    "releases":{"nodes":[]}}]}}}}`

func fakeGitHub(t *testing.T, pages ...string) (*Client, *int) {
	t.Helper()
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "bearer tok" {
			t.Errorf("auth %q", r.Header.Get("Authorization"))
		}
		var body struct {
			Variables map[string]string `json:"variables"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		i := calls
		calls++
		if i == 1 && body.Variables["after"] != "c1" {
			t.Errorf("second page without the cursor: %v", body.Variables)
		}
		_, _ = w.Write([]byte(pages[min(i, len(pages)-1)]))
	}))
	t.Cleanup(srv.Close)
	return &Client{HTTP: srv.Client(), Endpoint: srv.URL, Token: "tok"}, &calls
}

func count(t *testing.T, db store.DB, sql string, args ...any) int {
	t.Helper()
	res, err := db.Query(context.Background(), sql, args...)
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := store.Scan[struct {
		N int `json:"n"`
	}](res)
	return rows[0].N
}

func TestFetchWriteLoad(t *testing.T) {
	ctx := context.Background()
	c, calls := fakeGitHub(t, page1, page2)
	snap, err := c.Fetch(ctx)
	if err != nil || *calls != 2 || !snap.Complete || len(snap.Repos) != 2 || len(snap.Weeks) != 3 || snap.Total != 60 ||
		len(snap.Pinned) != 2 || len(snap.Repos[0].Releases) != 1 || snap.Repos[0].Releases[0].Note != "Webmentions." {
		t.Fatalf("snapshot %+v %v", snap, err)
	}
	db := storetest.NewDB(t)
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	if err := Write(ctx, db, snap, now); err != nil {
		t.Fatal(err)
	}
	// Re-running is idempotent: no duplicate activity, pins seeded once.
	if err := Write(ctx, db, snap, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) AS n FROM gh_activity`); n != 3 { // 1 release + 2 repos
		t.Errorf("activity %d", n)
	}
	if _, err := db.Query(ctx, `UPDATE gh_repos SET pinned = 0 WHERE id = 'R1'`); err != nil { // the admin unpins
		t.Fatal(err)
	}
	if err := Write(ctx, db, snap, now.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) AS n FROM gh_repos WHERE pinned = 1`); n != 1 {
		t.Errorf("the sync overrode the admin's pins: %d pinned", n)
	}

	s, err := Load(ctx, db, "en")
	if err != nil || s == nil {
		t.Fatalf("load %v %v", s, err)
	}
	if s.Total != 60 || len(s.Weeks) != 3 || s.Weeks[0].Count != 30 || s.Weeks[1].Level != 3 || s.Weeks[2].Height != 8 || s.Labels[0] != "2026-09" {
		t.Errorf("weeks %+v %v", s.Weeks, s.Labels)
	}
	if len(s.Langs) != 3 || s.Langs[0].Name != "Shell" || s.Langs[0].Percent != 50 || s.Langs[1].Name != "Go" || s.Langs[1].Percent != 40 {
		t.Errorf("langs %+v", s.Langs)
	}
	if len(s.Pinned) != 1 || s.Pinned[0].Name != "dotfiles" || s.Pinned[0].Stars != 29 {
		t.Errorf("pinned %+v", s.Pinned)
	}
	if len(s.Releases) != 1 || s.Releases[0].Title != "site v0.4.0" || s.Releases[0].Date != "2026-10-09" {
		t.Errorf("releases %+v", s.Releases)
	}
	st, err := Stats(ctx, db, "en", []string{"Halid/Site", "halid/missing"}, now)
	if err != nil || st["halid/site"].Stars != 42 || st["halid/site"].Activity != "pushed 2h ago" || len(st) != 1 {
		t.Errorf("stats %+v %v", st, err)
	}

	// A complete sync without dotfiles marks it gone, keeps the row.
	snap.Repos = snap.Repos[:1]
	if err := Write(ctx, db, snap, now.Add(3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if n := count(t, db, `SELECT COUNT(*) AS n FROM gh_repos WHERE gone = 1 AND id = 'R2'`); n != 1 {
		t.Error("missing repo not marked gone")
	}
	if s, _ := Load(ctx, db, "en"); len(s.Pinned) != 0 {
		t.Error("gone repo still pinned on the page")
	}
}

func TestRateLimitAndPartial(t *testing.T) {
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", "1800000000")
		w.WriteHeader(http.StatusForbidden)
	}))
	defer srv.Close()
	_, err := (&Client{HTTP: srv.Client(), Endpoint: srv.URL, Token: "tok"}).Fetch(ctx)
	var rl RateLimitError
	if !errors.As(err, &rl) || rl.Reset.Unix() != 1800000000 || !strings.Contains(err.Error(), "rate limited until 2027-01-15") {
		t.Fatalf("rate limit: %v", err)
	}
	if _, err := (&Client{Token: ""}).Fetch(ctx); !errors.Is(err, ErrNoToken) {
		t.Errorf("no token: %v", err)
	}
	// GitHub answers with data and an error: the data is used, the error kept.
	partial := strings.Replace(page2, `{"data"`, `{"errors":[{"type":"SERVICE_UNAVAILABLE","message":"contributions timed out"}],"data"`, 1)
	c, _ := fakeGitHub(t, partial)
	snap, err := c.Fetch(ctx)
	if err != nil || len(snap.Repos) != 1 || snap.Weeks != nil || len(snap.Partial) != 1 {
		t.Fatalf("partial %+v %v", snap, err)
	}
}

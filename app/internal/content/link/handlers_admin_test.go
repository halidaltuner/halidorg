package link

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store/storetest"
)

func TestReadToggle(t *testing.T) {
	t.Setenv("APP_ENV", "staging")
	ctx := context.Background()
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	mux := http.NewServeMux()
	NewAdmin(s.DB, a, jobs.New(s.DB, s.Queue)).Register(mux)
	cookie, err := a.TestSession(ctx, "me@halid.org", time.Hour) // no recent sign-in needed
	if err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(s.DB)
	l, err := repo.Create(ctx, Draft{Link: Link{Entry: content.Entry{Slug: "grug", Lang: "en", Title: "Grug"}, URL: "https://grugbrain.dev/"}})
	if err != nil {
		t.Fatal(err)
	}
	do := func(c *http.Cookie, method, target, body, site string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Sec-Fetch-Site", site)
		if c != nil {
			req.AddCookie(c)
		}
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}

	if rec := do(cookie, "GET", "/admin/links/session", "", "same-origin"); rec.Code != http.StatusNoContent {
		t.Errorf("session signed in: %d", rec.Code)
	}
	if rec := do(nil, "GET", "/admin/links/session", "", "same-origin"); rec.Code != http.StatusSeeOther {
		t.Errorf("session signed out: %d", rec.Code)
	}

	rec := do(cookie, "POST", "/admin/links/"+l.ID+"/read", "status=read", "same-origin")
	if rec.Code != 200 || rec.Body.String() != "read" {
		t.Fatalf("toggle %d %q", rec.Code, rec.Body.String())
	}
	got, _ := repo.Get(ctx, l.ID)
	if got.ReadStatus != Read {
		t.Errorf("status %q", got.ReadStatus)
	}
	if n := len(s.Queue.(*storetest.Queue).Sent); n != 1 {
		t.Errorf("render jobs queued: %d", n)
	}

	for name, rec := range map[string]*httptest.ResponseRecorder{
		"cross-site":   do(cookie, "POST", "/admin/links/"+l.ID+"/read", "status=unread", "cross-site"),
		"signed out":   do(nil, "POST", "/admin/links/"+l.ID+"/read", "status=unread", "same-origin"),
		"bad status":   do(cookie, "POST", "/admin/links/"+l.ID+"/read", "status=maybe", "same-origin"),
		"no such link": do(cookie, "POST", "/admin/links/01JABCDEFGHJKMNPQRSTVWXYZ0/read", "status=unread", "same-origin"),
	} {
		if rec.Code == 200 {
			t.Errorf("%s: accepted", name)
		}
	}
	if got, _ := repo.Get(ctx, l.ID); got.ReadStatus != Read {
		t.Errorf("refused toggles changed the status: %q", got.ReadStatus)
	}
}

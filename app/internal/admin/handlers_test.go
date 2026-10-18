package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

func newMux(t *testing.T, cfg auth.Config) *http.ServeMux {
	mux, _, _ := newSite(t, cfg)
	return mux
}

// newSite builds the admin routes over SQLite stores.
func newSite(t *testing.T, cfg auth.Config) (*http.ServeMux, store.Stores, *auth.Auth) {
	t.Helper()
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DEPLOY_SHA", "a3f91c2deadbeef")
	s := storetest.Stores(t)
	a := auth.New(cfg, s.DB)
	h, err := New(a, NewUI(s.DB), s.DB, s.Search, jobs.New(s.DB, s.Queue))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, s, a
}

func serve(mux *http.ServeMux, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestLoginPageDefaultState(t *testing.T) {
	rec := serve(newMux(t, auth.Config{}), "GET", "/admin/login")
	body := rec.Body.String()
	for _, want := range []string{"Continue with Google", `data-loading="signing in…"`, `class="env-badge">staging`, "deploy a3f91c2 · staging", "@halid.org", `action="/admin/login"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(body, "login-error") {
		t.Error("default state must not show the error banner")
	}
	if rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
	}
}

func TestLoginFailsClosedWithoutSecrets(t *testing.T) {
	rec := serve(newMux(t, auth.Config{}), "POST", "/admin/login")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "Sign-in did not complete") {
		t.Fatalf("got %d", rec.Code)
	}
}

func TestLoginRedirectsToGoogle(t *testing.T) {
	cfg := auth.Config{ClientID: "id", ClientSecret: "s", SessionSecret: strings.Repeat("k", 32), PublicOrigin: "https://staging.halid.org"}
	rec := serve(newMux(t, cfg), "POST", "/admin/login")
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "https://accounts.google.com/") {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}
}

func TestAdminNeedsSession(t *testing.T) {
	mux := newMux(t, auth.Config{})
	for _, path := range []string{"/admin", "/admin/reauth"} {
		if rec := serve(mux, "GET", path); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != auth.LoginPath {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Header().Get("Location"))
		}
	}
	if rec := serve(mux, "POST", "/admin/logout"); rec.Code != http.StatusSeeOther {
		t.Errorf("logout without session: %d", rec.Code)
	}
}

func TestLoginFollowsLangCookie(t *testing.T) {
	mux := newMux(t, auth.Config{})
	req := httptest.NewRequest("GET", "/admin/login", nil)
	req.Header.Set("Accept-Language", "en-US")
	req.AddCookie(&http.Cookie{Name: "lang", Value: "tr"})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()
	for _, want := range []string{`<html lang="tr">`, "Google ile devam et", `href="/lang/en?next=%2Fadmin%2Flogin"`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
}

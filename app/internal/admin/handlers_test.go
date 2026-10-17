package admin

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/store"
)

type noDB struct{}

func (noDB) Query(context.Context, string, ...any) (store.Result, error) { return store.Result{}, nil }
func (noDB) Batch(context.Context, ...store.Stmt) ([]store.Result, error) {
	return nil, errors.New("unused")
}

func newMux(t *testing.T, cfg auth.Config) *http.ServeMux {
	t.Helper()
	t.Setenv("APP_ENV", "staging")
	t.Setenv("DEPLOY_SHA", "a3f91c2deadbeef")
	h, err := New(auth.New(cfg, noDB{}))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
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

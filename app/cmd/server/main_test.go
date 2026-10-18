package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"halid.org/engine/internal/store/storetest"
)

func TestRoutes(t *testing.T) {
	mux, err := routes(storetest.Stores(t))
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]int{
		"/healthz":               http.StatusOK,
		"/en/":                   http.StatusOK,
		"/admin/login":           http.StatusOK,
		"/admin":                 http.StatusSeeOther,
		"/_styleguide":           http.StatusOK,
		"/static/css/tokens.css": http.StatusOK,
		"/static/js/header.js":   http.StatusOK,
		"/static/fonts/ibm-plex-sans-latin-400-normal.woff2": http.StatusOK,
		"/static/css/missing.css":                            http.StatusNotFound,
		"/static/":                                           http.StatusNotFound,
		"/robots.txt":                                        http.StatusOK,
		"/lang/en":                                           http.StatusSeeOther,
		"/internal/jobs":                                     http.StatusNotFound,
		"/en/posts":                                          http.StatusOK,
		"/tr/feed.xml":                                       http.StatusOK,
		"/sitemap.xml":                                       http.StatusOK,
		"/en/posts/missing":                                  http.StatusNotFound,
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: got %d, want %d", path, rec.Code, want)
		}
	}
}

func TestStyleguideIs404InProd(t *testing.T) {
	t.Setenv("APP_ENV", "prod")
	mux, err := routes(storetest.Stores(t))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/_styleguide", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

// The jobs endpoint only answers the Worker's queue consumer.
func TestInternalJobsNeedsHeader(t *testing.T) {
	mux, err := routes(storetest.Stores(t))
	if err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("POST", "/internal/jobs", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

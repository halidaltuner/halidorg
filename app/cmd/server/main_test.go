package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"halid.org/engine/internal/store/storetest"
	"halid.org/engine/web"
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
		"/en/snippets":                                       http.StatusOK,
		"/tr/notes":                                          http.StatusOK,
		"/en/snippets/missing":                               http.StatusNotFound,
		"/en/snippets/missing/raw/main.go":                   http.StatusNotFound,
		"/en/notes/missing":                                  http.StatusNotFound,
		"/en/snippets/feed.xml":                              http.StatusOK,
		"/static/js/snippet.js":                              http.StatusOK,
		"/en/links":                                          http.StatusOK,
		"/tr/favs?type=book":                                 http.StatusOK,
		"/en/awesome/none":                                   http.StatusNotFound,
		"/tr/awesome":                                        http.StatusOK,
		"/static/js/filter.js":                               http.StatusOK,
		"/static/js/links.js":                                http.StatusOK,
		"/admin/links/session":                               http.StatusSeeOther,
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: got %d, want %d", path, rec.Code, want)
		}
	}
}

// Versioned assets (web.Asset) are immutable; plain paths stay short.
func TestStaticCaching(t *testing.T) {
	mux, err := routes(storetest.Stores(t))
	if err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{
		web.Asset("/static/css/app.css"):   "public, max-age=31536000, immutable",
		"/static/css/app.css?v=0123456789": "public, max-age=60", // an older page's hash
		"/static/css/app.css":              "public, max-age=3600",
	} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != 200 || rec.Header().Get("Cache-Control") != want {
			t.Errorf("%s: %d %q", path, rec.Code, rec.Header().Get("Cache-Control"))
		}
	}
	// Pages link the versioned URL.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/en/", nil))
	if !strings.Contains(rec.Body.String(), `href="`+web.Asset("/static/css/app.css")+`"`) {
		t.Error("home does not link the versioned stylesheet")
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

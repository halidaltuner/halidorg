package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRoutes(t *testing.T) {
	mux, err := routes()
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]int{
		"/healthz":               http.StatusOK,
		"/en/":                   http.StatusOK,
		"/admin/login":           http.StatusOK,
		"/admin":                 http.StatusSeeOther,
		"/static/css/tokens.css": http.StatusOK,
		"/static/fonts/ibm-plex-sans-latin-400-normal.woff2": http.StatusOK,
		"/static/css/missing.css":                            http.StatusNotFound,
		"/static/":                                           http.StatusNotFound,
	}
	for path, want := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		if rec.Code != want {
			t.Errorf("%s: got %d, want %d", path, rec.Code, want)
		}
	}
}

package public

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newMux(t *testing.T) *http.ServeMux {
	t.Helper()
	h, err := New()
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux
}

func TestRootRedirectsByAcceptLanguage(t *testing.T) {
	mux := newMux(t)
	cases := map[string]string{
		"tr-TR,tr;q=0.9,en;q=0.8": "/tr/",
		"en-US,en;q=0.9":          "/en/",
		"de-DE":                   "/en/",
		"":                        "/en/",
		"en;q=0.5,tr;q=0.7":       "/tr/",
	}
	for header, want := range cases {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Language", header)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusFound || rec.Header().Get("Location") != want {
			t.Errorf("Accept-Language %q: got %d %q, want 302 %q", header, rec.Code, rec.Header().Get("Location"), want)
		}
		if !strings.Contains(rec.Header().Get("Cache-Control"), "no-store") {
			t.Errorf("redirect must not be cacheable, got %q", rec.Header().Get("Cache-Control"))
		}
	}
}

func TestHello(t *testing.T) {
	mux := newMux(t)
	for lang, want := range map[string]string{"tr": "Merhaba", "en": "Hello"} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/"+lang+"/", nil))
		body := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", lang, rec.Code)
		}
		if !strings.Contains(body, "<h1>"+want+"</h1>") || !strings.Contains(body, `<html lang="`+lang+`">`) {
			t.Errorf("%s: unexpected body:\n%s", lang, body)
		}
		if !strings.Contains(body, `hreflang="tr"`) || !strings.Contains(body, `hreflang="en"`) {
			t.Errorf("%s: missing hreflang alternates", lang)
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.HasPrefix(cc, "public") {
			t.Errorf("%s: Cache-Control %q, want public opt-in", lang, cc)
		}

		etag := rec.Header().Get("ETag")
		req := httptest.NewRequest("GET", "/"+lang+"/", nil)
		req.Header.Set("If-None-Match", etag)
		rec = httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotModified {
			t.Errorf("%s: If-None-Match got %d, want 304", lang, rec.Code)
		}
	}
}

func TestUnknownLangIs404(t *testing.T) {
	rec := httptest.NewRecorder()
	newMux(t).ServeHTTP(rec, httptest.NewRequest("GET", "/de/", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("got %d, want 404", rec.Code)
	}
}

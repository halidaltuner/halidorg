package public

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

func newMux(t *testing.T) *http.ServeMux {
	t.Helper()
	mux, _ := newSite(t)
	return mux
}

func newSite(t *testing.T) (*http.ServeMux, store.DB) {
	mux, db, _ := newSiteSearch(t)
	return mux, db
}

func newSiteSearch(t *testing.T) (*http.ServeMux, store.DB, store.DB) {
	t.Helper()
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	db := storetest.NewDB(t)
	sdb := storetest.NewSearchDB(t)
	h, err := New(db, sdb)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, db, sdb
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

func TestHomeEmpty(t *testing.T) {
	mux := newMux(t)
	for lang, want := range map[string]string{"tr": "Henüz yayınlanmış yazı yok.", "en": "No posts published yet."} {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", "/"+lang+"/", nil))
		body := rec.Body.String()
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d", lang, rec.Code)
		}
		if !strings.Contains(body, want) || !strings.Contains(body, `<html lang="`+lang+`">`) {
			t.Errorf("%s: unexpected body:\n%s", lang, body)
		}
		if !strings.Contains(body, `hreflang="tr"`) || !strings.Contains(body, `hreflang="en"`) ||
			!strings.Contains(body, `<link rel="canonical" href="https://halid.org/`+lang+`/">`) {
			t.Errorf("%s: missing hreflang or canonical", lang)
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

func TestSetLang(t *testing.T) {
	mux := newMux(t)
	cases := map[string]string{
		"/lang/en?next=%2Fen%2F":                    "/en/",
		"/lang/tr?next=%2Fadmin%2Flogin%3Fnext%3Dx": "/admin/login?next=x",
		"/lang/tr": "/tr/",
		"/lang/tr?next=https%3A%2F%2Fevil.example": "/tr/",
		"/lang/tr?next=%2F%2Fevil.example":         "/tr/",
		"/lang/tr?next=%2F%5Cevil.example":         "/tr/",
		"/lang/tr?next=evil":                       "/tr/",
	}
	for target, want := range cases {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", target, nil))
		if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != want {
			t.Errorf("%s: got %d %q, want 303 %q", target, rec.Code, rec.Header().Get("Location"), want)
		}
		c := rec.Result().Cookies()
		if len(c) != 1 || c[0].Name != "lang" || c[0].Path != "/" || !c[0].Secure || c[0].MaxAge <= 0 {
			t.Errorf("%s: cookie %+v", target, c)
		}
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/lang/de?next=%2F", nil))
	if rec.Code != http.StatusNotFound || len(rec.Result().Cookies()) != 0 {
		t.Errorf("unsupported lang: %d", rec.Code)
	}
}

func TestRootHonoursLangCookie(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Accept-Language", "tr-TR")
	req.AddCookie(&http.Cookie{Name: "lang", Value: "en"})
	rec := httptest.NewRecorder()
	newMux(t).ServeHTTP(rec, req)
	if rec.Header().Get("Location") != "/en/" || !strings.Contains(rec.Header().Get("Vary"), "Cookie") {
		t.Fatalf("got %q vary %q", rec.Header().Get("Location"), rec.Header().Get("Vary"))
	}
}

func TestHomeHasSwitcher(t *testing.T) {
	rec := httptest.NewRecorder()
	newMux(t).ServeHTTP(rec, httptest.NewRequest("GET", "/tr/", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`aria-label="Dil"`,
		`href="/lang/tr?next=%2Ftr%2F" hreflang="tr" lang="tr" aria-current="true">tr</a>`,
		`href="/lang/en?next=%2Fen%2F" hreflang="en" lang="en">en</a>`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in\n%s", want, body)
		}
	}
}

func TestHomeHasChrome(t *testing.T) {
	t.Setenv("DEPLOY_SHA", "a3f91c2deadbeef")
	rec := httptest.NewRecorder()
	newMux(t).ServeHTTP(rec, httptest.NewRequest("GET", "/en/", nil))
	body := rec.Body.String()
	for _, want := range []string{
		`<header class="site-header">`,
		`<a href="/en/posts" data-s="posts">posts</a>`,
		`<details class="nav-sheet">`,
		`<footer class="site-footer">`,
		`<span class="sha">a3f91c2</span>`,
		`/static/js/header.js`,
		`<details class="nav-more">`, `href="/en/notes"`, `href="/en/snippets"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s", want)
		}
	}
	// Sections without pages stay out, and home is not a section page.
	for _, absent := range []string{`aria-current="page"`, "/en/projects", "/en/links"} {
		if strings.Contains(body, absent) {
			t.Errorf("unexpected %s", absent)
		}
	}
}

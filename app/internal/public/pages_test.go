package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/store"
)

func seed(t *testing.T, db store.DB) {
	t.Helper()
	now := time.Unix(1_790_000_000, 0)
	old := content.Clock
	content.Clock = func() time.Time { now = now.Add(time.Hour); return now }
	t.Cleanup(func() { content.Clock = old })
	ctx := context.Background()
	repo := post.NewRepo(db)
	if _, err := db.Query(ctx, `INSERT INTO series (id, slug, title_tr, title_en) VALUES ('s1', 'kromatik', 'kromatik', 'building kromatik')`); err != nil {
		t.Fatal(err)
	}
	s1 := "s1"
	mk := func(slug, lang, title, md, group string, order int64, tags ...string) post.Post {
		d := post.Draft{Post: post.Post{Entry: content.Entry{Slug: slug, Lang: lang, Title: title, Summary: "summary of " + title, TranslationGroupID: group}},
			Markdown: md, Tags: tags}
		if order > 0 {
			d.SeriesID, d.SeriesOrder = &s1, &order
		}
		p, err := repo.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		p, err = repo.Publish(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	first := mk("ghost", "en", "Leaving Ghost", "## Why\n\nBecause.", "", 1, "meta")
	mk("htmx-admin", "en", "Notes on an htmx-first admin", "## Why not an SPA\n\nText[^1].\n\n[^1]: Side.\n\n## Autosave\n\n```go\nx := 1\n```\n", "", 2, "htmx", "go")
	mk("ghosttan-ayrilmak", "tr", "Ghost’tan ayrılmak", "## Neden\n\nÇünkü.", first.TranslationGroupID, 0, "meta")
	mk("only-tr", "tr", "Sadece Türkçe", "Merhaba.", "", 0)
}

func get(t *testing.T, mux *http.ServeMux, path string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	return rec.Code, rec.Body.String()
}

func must(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q", w)
		}
	}
}

func TestHomeAndLists(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db)
	code, body := get(t, mux, "/en/")
	if code != 200 {
		t.Fatalf("home %d", code)
	}
	must(t, body,
		`<a class="featured" href="/en/posts/htmx-admin">`,
		`<h3>Leaving Ghost</h3>`,
		`<a class="stream-row" href="/en/posts/ghost" data-filter-value="post"><span class="glyph k-post mono" aria-hidden="true">¶</span>`,
		`#htmx`,
	)
	if strings.Contains(body, "Sadece") {
		t.Error("tr post on en home")
	}

	code, body = get(t, mux, "/en/posts")
	if code != 200 {
		t.Fatalf("list %d", code)
	}
	must(t, body, `content="https://halid.org/og/en/site-`, "<h1>Posts</h1>", "/en/posts/ghost", "/en/posts/htmx-admin", `<link rel="canonical" href="https://halid.org/en/posts">`)

	code, body = get(t, mux, "/en/tags/go")
	if code != 200 || strings.Contains(body, "/en/posts/ghost\"") {
		t.Fatalf("tag page %d", code)
	}
	must(t, body, "<h1>#go</h1>", "/en/posts/htmx-admin")
	if code, _ := get(t, mux, "/en/tags/unknown"); code != 404 {
		t.Errorf("empty tag: %d", code)
	}
}

func TestPostPage(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db)
	code, body := get(t, mux, "/en/posts/htmx-admin")
	if code != 200 {
		t.Fatalf("post %d", code)
	}
	must(t, body,
		`<title>Notes on an htmx-first admin · halid.org</title>`,
		`<article class="post" lang="en">`,
		`<a class="toc-l2" href="#why-not-an-spa">Why not an SPA</a>`,
		`<aside class="sn" id="sn-1">`,
		`series: <a href="/en/series/kromatik">building kromatik</a> (2/2)`,
		`<a href="/en/posts/ghost"><span class="mono">← 1/2</span>`,
		`tr: not yet`,
		`<meta property="og:type" content="article">`,
		`/static/js/post.js`,
		`min read`,
		`<meta property="og:image" content="https://halid.org/og/en/posts/htmx-admin.png?v=`,
		`<meta name="twitter:card" content="summary_large_image">`,
	)
	if strings.Contains(body, "banner-untranslated") || strings.Contains(body, `name="robots"`) {
		t.Error("translated-language page shows the banner or noindex")
	}

	// Translated post: alternates point at each other, switch goes to the sibling slug.
	_, body = get(t, mux, "/en/posts/ghost")
	must(t, body, `hreflang="tr" href="https://halid.org/tr/posts/ghosttan-ayrilmak"`, `<a href="/tr/posts/ghosttan-ayrilmak" hreflang="tr">tr ↗</a>`)
}

func TestUntranslatedFallback(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db)
	// The tr page for an en-only post renders it with the banner, noindex.
	code, body := get(t, mux, "/tr/posts/htmx-admin")
	if code != 200 {
		t.Fatalf("fallback %d", code)
	}
	must(t, body,
		`<html lang="tr">`,
		`<article class="post" lang="en">`,
		`Bu yazı henüz Türkçeye çevrilmedi.`,
		`<a href="/en/posts/htmx-admin">özgün sürüm →</a>`,
		`<meta name="robots" content="noindex">`,
		`<link rel="canonical" href="https://halid.org/en/posts/htmx-admin">`,
		`content="https://halid.org/og/en/posts/htmx-admin.png?v=`,
	)
	// A translation under another slug: redirect, not banner.
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/tr/posts/ghost", nil))
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/tr/posts/ghosttan-ayrilmak" {
		t.Errorf("sibling redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if code, _ := get(t, mux, "/en/posts/nope"); code != 404 {
		t.Errorf("missing post: %d", code)
	}
}

func TestErrorsAndRedirects(t *testing.T) {
	mux, db := newSite(t)
	ctx := context.Background()
	_, _ = db.Query(ctx, `INSERT INTO redirects (from_path, to_path, code, created_at) VALUES ('/old-ghost-url/', '/en/posts/ghost', 301, 1)`)
	_, _ = db.Query(ctx, `INSERT INTO redirects (from_path, to_path, code, created_at) VALUES ('/gone/', '', 410, 1)`)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/old-ghost-url/", nil))
	if rec.Code != 301 || rec.Header().Get("Location") != "/en/posts/ghost" {
		t.Errorf("redirect: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	code, body := get(t, mux, "/gone/")
	if code != 410 || !strings.Contains(body, "This page was removed") && !strings.Contains(body, "Bu sayfa kaldırıldı") {
		t.Errorf("410: %d", code)
	}
	code, body = get(t, mux, "/tr/yok-boyle-bir-sey")
	if code != 404 || !strings.Contains(body, "Sayfa bulunamadı") || !strings.Contains(body, `<html lang="tr">`) {
		t.Errorf("tr 404: %d", code)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/de/", nil))
	if rec.Code != 404 || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("404 headers: %d %q", rec.Code, rec.Header().Get("Cache-Control"))
	}
}

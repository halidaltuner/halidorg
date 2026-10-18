package feeds

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/store/storetest"
)

func site(t *testing.T, env string) (*http.ServeMux, *post.Repo) {
	t.Helper()
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	t.Setenv("APP_ENV", env)
	now := time.Unix(1_790_000_000, 0)
	old := content.Clock
	content.Clock = func() time.Time { now = now.Add(time.Minute); return now }
	t.Cleanup(func() { content.Clock = old })

	db := storetest.NewDB(t)
	repo := post.NewRepo(db)
	ctx := context.Background()
	mk := func(slug, lang, title, md, group string, publish bool, tags ...string) string {
		d := post.Draft{Post: post.Post{Entry: content.Entry{Slug: slug, Lang: lang, Title: title, Summary: "about " + title, TranslationGroupID: group}},
			Markdown: md, Tags: tags}
		p, err := repo.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if publish {
			if _, err := repo.Publish(ctx, p.ID); err != nil {
				t.Fatal(err)
			}
		}
		return p.TranslationGroupID
	}
	g := mk("first", "en", "First", "![x](/media/a.png) [home](/en/) [cdn](//cdn.example/x)", "", true, "go")
	mk("ilk", "tr", "İlk", "merhaba", g, true, "go")
	mk("second", "en", "Second & <more>", "body two", "", true, "htmx")
	mk("draft", "en", "Draft", "hidden", "", false, "go")

	mux := http.NewServeMux()
	New(db).Register(mux)
	return mux, repo
}

func get(t *testing.T, mux *http.ServeMux, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
	if rec.Code != 200 {
		t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "public") || rec.Header().Get("ETag") == "" {
		t.Errorf("%s: not cacheable: %q", path, cc)
	}
	return rec
}

func TestRSS(t *testing.T) {
	mux, _ := site(t, "staging")
	rec := get(t, mux, "/en/feed.xml")
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/rss+xml") {
		t.Errorf("content type %q", ct)
	}
	var doc struct {
		Channel struct {
			Title string `xml:"title"`
			Items []struct {
				Title   string `xml:"title"`
				Link    string `xml:"link"`
				Content string `xml:"encoded"`
			} `xml:"item"`
		} `xml:"channel"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &doc); err != nil {
		t.Fatalf("invalid xml: %v\n%s", err, rec.Body)
	}
	items := doc.Channel.Items
	if doc.Channel.Title != "halid.org · posts" || len(items) != 2 {
		t.Fatalf("channel %q items %d", doc.Channel.Title, len(items))
	}
	if items[0].Title != "Second & <more>" || items[1].Link != "https://halid.org/en/posts/first" {
		t.Errorf("items = %+v", items)
	}
	c := items[1].Content
	for _, want := range []string{`src="https://halid.org/media/a.png"`, `href="https://halid.org/en/"`, `href="//cdn.example/x"`} {
		if !strings.Contains(c, want) {
			t.Errorf("content misses %s: %s", want, c)
		}
	}

	tag := get(t, mux, "/en/tags/go/feed.xml").Body.String()
	if !strings.Contains(tag, "/en/posts/first") || strings.Contains(tag, "/en/posts/second") || strings.Contains(tag, "draft") {
		t.Errorf("tag feed: %s", tag)
	}
	if !strings.Contains(get(t, mux, "/tr/feed.xml").Body.String(), "halid.org · yazılar") {
		t.Error("tr feed title")
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/en/tags/Bad_Tag/feed.xml", nil))
	if rec.Code != 404 {
		t.Errorf("bad tag: %d", rec.Code)
	}
}

func TestAtomAndJSON(t *testing.T) {
	mux, _ := site(t, "staging")
	var a struct {
		Entries []struct {
			ID string `xml:"id"`
		} `xml:"entry"`
	}
	if err := xml.Unmarshal(get(t, mux, "/en/atom.xml").Body.Bytes(), &a); err != nil || len(a.Entries) != 2 {
		t.Fatalf("atom: %v %+v", err, a)
	}
	var j jsonFeedDoc
	rec := get(t, mux, "/en/feed.json")
	if err := json.Unmarshal(rec.Body.Bytes(), &j); err != nil {
		t.Fatal(err)
	}
	if j.Version != "https://jsonfeed.org/version/1.1" || len(j.Items) != 2 || j.Items[1].Tags[0] != "go" {
		t.Errorf("json feed = %+v", j)
	}
}

func TestSitemapAndRobots(t *testing.T) {
	mux, _ := site(t, "prod")
	sm := get(t, mux, "/sitemap.xml").Body.String()
	for _, want := range []string{
		"<loc>https://halid.org/en/posts/first</loc>",
		`hreflang="tr" href="https://halid.org/tr/posts/ilk"`,
		"<loc>https://halid.org/tr/posts</loc>",
		"<loc>https://halid.org/en/stream</loc>",
		"<loc>https://halid.org/en/stream/20", // the month archive of the published posts
	} {
		if !strings.Contains(sm, want) {
			t.Errorf("sitemap misses %s", want)
		}
	}
	if strings.Contains(sm, "draft") {
		t.Error("draft in sitemap")
	}
	if r := get(t, mux, "/robots.txt").Body.String(); !strings.Contains(r, "Sitemap: https://halid.org/sitemap.xml") {
		t.Errorf("prod robots: %s", r)
	}
	stg, _ := site(t, "staging")
	if r := get(t, stg, "/robots.txt").Body.String(); r != "User-agent: *\nDisallow: /\n" {
		t.Errorf("staging robots: %q", r)
	}
}

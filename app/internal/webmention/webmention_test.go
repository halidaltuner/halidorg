package webmention

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/fetch"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

const postURL = "https://halid.org/en/posts/htmx"

func base(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

func TestParseReply(t *testing.T) {
	page := `<!doctype html><html><body>
<div class="h-card"><a class="u-url p-name" href="https://blog.example/">Page Owner</a></div>
<article class="h-entry">
  <a class="u-in-reply-to" href="https://halid.org/en/posts/htmx/#why">re: htmx</a>
  <div class="p-author h-card"><img class="u-photo" src="/me.jpg" alt=""><a class="p-name u-url" href="https://ada.example">Ada</a></div>
  <div class="e-content">The 900 revisions   detail is <b>painfully</b> familiar.<script>alert(1)</script></div>
  <time class="dt-published" datetime="2026-10-16T09:12:00+03:00">Oct 16</time>
</article></body></html>`
	p := Parse([]byte(page), "text/html; charset=utf-8", base(t, "https://blog.example/p/1"), postURL)
	if !p.Linked || p.Type != Reply {
		t.Fatalf("linked %v type %q", p.Linked, p.Type)
	}
	if p.Author != "Ada" || p.AuthorURL != "https://ada.example" || p.Photo != "https://blog.example/me.jpg" {
		t.Errorf("author %q %q %q", p.Author, p.AuthorURL, p.Photo)
	}
	if !strings.HasPrefix(p.Content, "The 900 revisions detail is painfully familiar.") || strings.Contains(p.Content, "<") {
		t.Errorf("content %q", p.Content)
	}
	if p.Published == nil || *p.Published != 1792131120 {
		t.Errorf("published %v", p.Published)
	}
}

func TestParseKinds(t *testing.T) {
	like := `<html><body><div class="h-entry"><a class="u-like-of" href="http://halid.org/en/posts/htmx">♥</a></div></body></html>`
	if p := Parse([]byte(like), "text/html", base(t, "https://social.example/l/1"), postURL); !p.Linked || p.Type != Like || p.Author != "social.example" {
		t.Errorf("like: %+v", p)
	}
	repost := `<html><body><div class="h-feed"><div class="h-entry"><a class="u-repost-of" href="https://halid.org/en/posts/htmx">rt</a></div></div></body></html>`
	if p := Parse([]byte(repost), "text/html", base(t, "https://social.example/r/1"), postURL); p.Type != Repost {
		t.Errorf("repost in a feed: %+v", p)
	}
	// A plain link in someone's post: a mention, the page's h-card as author.
	plain := `<html><body><header class="h-card"><a class="p-name u-url" href="/">Bob</a></header>
<article class="h-entry"><p class="p-name">Weekly notes</p><div class="e-content">See <a href="https://halid.org/en/posts/htmx">this</a>.</div></article></body></html>`
	if p := Parse([]byte(plain), "text/html", base(t, "https://www.bob.example/w/42"), postURL); p.Type != Mention || p.Author != "Bob" || p.Content != "See this." {
		t.Errorf("mention: %+v", p)
	}
	// No link, or a link to another page: not a mention.
	other := `<html><body><a href="https://halid.org/en/posts/other">x</a></body></html>`
	if p := Parse([]byte(other), "text/html", base(t, "https://a.example/"), postURL); p.Linked {
		t.Error("link to another post counted")
	}
	// Plain text only has to contain the URL.
	if p := Parse([]byte("see "+postURL+" ok"), "text/plain", base(t, "https://a.example/t.txt"), postURL); !p.Linked || p.Author != "a.example" {
		t.Errorf("text: %+v", p)
	}
}

// fakeFetch serves pages by URL.
type fakeFetch map[string]*fetch.Response

func (f fakeFetch) Get(_ context.Context, raw string, _ int64) (*fetch.Response, error) {
	if res, ok := f[raw]; ok {
		if res.URL == nil {
			res.URL, _ = url.Parse(raw)
		}
		return res, nil
	}
	return nil, errors.New("connection refused")
}

func seed(t *testing.T, db store.DB) string {
	t.Helper()
	e := content.Entry{Kind: "post", Slug: "htmx", Lang: "en", Title: "Notes on htmx", Status: content.Draft, Visibility: content.Public}
	at := int64(100)
	e.PublishedAt = &at
	stmts := content.InsertEntry(&e, content.Body{MD: "x"}, content.OriginManual)
	stmts = append(stmts, store.Stmt{SQL: `INSERT INTO posts (entry_id) VALUES (?)`, Args: []any{e.ID}},
		store.Stmt{SQL: `UPDATE entries SET status = 'published', published_revision_id = current_revision_id WHERE id = ?`, Args: []any{e.ID}})
	if _, err := db.Batch(context.Background(), stmts...); err != nil {
		t.Fatal(err)
	}
	return e.ID
}

func rows(t *testing.T, db store.DB) []Row {
	t.Helper()
	res, err := db.Query(context.Background(), `SELECT `+cols+` FROM webmentions w ORDER BY w.source_url`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := store.Scan[Row](res)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func permanent(err error) bool {
	var p jobs.Permanent
	return errors.As(err, &p)
}

func TestReceive(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	blob := storetest.NewBlob()
	id := seed(t, db)
	png := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR")
	reply := `<html><body><article class="h-entry"><a class="u-in-reply-to" href="` + postURL + `">re</a>
<span class="p-author h-card"><img class="u-photo" src="https://ada.example/me.png"><span class="p-name">Ada</span></span>
<p class="e-content">Did you consider diffs?</p></article></body></html>`
	pages := fakeFetch{
		"https://ada.example/r/1":    {Status: 200, ContentType: "text/html", Body: []byte(reply)},
		"https://ada.example/me.png": {Status: 200, Body: png},
		"https://bob.example/like":   {Status: 200, ContentType: "text/html", Body: []byte(`<div class="h-entry"><a class="u-like-of" href="` + postURL + `"></a></div>`)},
	}
	r := NewReceiver(db, blob, pages).WithOrigin("https://halid.org")

	// A reply waits for moderation: nothing to re-render yet.
	paths, err := r.Receive(ctx, Received{Source: "https://ada.example/r/1", Target: postURL})
	if err != nil || len(paths) != 0 {
		t.Fatalf("reply: %v %v", paths, err)
	}
	// A like shows at once, on the post and its untranslated fallback.
	paths, err = r.Receive(ctx, Received{Source: "https://bob.example/like", Target: "https://halid.org/en/posts/htmx/"})
	if err != nil || strings.Join(paths, " ") != "/tr/posts/htmx /en/posts/htmx" {
		t.Fatalf("like: %v %v", paths, err)
	}
	got := rows(t, db)
	if len(got) != 2 || got[0].Type != Reply || got[0].Status != Pending || got[0].EntryID != id || got[1].Status != Approved {
		t.Fatalf("rows %+v", got)
	}
	if got[0].Author != "Ada" || got[0].Content != "Did you consider diffs?" || !strings.HasPrefix(got[0].Avatar, "/avatars/") || !strings.HasSuffix(got[0].Avatar, ".png") {
		t.Errorf("reply row %+v", got[0])
	}
	if _, ok := blob.Objects[strings.TrimPrefix(got[0].Avatar, "/")]; !ok {
		t.Error("avatar not stored in R2")
	}

	// Approved, then edited: back to pending, the page drops it.
	if _, err := NewRepo(db).Moderate(ctx, got[0].ID, Approved); err != nil {
		t.Fatal(err)
	}
	pages["https://ada.example/r/1"].Body = []byte(strings.Replace(reply, "Did you consider diffs?", "Buy pills", 1))
	paths, err = r.Receive(ctx, Received{Source: "https://ada.example/r/1", Target: postURL})
	if err != nil || len(paths) == 0 || rows(t, db)[0].Status != Pending {
		t.Fatalf("edited reply: %v %v %+v", paths, err, rows(t, db)[0])
	}
	// Rejected stays rejected when sent again.
	if _, err := NewRepo(db).Moderate(ctx, got[0].ID, Rejected); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Receive(ctx, Received{Source: "https://ada.example/r/1", Target: postURL}); err != nil || rows(t, db)[0].Status != Rejected {
		t.Fatalf("resent rejected: %v %+v", err, rows(t, db)[0])
	}

	// The like is deleted at the source: the row goes, the page re-renders.
	pages["https://bob.example/like"] = &fetch.Response{Status: 410}
	paths, err = r.Receive(ctx, Received{Source: "https://bob.example/like", Target: postURL})
	if err != nil || len(paths) != 2 || len(rows(t, db)) != 1 {
		t.Fatalf("deleted like: %v %v %d rows", paths, err, len(rows(t, db)))
	}
}

func TestReceiveRefuses(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	seed(t, db)
	pages := fakeFetch{
		"https://a.example/nolink": {Status: 200, ContentType: "text/html", Body: []byte(`<p>hello</p>`)},
		"https://a.example/500":    {Status: 503},
	}
	r := NewReceiver(db, nil, pages).WithOrigin("https://halid.org")
	for _, m := range []Received{
		{Source: "https://a.example/x", Target: "https://other.example/en/posts/htmx"}, // not this site
		{Source: "https://a.example/x", Target: "https://halid.org/en/posts/nope"},     // no such post
		{Source: "https://a.example/x", Target: "https://halid.org/en/links"},          // not an entry page
		{Source: "https://a.example/x", Target: "https://halid.org/en/tags/htmx"},      // not an entry page
		{Source: "http://10.0.0.1/x", Target: postURL},                                 // private source
		{Source: "https://halid.org/en/notes/n", Target: postURL},                      // this site
	} {
		if _, err := r.Receive(ctx, m); !permanent(err) {
			t.Errorf("%+v: %v", m, err)
		}
	}
	// No link: nothing stored, no error.
	if paths, err := r.Receive(ctx, Received{Source: "https://a.example/nolink", Target: postURL}); err != nil || paths != nil || len(rows(t, db)) != 0 {
		t.Errorf("no link: %v %v", paths, err)
	}
	// A source that is down is retried.
	for _, src := range []string{"https://a.example/500", "https://a.example/down"} {
		if _, err := r.Receive(ctx, Received{Source: src, Target: postURL}); err == nil || permanent(err) {
			t.Errorf("%s: %v", src, err)
		}
	}
}

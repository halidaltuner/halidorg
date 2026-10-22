package webmention

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"halid.org/engine/internal/fetch"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

func page(raw, ct, body string, header http.Header) *fetch.Response {
	u, _ := url.Parse(raw)
	if header == nil {
		header = http.Header{}
	}
	return &fetch.Response{URL: u, Status: 200, ContentType: ct, Header: header, Body: []byte(body)}
}

func TestDiscover(t *testing.T) {
	for _, c := range []struct {
		name string
		p    *fetch.Response
		want string
	}{
		{"header", page("https://a.example/p", "text/html", `<link rel="webmention" href="/html">`,
			http.Header{"Link": {`<https://a.example/other>; rel="other", </wm?x=1>; rel="nofollow webmention"`}}), "https://a.example/wm?x=1"},
		{"link", page("https://a.example/p/1", "text/html", `<html><head><link rel="stylesheet" href="s.css"><link rel="webmention" href="../wm"></head></html>`, nil), "https://a.example/wm"},
		{"a first", page("https://a.example/p", "text/html", `<body><a rel="webmention" href="https://wm.example/a">x</a><link rel="webmention" href="/later"></body>`, nil), "https://wm.example/a"},
		{"empty href", page("https://a.example/p?q=1", "text/html", `<link rel="webmention" href="">`, nil), "https://a.example/p?q=1"},
		{"no href", page("https://a.example/p", "text/html", `<link rel="webmention"><a rel="webmention" href="/ok">`, nil), "https://a.example/ok"},
		{"none", page("https://a.example/p", "text/html", `<a rel="me" href="/x">`, nil), ""},
		{"not html", page("https://a.example/p.txt", "text/plain", `<link rel="webmention" href="/wm">`, nil), ""},
	} {
		if got := Discover(c.p); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

func TestOutbound(t *testing.T) {
	body := `<p><a href="https://go.dev/blog#x">a</a> <a href="/en/posts/y">internal</a> <a href="https://halid.org/en/notes/z">self</a>
<a href="mailto:me@x.example">mail</a> <a href="https://go.dev/blog">again</a> <a href="http://b.example/">b</a> <a href="#fn1">fn</a></p>`
	got := Outbound(body, "https://halid.org/en/posts/htmx", "https://halid.org")
	if strings.Join(got, " ") != "https://go.dev/blog http://b.example/" {
		t.Errorf("outbound %q", got)
	}
}

// fakeWeb serves pages and records posts; endpoints answer with status.
type fakeWeb struct {
	pages  map[string]*fetch.Response
	status map[string]int
	posts  []string
}

func (f *fakeWeb) Get(ctx context.Context, raw string, max int64) (*fetch.Response, error) {
	return f.GetPrefix(ctx, raw, max)
}

func (f *fakeWeb) GetPrefix(_ context.Context, raw string, _ int64) (*fetch.Response, error) {
	if p, ok := f.pages[raw]; ok {
		return p, nil
	}
	if strings.Contains(raw, "gone.example") {
		return nil, &net.DNSError{Err: "no such host", Name: "gone.example", IsNotFound: true}
	}
	return nil, errors.New("connection refused")
}

func (f *fakeWeb) PostForm(_ context.Context, raw string, form url.Values, _ int64) (*fetch.Response, error) {
	f.posts = append(f.posts, raw+" "+form.Get("source")+" "+form.Get("target"))
	st, ok := f.status[raw]
	if !ok {
		return nil, errors.New("connection reset")
	}
	return &fetch.Response{Status: st, Body: []byte("bad target")}, nil
}

func publishRev(t *testing.T, db store.DB, id, revID, html string) {
	t.Helper()
	ctx := context.Background()
	if _, err := db.Batch(ctx,
		store.Stmt{SQL: `INSERT INTO revisions (id, entry_id, body_md, body_html, toc_json, data_json, origin, created_at) VALUES (?, ?, '', ?, '[]', '{}', 'publish', 1)`, Args: []any{revID, id, html}},
		store.Stmt{SQL: `UPDATE entries SET published_revision_id = ?, current_revision_id = ? WHERE id = ?`, Args: []any{revID, revID, id}}); err != nil {
		t.Fatal(err)
	}
}

func sends(t *testing.T, db store.DB) []Result {
	t.Helper()
	res, err := db.Query(context.Background(), `SELECT target_url, endpoint, status, error FROM webmention_sends ORDER BY target_url`)
	if err != nil {
		t.Fatal(err)
	}
	out, err := store.Scan[Result](res)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestSend(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	id := seed(t, db)
	src := "https://halid.org/en/posts/htmx"
	web := &fakeWeb{
		pages: map[string]*fetch.Response{
			"https://a.example/p":   page("https://a.example/p", "text/html", `<link rel="webmention" href="/wm">`, nil),
			"https://b.example/p":   page("https://b.example/p", "text/html", `<p>no endpoint</p>`, nil),
			"https://c.example/bad": page("https://c.example/bad", "text/html", `<link rel="webmention" href="https://c.example/wm">`, nil),
		},
		status: map[string]int{"https://a.example/wm": 202, "https://c.example/wm": 400},
	}
	s := NewSender(db, web).WithOrigin("https://halid.org")
	publishRev(t, db, id, "01REV0000000000000000000A1", `<a href="https://a.example/p">a</a> <a href="https://b.example/p">b</a> <a href="https://c.example/bad">c</a> <a href="https://gone.example/">dead</a> <a href="/en/notes/x">self</a>`)

	if err := s.Send(ctx, id); err != nil {
		t.Fatal(err)
	}
	if strings.Join(web.posts, "\n") != "https://a.example/wm "+src+" https://a.example/p\nhttps://c.example/wm "+src+" https://c.example/bad" {
		t.Fatalf("posts %q", web.posts)
	}
	got := sends(t, db)
	if len(got) != 4 || got[0].Status != 202 || got[1].Endpoint != "" || got[2].Status != 400 || got[2].Error != "bad target" || got[3].Target != "https://gone.example/" {
		t.Fatalf("sends %+v", got)
	}
	// Same revision again: nothing is sent.
	web.posts = nil
	if err := s.Send(ctx, id); err != nil || len(web.posts) != 0 {
		t.Fatalf("resend: %v %q", err, web.posts)
	}

	// A new revision drops a link: it is told once, then forgotten.
	publishRev(t, db, id, "01REV0000000000000000000A2", `<a href="https://b.example/p">b</a>`)
	if err := s.Send(ctx, id); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(web.posts, "\n"), "https://a.example/wm "+src+" https://a.example/p") {
		t.Errorf("dropped link not told: %q", web.posts)
	}
	if got := sends(t, db); len(got) != 1 || got[0].Target != "https://b.example/p" {
		t.Errorf("after drop %+v", got)
	}

	// An endpoint that is down fails the job and saves nothing, so it is retried.
	publishRev(t, db, id, "01REV0000000000000000000A3", `<a href="https://a.example/p">a</a>`)
	web.status["https://a.example/wm"] = 503
	if err := s.Send(ctx, id); err == nil || !strings.Contains(err.Error(), "https://a.example/p") {
		t.Fatalf("503: %v", err)
	}
	if got := sends(t, db); len(got) != 1 || got[0].Target != "https://b.example/p" {
		t.Errorf("state saved after failure: %+v", got)
	}
	web.status["https://a.example/wm"] = 201
	if err := s.Send(ctx, id); err != nil {
		t.Fatal(err)
	}

	// Unpublished: every last target is told, then the state goes.
	web.posts = nil
	if _, err := db.Query(ctx, `UPDATE entries SET status = 'draft', published_revision_id = NULL WHERE id = ?`, id); err != nil {
		t.Fatal(err)
	}
	if err := s.Send(ctx, id); err != nil {
		t.Fatal(err)
	}
	if strings.Join(web.posts, "\n") != "https://a.example/wm "+src+" https://a.example/p" {
		t.Errorf("unpublish told %q", web.posts)
	}
	res, _ := db.Query(ctx, `SELECT COUNT(*) AS n FROM webmention_sources`)
	n, _ := store.Scan[struct {
		N int `json:"n"`
	}](res)
	if n[0].N != 0 || len(sends(t, db)) != 0 {
		t.Error("state kept after unpublish")
	}
}

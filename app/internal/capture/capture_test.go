package capture

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/fav"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/fetch"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

type fakeFetch struct{ calls []string }

func (f *fakeFetch) Meta(_ context.Context, u string) (fetch.Meta, error) {
	f.calls = append(f.calls, u)
	switch {
	case strings.Contains(u, "internal"):
		return fetch.Meta{}, fetch.ErrBlocked
	case strings.Contains(u, "down"):
		return fetch.Meta{}, errors.New("timeout")
	}
	return fetch.Meta{URL: u, Title: "Hypermedia: A Reintroduction", Description: "Chapter 1 of Hypermedia Systems."}, nil
}

type site struct {
	mux          *http.ServeMux
	s            store.Stores
	f            *fakeFetch
	fresh, stale *http.Cookie
}

func newSite(t *testing.T) site {
	t.Helper()
	t.Setenv("APP_ENV", "staging")
	t.Setenv("PUBLIC_ORIGIN", "https://staging.halid.org")
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	f := &fakeFetch{}
	h, err := New(s.DB, admin.NewUI(s.DB), a, jobs.New(s.DB, s.Queue), f)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	fresh, _ := a.TestSession(context.Background(), "me@halid.org", time.Minute)
	stale, _ := a.TestSession(context.Background(), "me@halid.org", time.Hour)
	return site{mux: mux, s: s, f: f, fresh: fresh, stale: stale}
}

func (st site) do(c *http.Cookie, method, target string, form url.Values, siteHdr string) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Sec-Fetch-Site", siteHdr)
	req.Header.Set("Accept-Language", "en")
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	st.mux.ServeHTTP(rec, req)
	return rec
}

func must(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q", w)
		}
	}
}

func queued(st site) int { return len(st.s.Queue.(*storetest.Queue).Sent) }

func TestFormAndFetch(t *testing.T) {
	st := newSite(t)
	if rec := st.do(nil, "GET", "/admin/quick", nil, "none"); rec.Code != http.StatusSeeOther {
		t.Fatalf("signed out: %d", rec.Code)
	}
	body := st.do(st.fresh, "GET", "/admin/quick", nil, "none").Body.String()
	must(t, body, `<form class="qc" method="post" action="/admin/quick">`, `formmethod="get" formaction="/admin/quick"`,
		`name="action" value="draft" disabled`, `<span class="adm-env">staging</span>`,
		// Percent-encoded by html/template; browsers decode javascript: URLs before running them.
		`href="javascript:%28%28%29=%3e%7bwindow.open%28%27https://staging.halid.org/admin/quick?url=%27&#43;encodeURIComponent%28location.href%29`,
		`class="bm-chip mono"`, `data-bookmarklet`, `Drag this to your bookmarks bar`)
	if strings.Contains(body, "ZgotmplZ") || strings.Contains(body, `id="qc-title"`) {
		t.Error("bookmarklet sanitized away or fields shown before a fetch")
	}

	// The fetch button (and the bookmarklet) are a GET with the URL.
	body = st.do(st.fresh, "GET", "/admin/quick?url=hypermedia.systems/x&kind=fav&lang=tr", nil, "none").Body.String()
	must(t, body, `value="https://hypermedia.systems/x"`, `value="Hypermedia: A Reintroduction"`, `Chapter 1 of Hypermedia Systems.`,
		`<p class="qc-domain mono">hypermedia.systems</p>`, `id="qc-kind-fav" value="fav" checked`, `id="qc-lang-tr" value="tr" checked`)
	if strings.Contains(body, `value="draft" disabled`) {
		t.Error("buttons stay disabled after a fetch")
	}
	// A tab title from the bookmarklet is kept.
	body = st.do(st.fresh, "GET", "/admin/quick?url=https://a.dev/&title=Tab+title", nil, "none").Body.String()
	must(t, body, `value="Tab title"`)
	// Failures keep the fields open; blocked hosts say why.
	must(t, st.do(st.fresh, "GET", "/admin/quick?url=https://down.dev/", nil, "none").Body.String(), "Could not read the page", `id="qc-title"`)
	must(t, st.do(st.fresh, "GET", "/admin/quick?url=http://bridge.internal/kv", nil, "none").Body.String(), "public http(s) addresses only")
	// An empty URL opens a note.
	must(t, st.do(st.fresh, "GET", "/admin/quick?url=", nil, "none").Body.String(), `id="qc-kind-note" value="note" checked`)
}

func form(kind, u, title, comment, action string) url.Values {
	return url.Values{"kind": {kind}, "url": {u}, "title": {title}, "comment": {comment}, "lang": {"en"}, "action": {action}}
}

func TestSaveKinds(t *testing.T) {
	st := newSite(t)
	ctx := context.Background()

	// Draft link.
	rec := st.do(st.fresh, "POST", "/admin/quick", form("link", "https://hypermedia.systems/x", "Hypermedia: A Reintroduction", "The one to reread.", "draft"), "same-origin")
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/admin/quick/saved/") {
		t.Fatalf("draft %d %q %s", rec.Code, rec.Header().Get("Location"), rec.Body.String())
	}
	id := strings.TrimPrefix(rec.Header().Get("Location"), "/admin/quick/saved/")
	l, err := link.NewRepo(st.s.DB).Get(ctx, id)
	if err != nil || l.Status != content.Draft || l.Slug != "hypermedia-a-reintroduction" || l.Summary != "The one to reread." || l.Domain != "hypermedia.systems" {
		t.Fatalf("link %+v %v", l, err)
	}
	must(t, st.do(st.fresh, "GET", rec.Header().Get("Location"), nil, "none").Body.String(), "saved as link", "draft · en · hypermedia.systems", `href="/admin/quick"`, `<a class="qc-view mono" href="/admin/links/`+id+`">edit →</a>`)
	if queued(st) != 1 {
		t.Errorf("jobs %d", queued(st))
	}

	// The same title again gets the next slug; publish with a fresh sign-in.
	rec = st.do(st.fresh, "POST", "/admin/quick", form("link", "https://hypermedia.systems/y", "Hypermedia: A Reintroduction", "", "publish"), "same-origin")
	id2 := strings.TrimPrefix(rec.Header().Get("Location"), "/admin/quick/saved/")
	l2, _ := link.NewRepo(st.s.DB).Get(ctx, id2)
	if l2.Slug != "hypermedia-a-reintroduction-2" || l2.Status != content.Published {
		t.Fatalf("second link %+v", l2.Entry)
	}
	must(t, st.do(st.fresh, "GET", rec.Header().Get("Location"), nil, "none").Body.String(), "published · en", `href="/en/links"`)
	if queued(st) != 3 {
		t.Errorf("jobs after publish %d", queued(st))
	}

	// A fav from GitHub is a repo; a note may have no link.
	rec = st.do(st.fresh, "POST", "/admin/quick", form("fav", "https://github.com/sqlc-dev/sqlc", "sqlc", "Typed SQL.", "draft"), "same-origin")
	f, err := fav.NewRepo(st.s.DB).Get(ctx, strings.TrimPrefix(rec.Header().Get("Location"), "/admin/quick/saved/"))
	if err != nil || f.Type != "repo" || f.Summary != "Typed SQL." {
		t.Fatalf("fav %+v %v", f, err)
	}
	rec = st.do(st.fresh, "POST", "/admin/quick", form("note", "", "", "TIL: go test -run takes a regexp.", "publish"), "same-origin")
	n, err := note.NewRepo(st.s.DB).Get(ctx, strings.TrimPrefix(rec.Header().Get("Location"), "/admin/quick/saved/"))
	if err != nil || n.Status != content.Published || !strings.HasPrefix(n.Slug, "note-") || n.SourceURL != "" {
		t.Fatalf("note %+v %v", n, err)
	}
}

func TestPublishNeedsRecentSignIn(t *testing.T) {
	st := newSite(t)
	rec := st.do(st.stale, "POST", "/admin/quick", form("link", "https://grugbrain.dev/", "Grug", "", "publish"), "same-origin")
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(loc, "/admin/reauth?next=") {
		t.Fatalf("stale publish %d %q", rec.Code, loc)
	}
	next, _ := url.QueryUnescape(strings.TrimPrefix(loc, "/admin/reauth?next="))
	id := strings.TrimSuffix(strings.TrimPrefix(next, "/admin/quick/saved/"), "?publish=1")
	l, err := link.NewRepo(st.s.DB).Get(context.Background(), id)
	if err != nil || l.Status != content.Draft {
		t.Fatalf("the draft must be saved before re-auth: %+v %v", l.Entry, err)
	}
	// Back from re-auth: the saved screen offers publishing.
	must(t, st.do(st.fresh, "GET", next, nil, "none").Body.String(), `action="/admin/quick/`+id+`/publish"`, "publish now")
	if rec := st.do(st.stale, "POST", "/admin/quick/"+id+"/publish", url.Values{}, "same-origin"); !strings.HasPrefix(rec.Header().Get("Location"), "/admin/reauth") {
		t.Errorf("stale publish button: %q", rec.Header().Get("Location"))
	}
	if rec := st.do(st.fresh, "POST", "/admin/quick/"+id+"/publish", url.Values{}, "same-origin"); rec.Code != http.StatusSeeOther {
		t.Fatalf("publish %d", rec.Code)
	}
	if l, _ := link.NewRepo(st.s.DB).Get(context.Background(), id); l.Status != content.Published {
		t.Error("not published")
	}
}

func TestSaveRefusals(t *testing.T) {
	st := newSite(t)
	for name, c := range map[string]struct {
		form url.Values
		want string
	}{
		"http url":   {form("link", "http://example.com/", "x", "", "draft"), "must start with https://"},
		"no title":   {form("fav", "https://example.com/", "", "", "draft"), "A title is needed."},
		"empty note": {form("note", "", "", "", "draft"), "A note cannot be empty."},
	} {
		rec := st.do(st.fresh, "POST", "/admin/quick", c.form, "same-origin")
		if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), c.want) {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if rec := st.do(st.fresh, "POST", "/admin/quick", form("link", "https://example.com/", "x", "", "draft"), "cross-site"); rec.Code == http.StatusSeeOther {
		t.Error("cross-site save accepted")
	}
	if queued(st) != 0 {
		t.Errorf("refused saves queued %d jobs", queued(st))
	}
}

func TestDuplicateWarning(t *testing.T) {
	st := newSite(t)
	rec := st.do(st.fresh, "POST", "/admin/quick", form("link", "https://Hypermedia.systems/x/", "Hypermedia: A Reintroduction", "", "draft"), "same-origin")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("first save %d", rec.Code)
	}
	id := strings.TrimPrefix(rec.Header().Get("Location"), "/admin/quick/saved/")

	// Fetching the same address, spelled differently, warns and carries dup_ok.
	body := st.do(st.fresh, "GET", "/admin/quick?url=https://hypermedia.systems/x%23ch1", nil, "none").Body.String()
	must(t, body, `This address is already saved:`, `<a href="/admin/links/`+id+`">link: Hypermedia: A Reintroduction</a>`,
		`· draft`, `name="dup_ok" value="https://hypermedia.systems/x#ch1"`)
	if strings.Contains(st.do(st.fresh, "GET", "/admin/quick?url=https://hypermedia.systems/y", nil, "none").Body.String(), "already saved") {
		t.Error("warning for a new address")
	}

	// Saving without having seen it warns first and saves nothing.
	f := form("fav", "https://hypermedia.systems/x", "Hypermedia", "", "draft")
	rec = st.do(st.fresh, "POST", "/admin/quick", f, "same-origin")
	if rec.Code != http.StatusConflict || queued(st) != 1 {
		t.Fatalf("unwarned save %d, jobs %d", rec.Code, queued(st))
	}
	must(t, rec.Body.String(), `This address is already saved:`, `value="Hypermedia"`)
	// dup_ok for another address does not count.
	f.Set("dup_ok", "https://hypermedia.systems/z")
	if rec = st.do(st.fresh, "POST", "/admin/quick", f, "same-origin"); rec.Code != http.StatusConflict {
		t.Fatalf("stale dup_ok %d", rec.Code)
	}
	// The second press saves.
	f.Set("dup_ok", "https://hypermedia.systems/x")
	if rec = st.do(st.fresh, "POST", "/admin/quick", f, "same-origin"); rec.Code != http.StatusSeeOther {
		t.Fatalf("acknowledged save %d %s", rec.Code, rec.Body.String())
	}
}

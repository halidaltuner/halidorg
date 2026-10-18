package shortadmin_test

import (
	"context"
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
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

type site struct {
	mux          *http.ServeMux
	s            store.Stores
	fresh, stale *http.Cookie
}

func newSite(t *testing.T) site {
	t.Helper()
	t.Setenv("APP_ENV", "staging")
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	u := admin.NewUI(s.DB)
	j := jobs.New(s.DB, s.Queue)
	mux := http.NewServeMux()
	for _, f := range []func(store.DB, *admin.UI, *auth.Auth, *jobs.Jobs) (*shortadmin.Admin, error){
		note.NewEditor, snippet.NewEditor, link.NewEditor, fav.NewEditor,
	} {
		e, err := f(s.DB, u, a, j)
		if err != nil {
			t.Fatal(err)
		}
		e.Register(mux)
	}
	fresh, _ := a.TestSession(context.Background(), "me@halid.org", time.Minute)
	stale, _ := a.TestSession(context.Background(), "me@halid.org", time.Hour)
	return site{mux: mux, s: s, fresh: fresh, stale: stale}
}

func (st site) do(c *http.Cookie, method, target string, form url.Values) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Accept-Language", "en")
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	st.mux.ServeHTTP(rec, req)
	return rec
}

func (st site) queued() int { return len(st.s.Queue.(*storetest.Queue).Sent) }

func must(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q", w)
		}
	}
}

func idOf(t *testing.T, rec *httptest.ResponseRecorder, base string) string {
	t.Helper()
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(loc, base+"/") {
		t.Fatalf("want redirect into %s, got %d %q %s", base, rec.Code, loc, rec.Body.String())
	}
	return strings.SplitN(strings.TrimPrefix(loc, base+"/"), "?", 2)[0]
}

func TestSnippetEditor(t *testing.T) {
	st := newSite(t)
	ctx := context.Background()
	if rec := st.do(nil, "GET", "/admin/snippets", nil); rec.Code != http.StatusSeeOther {
		t.Fatalf("signed out: %d", rec.Code)
	}
	body := st.do(st.fresh, "GET", "/admin/snippets/new", nil).Body.String()
	must(t, body, `<form class="adm-form ent-form" method="post" action="/admin/snippets">`, `name="file_name"`, `name="lang" value="en" checked`)

	// Create with two files and an empty row; the slug comes from the title.
	f := url.Values{"title": {"go: graceful shutdown"}, "lang": {"en"}, "markdown": {"Stop on **SIGTERM**."}, "tags": {"Go, ops"},
		"file_name": {"main.go", "run.sh", ""}, "file_lang": {"", "", ""}, "file_order": {"0", "1", "2"},
		"file_code": {"package main\n", "go run .\n", ""}, "action": {"save"}}
	id := idOf(t, st.do(st.fresh, "POST", "/admin/snippets", f), "/admin/snippets")
	s, err := snippet.NewRepo(st.s.DB).Get(ctx, id)
	if err != nil || s.Slug != "go-graceful-shutdown" || s.Status != content.Draft || len(s.Files) != 2 || s.Files[0].Language != "go" || s.Files[1].Language != "bash" {
		t.Fatalf("created %+v %v", s, err)
	}
	if st.queued() != 1 {
		t.Errorf("jobs %d", st.queued())
	}
	body = st.do(st.fresh, "GET", "/admin/snippets/"+id+"?saved=1", nil).Body.String()
	must(t, body, `value="main.go"`, `value="run.sh"`, `>package main
</textarea>`, `value="go, ops"`, `Saved.`, `<span class="adm-st s-draft">draft</span>`)
	if strings.Count(body, `name="file_name"`) != 3 {
		t.Error("want the two files and one empty row")
	}

	// Reorder and drop a file: run.sh first, main.go emptied.
	f = url.Values{"title": {"go: graceful shutdown"}, "slug": {"go-graceful-shutdown"}, "markdown": {"Stop on SIGTERM."}, "tags": {"go"},
		"file_name": {"", "run.sh", ""}, "file_lang": {"go", "", ""}, "file_order": {"0", "1", "2"}, "file_code": {"", "go run .\n", ""}}
	idOf(t, st.do(st.fresh, "POST", "/admin/snippets/"+id, f), "/admin/snippets")
	s, _ = snippet.NewRepo(st.s.DB).Get(ctx, id)
	if len(s.Files) != 1 || s.Files[0].Filename != "run.sh" {
		t.Fatalf("files after save %+v", s.Files)
	}

	// Publishing with a stale sign-in saves, then goes through re-auth.
	f.Set("title", "go: graceful shutdown with context")
	rec := st.do(st.stale, "POST", "/admin/snippets/"+id+"/publish", f)
	if loc := rec.Header().Get("Location"); rec.Code != http.StatusSeeOther || !strings.HasPrefix(loc, "/admin/reauth?next=") {
		t.Fatalf("stale publish %d %q", rec.Code, loc)
	}
	s, _ = snippet.NewRepo(st.s.DB).Get(ctx, id)
	if s.Status != content.Draft || s.Title != "go: graceful shutdown with context" {
		t.Fatalf("stale publish lost the text or published: %+v", s.Entry)
	}
	must(t, st.do(st.fresh, "GET", "/admin/snippets/"+id+"?publish=1", nil).Body.String(), `name="now" value="1"`, `publish now`)
	rec = st.do(st.fresh, "POST", "/admin/snippets/"+id+"/publish", url.Values{"now": {"1"}})
	if rec.Code != http.StatusSeeOther || !strings.HasSuffix(rec.Header().Get("Location"), "?published=1") {
		t.Fatalf("publish now %d %q", rec.Code, rec.Header().Get("Location"))
	}
	s, _ = snippet.NewRepo(st.s.DB).Get(ctx, id)
	if s.Status != content.Published {
		t.Fatalf("not published: %s", s.Status)
	}
	body = st.do(st.fresh, "GET", "/admin/snippets/"+id, nil).Body.String()
	must(t, body, `<a class="mono ent-view" href="/en/snippets/go-graceful-shutdown">`, `update live`)

	// The list.
	must(t, st.do(st.fresh, "GET", "/admin/snippets", nil).Body.String(), `<a href="/admin/snippets/`+id+`"><span class="ent-title">go: graceful shutdown with context</span>`)

	// Delete needs a recent sign-in, then leaves a 410 at the public address.
	if rec := st.do(st.stale, "POST", "/admin/snippets/"+id+"/delete", url.Values{}); rec.Code == http.StatusSeeOther && strings.HasPrefix(rec.Header().Get("Location"), "/admin/snippets?") {
		t.Fatal("stale delete went through")
	}
	rec = st.do(st.fresh, "POST", "/admin/snippets/"+id+"/delete", url.Values{})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/snippets?deleted=go-graceful-shutdown" {
		t.Fatalf("delete %d %q", rec.Code, rec.Header().Get("Location"))
	}
	res, _ := st.s.DB.Query(ctx, `SELECT code FROM redirects WHERE from_path = '/en/snippets/go-graceful-shutdown'`)
	if rows, _ := store.Scan[struct {
		Code int `json:"code"`
	}](res); len(rows) != 1 || rows[0].Code != 410 {
		t.Errorf("tombstone %+v", rows)
	}
	must(t, st.do(st.fresh, "GET", "/admin/snippets?deleted=go-graceful-shutdown", nil).Body.String(), `go-graceful-shutdown was deleted.`, `Nothing here yet.`)
}

func TestValidationAndCSRF(t *testing.T) {
	st := newSite(t)
	// A snippet cannot publish without a file; the typed values stay.
	f := url.Values{"title": {"empty"}, "lang": {"en"}, "markdown": {"no files"}, "action": {"save"}}
	id := idOf(t, st.do(st.fresh, "POST", "/admin/snippets", f), "/admin/snippets")
	f.Set("title", "still empty")
	rec := st.do(st.fresh, "POST", "/admin/snippets/"+id+"/publish", f)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("publish without files %d", rec.Code)
	}
	must(t, rec.Body.String(), `A snippet needs at least one file to publish.`, `value="still empty"`)

	// A bad link address is refused on create.
	rec = st.do(st.fresh, "POST", "/admin/links", url.Values{"title": {"x"}, "lang": {"en"}, "url": {"http://plain.example"}})
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad url %d", rec.Code)
	}
	must(t, rec.Body.String(), `The address must start with https://.`, `value="http://plain.example"`)

	// Cross-site posts are refused (admin.Guard).
	req := httptest.NewRequest("POST", "/admin/notes", strings.NewReader("title=x&lang=en"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	req.AddCookie(st.fresh)
	w := httptest.NewRecorder()
	st.mux.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("cross-site %d", w.Code)
	}
}

func TestLinkNoteFavEditors(t *testing.T) {
	st := newSite(t)
	ctx := context.Background()

	// Link: create, then change the address (the icon is fetched again) and mark it read.
	lid := idOf(t, st.do(st.fresh, "POST", "/admin/links", url.Values{"title": {"Grug"}, "lang": {"en"}, "url": {"https://grugbrain.dev/"},
		"summary": {"Complexity is the enemy."}, "action": {"save"}}), "/admin/links")
	if _, err := st.s.DB.Query(ctx, `UPDATE links SET favicon_url = '/favicons/grugbrain.dev.ico' WHERE entry_id = ?`, lid); err != nil {
		t.Fatal(err)
	}
	must(t, st.do(st.fresh, "GET", "/admin/links/"+lid, nil).Body.String(), `value="https://grugbrain.dev/"`, `>Complexity is the enemy.</textarea>`,
		`name="read_status" value="unread" checked`)
	idOf(t, st.do(st.fresh, "POST", "/admin/links/"+lid, url.Values{"title": {"Grug"}, "slug": {"grug"}, "url": {"https://grugbrain.dev/x"},
		"summary": {"Still true."}, "read_status": {"read"}}), "/admin/links")
	l, _ := link.NewRepo(st.s.DB).Get(ctx, lid)
	if l.URL != "https://grugbrain.dev/x" || l.FaviconURL != "" || l.ReadStatus != link.Read || l.Summary != "Still true." {
		t.Fatalf("link %+v", l)
	}

	// Note: untitled, the slug and the list title come from the text.
	nid := idOf(t, st.do(st.fresh, "POST", "/admin/notes", url.Values{"lang": {"tr"}, "markdown": {"Go 1.22 loop variables\n\nEach iteration."},
		"til": {"1"}, "action": {"save"}}), "/admin/notes")
	n, _ := note.NewRepo(st.s.DB).Get(ctx, nid)
	if n.Slug != "go-1-22-loop-variables" || !n.TIL || n.Lang != "tr" {
		t.Fatalf("note %+v", n)
	}
	must(t, st.do(st.fresh, "GET", "/admin/notes", nil).Body.String(), `<span class="ent-title">Go 1.22 loop variables</span>`)

	// Fav: type, rating, then the rating cleared.
	fid := idOf(t, st.do(st.fresh, "POST", "/admin/favs", url.Values{"title": {"Helix"}, "lang": {"en"}, "fav_type": {"tool"}, "rating": {"4"},
		"author": {"modal editor"}, "action": {"save"}}), "/admin/favs")
	fv, _ := fav.NewRepo(st.s.DB).Get(ctx, fid)
	if fv.Type != "tool" || fv.Rating == nil || *fv.Rating != 4 || fv.Author != "modal editor" {
		t.Fatalf("fav %+v", fv)
	}
	must(t, st.do(st.fresh, "GET", "/admin/favs/"+fid, nil).Body.String(), `name="rating" value="4" checked`, `name="fav_type" value="tool" checked`)
	idOf(t, st.do(st.fresh, "POST", "/admin/favs/"+fid, url.Values{"title": {"Helix"}, "slug": {"helix"}, "fav_type": {"tool"}, "rating": {""}}), "/admin/favs")
	if fv, _ = fav.NewRepo(st.s.DB).Get(ctx, fid); fv.Rating != nil {
		t.Errorf("rating kept %v", *fv.Rating)
	}
	// A link id under /admin/favs is not found.
	if rec := st.do(st.fresh, "GET", "/admin/favs/"+lid, nil); rec.Code != http.StatusNotFound {
		t.Errorf("wrong kind %d", rec.Code)
	}
	// Publishing a link from the new form, signed in recently.
	rec := st.do(st.fresh, "POST", "/admin/links", url.Values{"title": {"htmx"}, "lang": {"en"}, "url": {"https://htmx.org/"}, "action": {"publish"}})
	pid := idOf(t, rec, "/admin/links")
	if l, _ := link.NewRepo(st.s.DB).Get(ctx, pid); l.Status != content.Published {
		t.Errorf("new + publish: %s", l.Status)
	}
}

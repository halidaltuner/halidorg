package post

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
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/media"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

type site struct {
	mux   *http.ServeMux
	s     store.Stores
	auth  *auth.Auth
	fresh *http.Cookie // signed in a minute ago
	stale *http.Cookie // signed in an hour ago: publish and delete need re-auth
}

func newAdminSite(t *testing.T) site {
	t.Helper()
	t.Setenv("APP_ENV", "staging")
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	h, err := NewAdmin(s.DB, admin.NewUI(s.DB), a, jobs.New(s.DB, s.Queue), media.NewRepo(s.DB, s.Blob))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	fresh, err := a.TestSession(context.Background(), "me@halid.org", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	stale, err := a.TestSession(context.Background(), "me@halid.org", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return site{mux: mux, s: s, auth: a, fresh: fresh, stale: stale}
}

func (st site) do(c *http.Cookie, method, target string, form url.Values, htmx bool) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	req.Header.Set("Accept-Language", "en")
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	if c != nil {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	st.mux.ServeHTTP(rec, req)
	return rec
}

func (st site) newPost(t *testing.T, title string) string {
	t.Helper()
	rec := st.do(st.fresh, "POST", "/admin/posts", url.Values{"title": {title}, "lang": {"en"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	return strings.TrimPrefix(rec.Header().Get("Location"), "/admin/posts/")
}

func editorForm(title, slug, md string) url.Values {
	return url.Values{"title": {title}, "slug": {slug}, "markdown": {md}, "summary": {"Short."}, "tags": {"go, #htmx, Bad Tag!"}, "lang": {"en"}}
}

func must(t *testing.T, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("missing %q", w)
		}
	}
}

func TestEditorFlowWithoutJS(t *testing.T) {
	st := newAdminSite(t)
	id := st.newPost(t, "Ghost'tan ayrılmak")
	repo := NewRepo(st.s.DB)
	p, err := repo.Get(context.Background(), id)
	if err != nil || p.Slug != "ghosttan-ayrilmak" || p.Status != content.Draft {
		t.Fatalf("created %+v %v", p, err)
	}

	// The editor renders every panel from the prototype.
	rec := st.do(st.fresh, "GET", "/admin/posts/"+id, nil, false)
	must(t, rec.Body.String(),
		`<form id="editor"`, `action="/admin/posts/`+id+`"`, `name="markdown"`, `formaction="/admin/posts/`+id+`/publish"`,
		`seo description · <span id="ed-seo-n">0</span>/160`, `create tr draft from this →`, `delete entry`,
		`/static/vendor/htmx.min.js`, `/static/js/editor.js`, `<span class="adm-env">staging</span>`,
	)

	// A plain save: 303 back, text, tags and a manual revision stored.
	rec = st.do(st.fresh, "POST", "/admin/posts/"+id, editorForm("Leaving Ghost", "leaving-ghost", "## Why\n\nBecause."), false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("save %d %s", rec.Code, rec.Body.String())
	}
	p, _ = repo.Get(context.Background(), id)
	tags, _ := content.TagsOf(context.Background(), st.s.DB, id)
	if p.Title != "Leaving Ghost" || p.Slug != "leaving-ghost" || p.Summary != "Short." || len(tags) != 2 {
		t.Fatalf("after save %+v tags %+v", p.Entry, tags)
	}

	// Validation: the editor comes back with the typed text and the error.
	rec = st.do(st.fresh, "POST", "/admin/posts/"+id, editorForm("Leaving Ghost", "Bad Slug", "kept text"), false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad slug %d", rec.Code)
	}
	must(t, rec.Body.String(), "Slug: lowercase letters", ">kept text</textarea>", "not saved")

	// Publish with a fresh sign-in.
	rec = st.do(st.fresh, "POST", "/admin/posts/"+id+"/publish", editorForm("Leaving Ghost", "leaving-ghost", "## Why\n\nBecause, published."), false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/posts/"+id {
		t.Fatalf("publish %d %q", rec.Code, rec.Header().Get("Location"))
	}
	p, _ = repo.Get(context.Background(), id)
	if p.Status != content.Published || !samePtr(p.CurrentRevisionID, p.PublishedRevisionID) {
		t.Fatalf("not published: %+v", p.Entry)
	}
	if n := len(st.s.Queue.(*storetest.Queue).Sent); n < 3 {
		t.Errorf("render jobs queued: %d", n)
	}
}

func TestAutosaveCoalescesAndReturnsFragments(t *testing.T) {
	st := newAdminSite(t)
	id := st.newPost(t, "Autosave")
	for _, md := range []string{"a", "ab", "abc"} {
		f := editorForm("Autosave", "autosave", md)
		f.Set("origin", "autosave")
		rec := st.do(st.fresh, "POST", "/admin/posts/"+id, f, true)
		if rec.Code != 200 {
			t.Fatalf("autosave %d", rec.Code)
		}
		body := rec.Body.String()
		must(t, body, `id="save-state"`, "autosaved ", `id="ed-revs" hx-swap-oob="true"`, `id="ed-title" hx-swap-oob="true">Autosave<`)
		if strings.Contains(body, "<html") {
			t.Fatal("htmx save returned a full page")
		}
	}
	revs, _ := content.Revisions(context.Background(), st.s.DB, id, 10)
	if len(revs) != 2 || revs[0].Origin != content.OriginAutosave {
		t.Fatalf("revisions %+v, want create + one coalesced autosave", revs)
	}
	// ⌘S always adds one.
	f := editorForm("Autosave", "autosave", "abcd")
	f.Set("origin", "manual")
	st.do(st.fresh, "POST", "/admin/posts/"+id, f, true)
	if revs, _ = content.Revisions(context.Background(), st.s.DB, id, 10); len(revs) != 3 || revs[0].Origin != content.OriginManual {
		t.Fatalf("after manual save %+v", revs)
	}
	// htmx validation errors swap in (200) instead of failing silently.
	rec := st.do(st.fresh, "POST", "/admin/posts/"+id, editorForm("Autosave", "", "x"), true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `class="adm-error"`) {
		t.Fatalf("htmx error %d %s", rec.Code, rec.Body.String())
	}
}

// TestPublishReauthKeepsText: a stale sign-in saves the text, then goes
// through re-auth and back to the editor, which offers to finish.
func TestPublishReauthKeepsText(t *testing.T) {
	st := newAdminSite(t)
	id := st.newPost(t, "Reauth")
	rec := st.do(st.stale, "POST", "/admin/posts/"+id+"/publish", editorForm("Reauth", "reauth", "typed just now"), true)
	want := auth.ReauthPath("/admin/posts/" + id + "?publish=1")
	if rec.Code != 200 || rec.Header().Get("HX-Redirect") != want {
		t.Fatalf("stale publish %d %q, want HX-Redirect %q", rec.Code, rec.Header().Get("HX-Redirect"), want)
	}
	p, _ := NewRepo(st.s.DB).Get(context.Background(), id)
	if p.Status != content.Draft {
		t.Fatal("published without a recent sign-in")
	}
	rev, _ := content.GetRevision(context.Background(), st.s.DB, *p.CurrentRevisionID)
	if rev.BodyMD != "typed just now" {
		t.Fatalf("text lost on the way to re-auth: %q", rev.BodyMD)
	}
	// Back from re-auth (fresh session): the banner, then "publish now".
	body := st.do(st.fresh, "GET", "/admin/posts/"+id+"?publish=1", nil, false).Body.String()
	must(t, body, "Signed in again", `name="now" value="1"`, "typed just now")
	rec = st.do(st.fresh, "POST", "/admin/posts/"+id+"/publish", url.Values{"now": {"1"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("publish now %d", rec.Code)
	}
	if p, _ = NewRepo(st.s.DB).Get(context.Background(), id); p.Status != content.Published {
		t.Fatal("publish now did not publish")
	}
	if rev, _ = content.GetRevision(context.Background(), st.s.DB, *p.PublishedRevisionID); rev.BodyMD != "typed just now" {
		t.Fatalf("published text %q", rev.BodyMD)
	}
}

func TestDeleteWithBacklinksNeedsTypedSlug(t *testing.T) {
	st := newAdminSite(t)
	ctx := context.Background()
	target := st.newPost(t, "Target")
	st.do(st.fresh, "POST", "/admin/posts/"+target+"/publish", editorForm("Target", "target", "Body."), false)
	source := st.newPost(t, "Source")
	if _, err := st.s.DB.Batch(ctx, content.SetLinksStmts(source, []content.Link{{Lang: "en", Section: "posts", Slug: "target"}})...); err != nil {
		t.Fatal(err)
	}

	// Stale sign-in: re-auth first.
	rec := st.do(st.stale, "POST", "/admin/posts/"+target+"/delete", url.Values{"confirm": {"target"}}, false)
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/admin/reauth?next=") {
		t.Fatalf("stale delete %d %q", rec.Code, rec.Header().Get("Location"))
	}
	// The editor shows the typed confirmation.
	must(t, st.do(st.fresh, "GET", "/admin/posts/"+target, nil, false).Body.String(),
		"1 entries link here. Type", `data-confirm="target"`, `data-needs-confirm`)
	// Wrong slug: refused by the server, not just the disabled button.
	rec = st.do(st.fresh, "POST", "/admin/posts/"+target+"/delete", url.Values{"confirm": {"targe"}}, false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "Type the slug exactly") {
		t.Fatalf("mismatch %d", rec.Code)
	}
	// Right slug: gone, and the URL becomes a 410 tombstone.
	rec = st.do(st.fresh, "POST", "/admin/posts/"+target+"/delete", url.Values{"confirm": {"target"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete %d %s", rec.Code, rec.Body.String())
	}
	if _, err := NewRepo(st.s.DB).Get(ctx, target); err == nil {
		t.Fatal("post still exists")
	}
	res, _ := st.s.DB.Query(ctx, `SELECT code FROM redirects WHERE from_path = '/en/posts/target'`)
	rows, _ := store.Scan[struct {
		Code int `json:"code"`
	}](res)
	if len(rows) != 1 || rows[0].Code != 410 {
		t.Fatalf("tombstone %+v", rows)
	}
	// Without backlinks no confirmation is needed; a redirect target is kept.
	solo := st.newPost(t, "Solo")
	st.do(st.fresh, "POST", "/admin/posts/"+solo+"/publish", editorForm("Solo", "solo", "Body."), false)
	if rec := st.do(st.fresh, "POST", "/admin/posts/"+solo+"/delete", url.Values{"redirect_to": {"/en/posts"}}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("solo delete %d", rec.Code)
	}
	res, _ = st.s.DB.Query(ctx, `SELECT code FROM redirects WHERE from_path = '/en/posts/solo' AND to_path = '/en/posts'`)
	if rows, _ := store.Scan[struct {
		Code int `json:"code"`
	}](res); len(rows) != 1 || rows[0].Code != 301 {
		t.Fatalf("redirect %+v", rows)
	}
}

func TestTranslateCreatesDraftOnce(t *testing.T) {
	st := newAdminSite(t)
	id := st.newPost(t, "Bilingual")
	st.do(st.fresh, "POST", "/admin/posts/"+id, editorForm("Bilingual", "bilingual", "Original text."), false)
	rec := st.do(st.fresh, "POST", "/admin/posts/"+id+"/translate", url.Values{"lang": {"tr"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("translate %d %s", rec.Code, rec.Body.String())
	}
	trID := strings.TrimPrefix(rec.Header().Get("Location"), "/admin/posts/")
	tr, err := NewRepo(st.s.DB).Get(context.Background(), trID)
	src, _ := NewRepo(st.s.DB).Get(context.Background(), id)
	if err != nil || tr.Lang != "tr" || tr.TranslationGroupID != src.TranslationGroupID || tr.Status != content.Draft {
		t.Fatalf("tr draft %+v %v", tr.Entry, err)
	}
	// A second click opens the same draft.
	if rec := st.do(st.fresh, "POST", "/admin/posts/"+id+"/translate", url.Values{"lang": {"tr"}}, false); rec.Header().Get("Location") != "/admin/posts/"+trID {
		t.Fatalf("second translate went to %q", rec.Header().Get("Location"))
	}
	must(t, st.do(st.fresh, "GET", "/admin/posts/"+id, nil, false).Body.String(), `href="/admin/posts/`+trID+`">draft →`)
}

func TestPreviewAndList(t *testing.T) {
	st := newAdminSite(t)
	rec := st.do(st.fresh, "POST", "/admin/preview", url.Values{"markdown": {"## Hi\n\n> [!TIP]\n> x"}, "lang": {"en"}, "title": {"<T>"}}, true)
	must(t, rec.Body.String(), `<h1 class="ed-prev-title">&lt;T&gt;</h1>`, `<h2 id="hi">`, "callout")
	st.newPost(t, "Listed")
	must(t, st.do(st.fresh, "GET", "/admin/posts", nil, false).Body.String(), ">Listed</a>", `aria-current="page"><span>posts</span>`)
	if rec := st.do(nil, "GET", "/admin/posts", nil, false); rec.Code != http.StatusSeeOther {
		t.Errorf("posts without session: %d", rec.Code)
	}
}

func TestEntryListFiltersAndBulk(t *testing.T) {
	st := newAdminSite(t)
	ctx := context.Background()
	a := st.newPost(t, "Alpha")
	st.do(st.fresh, "POST", "/admin/posts/"+a, editorForm("Alpha", "alpha", "Text."), false)
	b := st.newPost(t, "Beta")
	st.do(st.fresh, "POST", "/admin/posts/"+b, editorForm("Beta", "beta", "Text."), false)
	rec := st.do(st.fresh, "POST", "/admin/posts", url.Values{"title": {"Gamma tr"}, "lang": {"tr"}}, false)
	if rec.Code != http.StatusSeeOther {
		t.Fatal(rec.Code)
	}

	// Filters narrow the table; the translation column shows the other language.
	body := st.do(st.fresh, "GET", "/admin/posts?lang=tr", nil, false).Body.String()
	if !strings.Contains(body, ">Gamma tr</a>") || strings.Contains(body, ">Alpha</a>") {
		t.Fatalf("lang filter:\n%s", body)
	}
	must(t, st.do(st.fresh, "GET", "/admin/posts?tag=htmx", nil, false).Body.String(), ">Alpha</a>", ">Beta</a>", "tr ✕")

	// Bulk publish with a stale sign-in: re-auth first.
	form := url.Values{"action": {"publish"}, "id": {a, b}, "back": {"?status=draft"}}
	if rec := st.do(st.stale, "POST", "/admin/posts/bulk", form, false); rec.Code != http.StatusSeeOther || !strings.HasPrefix(rec.Header().Get("Location"), "/admin/reauth") {
		t.Fatalf("stale bulk %d %q", rec.Code, rec.Header().Get("Location"))
	}
	rec = st.do(st.fresh, "POST", "/admin/posts/bulk", form, false)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/posts?status=draft&done=publish" {
		t.Fatalf("bulk publish %d %q", rec.Code, rec.Header().Get("Location"))
	}
	repo := NewRepo(st.s.DB)
	for _, id := range []string{a, b} {
		if p, _ := repo.Get(ctx, id); p.Status != content.Published {
			t.Errorf("%s not published", id)
		}
	}
	must(t, st.do(st.fresh, "GET", "/admin/posts?status=published&done=publish", nil, false).Body.String(), "The selected posts were published.", `class="adm-st s-published"`)

	// Archive takes them off the site.
	if rec := st.do(st.fresh, "POST", "/admin/posts/bulk", url.Values{"action": {"archive"}, "id": {a}}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("archive %d", rec.Code)
	}
	if p, _ := repo.Get(ctx, a); p.Status != content.Archived {
		t.Errorf("not archived: %s", p.Status)
	}
	if _, err := repo.GetPublished(ctx, "en", "alpha"); err == nil {
		t.Error("archived post still public")
	}
	// Nothing selected: the list comes back with an error.
	if rec := st.do(st.fresh, "POST", "/admin/posts/bulk", url.Values{"action": {"publish"}}, false); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("empty bulk %d", rec.Code)
	}
}

func TestCoverMustHaveAltText(t *testing.T) {
	st := newAdminSite(t)
	ctx := context.Background()
	id := st.newPost(t, "Covered")
	if _, err := st.s.DB.Query(ctx, `INSERT INTO media (id, r2_key, mime, created_at) VALUES ('M1', 'media/M1/a.png', 'image/png', 1)`); err != nil {
		t.Fatal(err)
	}
	f := editorForm("Covered", "covered", "x")
	f.Set("cover", "M1")
	rec := st.do(st.fresh, "POST", "/admin/posts/"+id, f, false)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "cover image is missing or has no alt text") {
		t.Fatalf("cover without alt %d", rec.Code)
	}
	if _, err := st.s.DB.Query(ctx, `UPDATE media SET alt_tr = 'a', alt_en = 'a' WHERE id = 'M1'`); err != nil {
		t.Fatal(err)
	}
	if rec := st.do(st.fresh, "POST", "/admin/posts/"+id, f, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("cover with alt %d", rec.Code)
	}
	p, _ := NewRepo(st.s.DB).Get(ctx, id)
	if p.CoverMediaID == nil || *p.CoverMediaID != "M1" {
		t.Fatalf("cover not saved: %v", p.CoverMediaID)
	}
	must(t, st.do(st.fresh, "GET", "/admin/posts/"+id, nil, false).Body.String(), `<img class="ed-cover" src="/media/M1/a.png"`, `<option value="M1" selected>`)
}

func TestRevisionsDiffAndRestore(t *testing.T) {
	st := newAdminSite(t)
	ctx := context.Background()
	id := st.newPost(t, "History")
	for _, md := range []string{"one\ntwo\nthree", "one\nTWO\nthree\nfour"} {
		st.do(st.fresh, "POST", "/admin/posts/"+id, editorForm("History", "history", md), false)
	}
	revs, _ := content.Revisions(ctx, st.s.DB, id, 10)
	if len(revs) != 3 {
		t.Fatalf("revisions %d", len(revs))
	}
	// Default view: newest against the one before it.
	body := st.do(st.fresh, "GET", "/admin/posts/"+id+"/revisions", nil, false).Body.String()
	must(t, body, `<span class="ok">+2</span> <span class="bad">−1</span>`, `<tr class="chg"><td class="ln">2</td><td class="l">two</td><td class="ln">2</td><td class="r">TWO</td></tr>`,
		`<tr class="add"><td class="ln"></td><td class="l"></td><td class="ln">4</td><td class="r">four</td></tr>`)
	if strings.Contains(body, "restore this revision") {
		t.Error("restore offered for the current revision")
	}
	// Restore the middle one: a new manual revision with a note.
	old := revs[1].ID
	must(t, st.do(st.fresh, "GET", "/admin/posts/"+id+"/revisions?b="+old, nil, false).Body.String(), "restore this revision")
	if rec := st.do(st.fresh, "POST", "/admin/posts/"+id+"/revisions/"+old+"/restore", url.Values{}, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("restore %d", rec.Code)
	}
	revs, _ = content.Revisions(ctx, st.s.DB, id, 10)
	cur, _ := content.GetRevision(ctx, st.s.DB, revs[0].ID)
	if len(revs) != 4 || cur.BodyMD != "one\ntwo\nthree" || !strings.HasPrefix(cur.Note, "restored from ") || cur.Origin != content.OriginManual {
		t.Fatalf("after restore %d revs, current %+v", len(revs), cur)
	}
	// A revision of another post is not restorable here.
	other := st.newPost(t, "Other")
	orevs, _ := content.Revisions(ctx, st.s.DB, other, 1)
	if rec := st.do(st.fresh, "POST", "/admin/posts/"+id+"/revisions/"+orevs[0].ID+"/restore", url.Values{}, false); rec.Code != http.StatusNotFound {
		t.Errorf("foreign revision %d", rec.Code)
	}
}

// The editor's POSSE section (Phase 8.3): share links once published, the
// copies' URLs saved with the post, a bad URL saves nothing.
func TestSyndication(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	st := newAdminSite(t)
	id := st.newPost(t, "Share me")
	body := st.do(st.fresh, "GET", "/admin/posts/"+id, nil, false).Body.String()
	must(t, body, "Share links appear once it is published.", `name="syndication"`, `/static/js/share.js`)

	form := editorForm("Share me", "share-me", "Body.")
	form.Set("syndication", "https://x.com/halid/status/1\nhttps://www.linkedin.com/posts/halid-1")
	if rec := st.do(st.fresh, "POST", "/admin/posts/"+id+"/publish", form, false); rec.Code != http.StatusSeeOther {
		t.Fatalf("publish %d %s", rec.Code, rec.Body.String())
	}
	body = st.do(st.fresh, "GET", "/admin/posts/"+id, nil, false).Body.String()
	must(t, body, `href="https://x.com/intent/tweet?text=Share%20me&amp;url=https%3A%2F%2Fhalid.org%2Fen%2Fposts%2Fshare-me"`,
		`href="https://www.linkedin.com/sharing/share-offsite/?url=https%3A%2F%2Fhalid.org%2Fen%2Fposts%2Fshare-me"`,
		"https://x.com/halid/status/1\nhttps://www.linkedin.com/posts/halid-1\n</textarea>")

	form.Set("syndication", "not a url")
	form.Set("title", "Changed title")
	rec := st.do(st.fresh, "POST", "/admin/posts/"+id, form, false)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad url %d", rec.Code)
	}
	must(t, rec.Body.String(), "Syndication URLs must be http(s) links")
	if p, _ := NewRepo(st.s.DB).Get(context.Background(), id); p.Title != "Share me" {
		t.Errorf("saved with a bad syndication URL: %q", p.Title)
	}
}

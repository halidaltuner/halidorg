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
	"halid.org/engine/internal/content/book"
	"halid.org/engine/internal/content/buildlog"
	"halid.org/engine/internal/content/fav"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/content/page"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/content/shortadmin"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/content/term"
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
		note.NewEditor, snippet.NewEditor, link.NewEditor, fav.NewEditor, project.NewEditor, buildlog.NewEditor, page.NewEditor,
		term.NewEditor, book.NewEditor,
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

func TestProjectEditor(t *testing.T) {
	st := newSite(t)
	ctx := context.Background()
	rack := "01K0000000000000000000HM01"

	body := st.do(st.fresh, "GET", "/admin/projects/new", nil).Body.String()
	must(t, body, `name="project_type" value="software" checked`, `<option value="active" selected>active</option>`, `name="part_name"`, `name="tone" value="orange"`)

	// A homelab build: rows, an amount typed the Turkish way, a bad amount.
	f := url.Values{"title": {"3-node Proxmox cluster"}, "lang": {"en"}, "project_type": {"homelab"}, "project_status": {"running"},
		"category": {rack}, "tagline": {"Three mini PCs."}, "currency": {"TRY"}, "time_spent": {"6 weekends"}, "started": {"2025-11"},
		"tone": {"orange"}, "markdown": {"## Goal\n\nQuiet."},
		"fact_key": {"nodes", "idle", ""}, "fact_value": {"3", "38 W", ""}, "fact_order": {"0", "1", "2"},
		"part_name": {"3× N100 mini PC", "UPS", ""}, "part_qty": {"3", "", ""}, "part_price": {"27.000", "3.900,50", ""},
		"part_currency": {"", "", ""}, "part_url": {"", "", ""}, "part_order": {"0", "1", "2"},
		"link_label": {"build log", ""}, "link_url": {"#log", ""}, "link_order": {"0", "1"},
		"cost_total": {"lots"}, "action": {"save"}}
	rec := st.do(st.fresh, "POST", "/admin/projects", f)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("bad cost %d", rec.Code)
	}
	must(t, rec.Body.String(), `The cost must be an amount (27000 or 149.90).`, `value="27000"`, `value="3900.50"`)
	f.Set("cost_total", "")
	id := idOf(t, st.do(st.fresh, "POST", "/admin/projects", f), "/admin/projects")
	p, err := project.NewRepo(st.s.DB).Get(ctx, id)
	if err != nil || p.Type != project.Homelab || len(p.Facts) != 2 || len(p.Parts) != 2 || *p.Parts[0].Price != 2700000 ||
		*p.Parts[1].Price != 390050 || p.Parts[0].Qty != 3 || len(p.Links) != 1 || *p.StartedAt == 0 || *p.Tone != "orange" || p.Slug != "3-node-proxmox-cluster" {
		t.Fatalf("project %+v %v", p, err)
	}
	body = st.do(st.fresh, "GET", "/admin/projects/"+id, nil).Body.String()
	must(t, body, `value="27000"`, `value="3900.50"`, `value="2025-11"`, `name="project_type" value="homelab" checked`,
		`href="/admin/buildlogs/new?project=`+id+`"`)

	// Software cannot keep the parts.
	f.Set("project_type", "software")
	f.Set("slug", p.Slug)
	if rec := st.do(st.fresh, "POST", "/admin/projects/"+id, f); rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("software with parts %d", rec.Code)
	} else {
		must(t, rec.Body.String(), `Only homelab builds have parts.`)
	}

	// A build log from the project page: project and language filled in.
	body = st.do(st.fresh, "GET", "/admin/buildlogs/new?project="+id, nil).Body.String()
	must(t, body, `<option value="`+id+`" selected>3-node Proxmox cluster · en</option>`, `name="lang" value="en" checked`, `type="date" name="log_date"`)
	lid := idOf(t, st.do(st.fresh, "POST", "/admin/buildlogs", url.Values{"title": {"Printed the brackets"}, "lang": {"en"}, "project": {id},
		"log_date": {"2026-10-09"}, "hours": {"2,5"}, "markdown": {"PLA, 4 hours of printing."}, "action": {"publish"}}), "/admin/buildlogs")
	b, err := buildlog.NewRepo(st.s.DB).Get(ctx, lid)
	if err != nil || b.Status != content.Published || *b.Hours != 2.5 || b.ProjectSlug != p.Slug {
		t.Fatalf("log %+v %v", b, err)
	}
	must(t, st.do(st.fresh, "GET", "/admin/buildlogs/"+lid, nil).Body.String(),
		`<a class="mono ent-view" href="/en/projects/3-node-proxmox-cluster/log/printed-the-brackets">`, `value="2026-10-09"`, `value="2.5"`,
		`<a class="adm-nav-i sub" href="/admin/projects" aria-current="page">`)
	must(t, st.do(st.fresh, "GET", "/admin/projects/"+id, nil).Body.String(), `<span class="ent-title">Printed the brackets</span>`)

	// Deleting the project takes the log with it.
	if rec := st.do(st.fresh, "POST", "/admin/projects/"+id+"/delete", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("delete %d", rec.Code)
	}
	if _, err := buildlog.NewRepo(st.s.DB).Get(ctx, lid); err == nil {
		t.Error("build log outlived its project")
	}
}

func TestPageEditor(t *testing.T) {
	st := newSite(t)
	ctx := context.Background()

	// A new page picks its template first; ?template= presets it.
	body := st.do(st.fresh, "GET", "/admin/pages/new", nil).Body.String()
	must(t, body, `name="template" value="about"`, `name="template" value="now"`, `Pick the template and save`)
	if strings.Contains(body, `name="build_title"`) || strings.Contains(body, `name="markdown"`) {
		t.Error("sections before a template is chosen")
	}
	body = st.do(st.fresh, "GET", "/admin/pages/new?template=now", nil).Body.String()
	must(t, body, `name="template" value="now" checked`, `name="title" value="Now"`, `name="build_title"`, `name="learn_level"`, `name="not_text"`)
	if strings.Contains(body, `name="markdown"`) {
		t.Error("Now has no body")
	}
	f := url.Values{"title": {"Now"}, "lang": {"en"}, "template": {"now"}, "place": {"istanbul"},
		"build_title": {"admin editor", ""}, "build_note": {"", ""}, "build_stage": {"in progress", ""}, "build_pct": {"65", ""},
		"build_href": {"/en/projects/site", ""}, "build_cover": {"", ""}, "build_tone": {"green", ""}, "build_order": {"0", "1"},
		"learn_title": {"Zig", ""}, "learn_note": {"", ""}, "learn_level": {"11", ""}, "learn_hours": {"26", ""}, "learn_tone": {"indigo", ""}, "learn_order": {"0", "1"},
		"not_text": {"side projects"}, "not_order": {"0"}, "action": {"save"}}
	rec := st.do(st.fresh, "POST", "/admin/pages", f)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("level 11 %d", rec.Code)
	}
	must(t, rec.Body.String(), `Learning 1: level is 1 to 10`, `value="11"`)
	f.Set("learn_level", "3")
	id := idOf(t, st.do(st.fresh, "POST", "/admin/pages", f), "/admin/pages")
	p, err := page.NewRepo(st.s.DB).Get(ctx, id)
	if err != nil || p.Slug != "now" || p.Template != page.Now || p.Data.Place != "istanbul" || len(p.Data.Building) != 1 ||
		p.Data.Building[0].Pct != 65 || len(p.Data.Learning) != 1 || p.Data.Learning[0].Level != 3 || len(p.Data.NotDoing) != 1 {
		t.Fatalf("page %+v %v", p, err)
	}
	body = st.do(st.fresh, "GET", "/admin/pages/"+id, nil).Body.String()
	must(t, body, `name="build_title" value="admin editor"`, `<option value="green" selected>green</option>`, `name="learn_level" value="3"`)
	if strings.Contains(body, `name="template" value="about"`) {
		t.Error("template offered again on edit")
	}
	// A second Now in the same language is refused.
	if rec := st.do(st.fresh, "POST", "/admin/pages", f); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("second Now %d", rec.Code)
	} else {
		must(t, rec.Body.String(), `This language already has this page.`)
	}

	// Publish, then a saved edit waits for the next publish.
	f.Set("action", "publish")
	if rec := st.do(st.fresh, "POST", "/admin/pages/"+id+"/publish", f); rec.Code != http.StatusSeeOther {
		t.Fatalf("publish %d %s", rec.Code, rec.Body.String())
	}
	f.Set("action", "save")
	f.Set("place", "ankara")
	st.do(st.fresh, "POST", "/admin/pages/"+id, f)
	must(t, st.do(st.fresh, "GET", "/admin/pages/"+id, nil).Body.String(), `href="/en/now"`, `changes not live`)

	// About has a body, the CTA and CV fields.
	body = st.do(st.fresh, "GET", "/admin/pages/new?template=about", nil).Body.String()
	must(t, body, `name="markdown"`, `name="cta_label"`, `name="skill_items"`, `name="job_role"`, `<select name="cv">`)
	f = url.Values{"title": {"About"}, "lang": {"en"}, "template": {"about"}, "name": {"Halid"}, "markdown": {"Backend engineer."},
		"cta_label": {"say hello", ""}, "cta_url": {"javascript:alert(1)", ""}, "cta_order": {"0", "1"},
		"skill_group": {"languages"}, "skill_items": {"Go, SQL, "}, "skill_tone": {"cyan"}, "skill_order": {"0"}, "action": {"save"}}
	rec = st.do(st.fresh, "POST", "/admin/pages", f)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("javascript cta %d", rec.Code)
	}
	must(t, rec.Body.String(), `Buttons 1: a label and an https, mailto: or /site URL`)
	f.Set("cta_url", "mailto:me@halid.org")
	aid := idOf(t, st.do(st.fresh, "POST", "/admin/pages", f), "/admin/pages")
	a, err := page.NewRepo(st.s.DB).Get(ctx, aid)
	if err != nil || len(a.Data.Skills) != 1 || len(a.Data.Skills[0].Items) != 2 || a.Data.Name != "Halid" || a.Data.Place != "" {
		t.Fatalf("about %+v %v", a, err)
	}
}

func TestTermAndBookEditors(t *testing.T) {
	st := newSite(t)
	rec := st.do(st.fresh, "POST", "/admin/glossary", url.Values{"title": {"KV"}, "lang": {"en"}, "aliases": {"key-value store, , KV"},
		"short_def": {"Edge key-value store."}, "markdown": {"Eventually consistent."}, "action": {"save"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("term %d %s", rec.Code, rec.Body.String())
	}
	body := st.do(st.fresh, "GET", rec.Header().Get("Location"), nil).Body.String()
	must(t, body, `name="aliases" value="key-value store"`, `Edge key-value store.</textarea>`)

	rec = st.do(st.fresh, "POST", "/admin/reading", url.Values{"title": {"DDIA"}, "lang": {"en"}, "author": {"Kleppmann"}, "book_status": {"reading"},
		"progress_pct": {"72%"}, "started": {"2026-09-01"}, "rating": {"5"}, "action": {"save"}})
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("book %d %s", rec.Code, rec.Body.String())
	}
	body = st.do(st.fresh, "GET", rec.Header().Get("Location"), nil).Body.String()
	must(t, body, `name="progress_pct" value="72"`, `name="started" value="2026-09-01"`, `value="reading" checked`, `value="5" checked`)
	if rec := st.do(st.fresh, "POST", "/admin/reading", url.Values{"title": {"Bad"}, "lang": {"en"}, "book_status": {"reading"}, "finished": {"soon"},
		"action": {"save"}}); rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "finish date") {
		t.Errorf("bad date %d", rec.Code)
	}
}

package taxonomy

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

type site struct {
	mux *http.ServeMux
	s   store.Stores
	c   *http.Cookie
}

func newSite(t *testing.T) site {
	t.Helper()
	t.Setenv("APP_ENV", "staging")
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	h, err := New(s.DB, admin.NewUI(s.DB), a, jobs.New(s.DB, s.Queue))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	c, _ := a.TestSession(context.Background(), "me@halid.org", time.Hour)
	return site{mux: mux, s: s, c: c}
}

func (st site) do(method, target string, form url.Values, siteHdr string) *httptest.ResponseRecorder {
	var req *http.Request
	if form != nil {
		req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, target, nil)
	}
	req.Header.Set("Sec-Fetch-Site", siteHdr)
	req.Header.Set("Accept-Language", "en")
	req.AddCookie(st.c)
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

// queued returns the payloads of the render jobs sent so far.
func queued(t *testing.T, st site) []jobs.EntryChanged {
	t.Helper()
	var out []jobs.EntryChanged
	for _, raw := range st.s.Queue.(*storetest.Queue).Sent {
		var m struct {
			Payload jobs.EntryChanged `json:"payload"`
		}
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m.Payload)
	}
	return out
}

func TestCategories(t *testing.T) {
	st := newSite(t)
	ctx := context.Background()

	body := st.do("GET", "/admin/categories", nil, "none").Body.String()
	must(t, body, `<a href="/admin/categories?kind=link" aria-current="page">links</a>`, "No categories of this kind yet.",
		`<label class="tx-tone tone-rose" title="rose">`)

	// Create: slug from the name, tone, then a child.
	rec := st.do("POST", "/admin/categories", url.Values{"kind": {"link"}, "name_en": {"Architecture"}, "name_tr": {"Mimari"}, "tone": {"green"}}, "same-origin")
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	cats, _ := content.Categories(ctx, st.s.DB, "link")
	if len(cats) != 1 || cats[0].Slug != "architecture-mimari" || cats[0].Tone == nil || *cats[0].Tone != "green" {
		t.Fatalf("created %+v", cats)
	}
	arch := cats[0]
	st.do("POST", "/admin/categories", url.Values{"kind": {"link"}, "slug": {"books"}, "name_en": {"Books"}, "parent": {arch.ID}}, "same-origin")
	st.do("POST", "/admin/categories", url.Values{"kind": {"link"}, "slug": {"web"}, "name_en": {"Web"}}, "same-origin")
	cats, _ = content.Categories(ctx, st.s.DB, "link")
	books := cats[1]
	if books.Slug != "books" || books.ParentID == nil || *books.ParentID != arch.ID {
		t.Fatalf("child %+v", books)
	}

	// Refusals: duplicate slug, no name, a child as parent, another kind's parent.
	for name, f := range map[string]url.Values{
		"duplicate":  {"kind": {"link"}, "slug": {"web"}, "name_en": {"x"}},
		"no name":    {"kind": {"link"}, "slug": {"x"}},
		"grandchild": {"kind": {"link"}, "slug": {"y"}, "name_en": {"y"}, "parent": {books.ID}},
		"bad tone":   {"kind": {"link"}, "slug": {"z"}, "name_en": {"z"}, "tone": {"pink"}},
	} {
		if rec := st.do("POST", "/admin/categories", f, "same-origin"); rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("%s: %d", name, rec.Code)
		}
	}
	if rec := st.do("POST", "/admin/categories", url.Values{"kind": {"snippet"}, "slug": {"s"}, "name_en": {"s"}, "parent": {arch.ID}}, "same-origin"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("parent of another kind: %d", rec.Code)
	}

	// Rename re-renders the category's entries, with their refs from before.
	l, err := link.NewRepo(st.s.DB).Create(ctx, link.Draft{Link: link.Link{Entry: content.Entry{Slug: "grug", Lang: "en", Title: "Grug"}, URL: "https://grugbrain.dev/", CategoryID: &books.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if rec := st.do("POST", "/admin/categories/"+books.ID, url.Values{"name_en": {"Reading"}, "name_tr": {"Okuma"}, "parent": {arch.ID}, "tone": {""}}, "same-origin"); rec.Code != http.StatusSeeOther {
		t.Fatalf("save %d %s", rec.Code, rec.Body.String())
	}
	got, _ := content.GetCategory(ctx, st.s.DB, books.ID)
	if got.NameEN != "Reading" || got.NameTR != "Okuma" || got.Tone != nil || got.Slug != "books" {
		t.Errorf("saved %+v", got)
	}
	q := queued(t, st)
	if len(q) != 1 || q[0].EntryID != l.ID || q[0].Prev == nil || q[0].Prev.Slug != "grug" {
		t.Errorf("render jobs %+v", q)
	}
	// A parent cannot get a parent while it has children.
	if rec := st.do("POST", "/admin/categories/"+arch.ID, url.Values{"name_en": {"Architecture"}, "parent": {cats[2].ID}}, "same-origin"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("parent with children moved under another: %d", rec.Code)
	}

	// Reorder: web moves up above books.
	st.do("POST", "/admin/categories/"+cats[2].ID+"/move", url.Values{"dir": {"up"}}, "same-origin")
	cats, _ = content.Categories(ctx, st.s.DB, "link")
	if cats[0].Slug != "architecture-mimari" || cats[1].Slug != "web" || cats[2].Slug != "books" || cats[1].Sort != 20 {
		t.Errorf("order %v %v %v", cats[0].Slug, cats[1].Slug, cats[2].Slug)
	}

	if rec := st.do("POST", "/admin/categories/"+books.ID, url.Values{"name_en": {"x"}}, "cross-site"); rec.Code == http.StatusSeeOther {
		t.Error("cross-site save accepted")
	}
}

func TestTags(t *testing.T) {
	st := newSite(t)
	ctx := context.Background()
	notes := note.NewRepo(st.s.DB)
	mk := func(slug string, tags ...string) string {
		n, err := notes.Create(ctx, note.Draft{Note: note.Note{Entry: content.Entry{Slug: slug, Lang: "en"}}, Markdown: "x", Tags: tags})
		if err != nil {
			t.Fatal(err)
		}
		return n.ID
	}
	a := mk("a", "golang")
	b := mk("b", "golang", "go")
	mk("c", "go")
	tags, _ := content.AllTags(ctx, st.s.DB)
	byslug := map[string]content.TagRow{}
	for _, x := range tags {
		byslug[x.Slug] = x
	}
	body := st.do("GET", "/admin/tags", nil, "none").Body.String()
	must(t, body, `<td class="mono">#golang</td>`, `<form id="t-`+byslug["go"].ID+`"`, "merge tags")

	// Rename and tone.
	if rec := st.do("POST", "/admin/tags/"+byslug["go"].ID, url.Values{"name_en": {"Go"}, "name_tr": {"Go"}, "tone": {"cyan"}}, "same-origin"); rec.Code != http.StatusSeeOther {
		t.Fatalf("save %d", rec.Code)
	}
	if n := len(queued(t, st)); n != 2 {
		t.Errorf("rename queued %d jobs, want 2 (b and c)", n)
	}

	// Merge golang into go: a moves, b keeps one go, golang is gone.
	if rec := st.do("POST", "/admin/tags/merge", url.Values{"from": {byslug["golang"].ID}, "into": {byslug["go"].ID}}, "same-origin"); rec.Code != http.StatusSeeOther {
		t.Fatalf("merge %d %s", rec.Code, rec.Body.String())
	}
	for _, id := range []string{a, b} {
		got, _ := content.TagsOf(ctx, st.s.DB, id)
		if len(got) != 1 || got[0].Slug != "go" {
			t.Errorf("%s tags %+v", id, got)
		}
	}
	tags, _ = content.AllTags(ctx, st.s.DB)
	if len(tags) != 1 || tags[0].Entries != 3 {
		t.Errorf("after merge %+v", tags)
	}
	q := queued(t, st)[2:]
	if len(q) != 2 || q[0].Prev == nil || !contains(q[0].Prev.Tags, "golang") {
		t.Errorf("merge jobs must carry the old tag: %+v", q)
	}
	if rec := st.do("POST", "/admin/tags/merge", url.Values{"from": {tags[0].ID}, "into": {tags[0].ID}}, "same-origin"); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("merge into itself: %d", rec.Code)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

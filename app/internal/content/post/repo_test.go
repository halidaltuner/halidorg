package post

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func tick(t *testing.T) func() {
	t.Helper()
	now := time.Unix(1_790_000_000, 0)
	old := content.Clock
	content.Clock = func() time.Time { now = now.Add(time.Second); return now }
	t.Cleanup(func() { content.Clock = old })
	return func() {}
}

func draft(slug, lang, title, md string, tags ...string) Draft {
	return Draft{
		Post:     Post{Entry: content.Entry{Slug: slug, Lang: lang, Title: title}},
		Markdown: md,
		Tags:     tags,
	}
}

func revisionCount(t *testing.T, r *Repo, id string) []content.Revision {
	t.Helper()
	revs, err := content.Revisions(context.Background(), r.db, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	return revs
}

func TestCreateGetAndTags(t *testing.T) {
	tick(t)
	ctx := context.Background()
	r := NewRepo(storetest.NewDB(t))

	words := strings.Repeat("kelime ", 500)
	p, err := r.Create(ctx, draft("htmx-admin", "en", "Notes on an htmx-first admin",
		"## Why\n\n"+words+"\n\n```mermaid\ngraph TD; A-->B\n```\n", "htmx", "#Go", "go", "  "))
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" || p.TranslationGroupID == "" || p.Status != content.Draft || p.Visibility != content.Public {
		t.Fatalf("defaults not applied: %+v", p.Entry)
	}
	got, err := r.Get(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.ReadingMinutes != 3 || !got.Mermaid || got.CurrentRevisionID == nil || got.PublishedRevisionID != nil {
		t.Errorf("derived fields: %+v", got)
	}
	rev, err := content.GetRevision(ctx, r.db, *got.CurrentRevisionID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rev.BodyHTML, `<h2 id="why">`) || rev.TOCJSON == "[]" || rev.Origin != content.OriginManual {
		t.Errorf("revision: %+v", rev)
	}
	tags, err := content.TagsOf(ctx, r.db, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(tags) != 2 || tags[0].Slug != "go" || tags[1].Slug != "htmx" {
		t.Errorf("tags = %+v", tags)
	}

	// Same slug and language is refused as a field error; other language is fine.
	if _, err := r.Create(ctx, draft("htmx-admin", "en", "Again", "x")); err == nil {
		t.Error("duplicate slug accepted")
	} else if ve, ok := err.(content.ValidationError); !ok || ve["slug"] == "" {
		t.Errorf("duplicate slug error = %v", err)
	}
	tr := draft("htmx-admin", "tr", "htmx öncelikli admin üzerine notlar", "x")
	tr.TranslationGroupID = p.TranslationGroupID
	if _, err := r.Create(ctx, tr); err != nil {
		t.Errorf("translation: %v", err)
	}
	// A second tr sibling in the same group is refused.
	tr2 := draft("other", "tr", "x", "x")
	tr2.TranslationGroupID = p.TranslationGroupID
	if _, err := r.Create(ctx, tr2); err == nil {
		t.Error("second tr sibling accepted")
	}
}

func TestAutosaveCoalescing(t *testing.T) {
	tick(t)
	ctx := context.Background()
	r := NewRepo(storetest.NewDB(t))
	p, err := r.Create(ctx, draft("rev", "en", "Revisions", "v1"))
	if err != nil {
		t.Fatal(err)
	}
	d := draft("rev", "en", "Revisions", "v2")
	d.ID = p.ID
	d.Status = content.Draft
	d.Visibility = content.Public

	save := func(md, origin string) string {
		t.Helper()
		d.Markdown = md
		_, rev, err := r.Save(ctx, d, origin)
		if err != nil {
			t.Fatal(err)
		}
		return rev
	}
	a1 := save("auto 1", content.OriginAutosave)
	a2 := save("auto 2", content.OriginAutosave)
	a3 := save("auto 3", content.OriginAutosave)
	if a1 != a2 || a2 != a3 {
		t.Errorf("consecutive autosaves did not coalesce: %s %s %s", a1, a2, a3)
	}
	m1 := save("manual 1", content.OriginManual)
	if m1 == a3 {
		t.Error("manual save reused the autosave revision")
	}
	m2 := save("manual 1", content.OriginManual)
	if m2 == m1 {
		t.Error("explicit saves must always create a revision")
	}
	a4 := save("auto 4", content.OriginAutosave)
	a5 := save("auto 5", content.OriginAutosave)
	if a4 == m2 || a4 != a5 {
		t.Errorf("autosave after manual: a4=%s a5=%s m2=%s", a4, a5, m2)
	}
	revs := revisionCount(t, r, p.ID)
	// create, auto 1..3, manual 1, manual 1 again, auto 4..5
	if len(revs) != 5 {
		t.Fatalf("revisions = %d, want 5", len(revs))
	}
	got, _ := r.Get(ctx, p.ID)
	if *got.CurrentRevisionID != a5 {
		t.Error("current revision does not point at the newest save")
	}
	last, _ := content.GetRevision(ctx, r.db, a5)
	if last.BodyMD != "auto 5" || !strings.Contains(last.BodyHTML, "auto 5") {
		t.Errorf("coalesced revision body = %q", last.BodyMD)
	}
}

func TestPublishMovesPointerOnly(t *testing.T) {
	tick(t)
	ctx := context.Background()
	r := NewRepo(storetest.NewDB(t))

	// Untitled posts cannot publish.
	untitled, err := r.Create(ctx, draft("untitled", "en", "", "body"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Publish(ctx, untitled.ID); err == nil {
		t.Error("untitled post published")
	}

	p, err := r.Create(ctx, draft("pub", "en", "Published", "first"))
	if err != nil {
		t.Fatal(err)
	}
	pub, err := r.Publish(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if pub.Status != content.Published || pub.PublishedAt == nil || pub.PublishedRevisionID == nil ||
		*pub.PublishedRevisionID != *pub.CurrentRevisionID {
		t.Fatalf("after publish: %+v", pub.Entry)
	}
	firstAt := *pub.PublishedAt

	// Editing a published post moves current, not published.
	d := Draft{Post: pub, Markdown: "second"}
	saved, rev, err := r.Save(ctx, d, content.OriginManual)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := r.Get(ctx, p.ID)
	if *got.PublishedRevisionID == rev || *got.CurrentRevisionID != rev || saved.Status != content.Published {
		t.Errorf("save moved the published pointer: %+v", got.Entry)
	}
	pubRev, _ := content.GetRevision(ctx, r.db, *got.PublishedRevisionID)
	if pubRev.BodyMD != "first" {
		t.Errorf("public body = %q, want first", pubRev.BodyMD)
	}

	// Republish keeps the original published_at.
	again, err := r.Publish(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if *again.PublishedRevisionID != rev || *again.PublishedAt != firstAt {
		t.Errorf("republish: %+v", again.Entry)
	}

	if _, err := r.GetPublished(ctx, "en", "pub"); err != nil {
		t.Errorf("GetPublished: %v", err)
	}
	if _, err := r.GetPublished(ctx, "en", "untitled"); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("draft visible publicly: %v", err)
	}
	// Create cannot publish directly.
	sneak := draft("sneak", "en", "Sneak", "x")
	sneak.Status = content.Published
	now := int64(1)
	sneak.PublishedAt = &now
	s, err := r.Create(ctx, sneak)
	if err != nil || s.Status != content.Draft {
		t.Errorf("create published: %v %+v", err, s.Entry)
	}
}

func TestListAndDelete(t *testing.T) {
	tick(t)
	ctx := context.Background()
	r := NewRepo(storetest.NewDB(t))
	var ids []string
	for i, s := range []string{"a", "b", "c", "d"} {
		p, err := r.Create(ctx, draft(s, "en", strings.ToUpper(s), "x", map[bool]string{true: "go", false: "misc"}[i%2 == 0]))
		if err != nil {
			t.Fatal(err)
		}
		if i < 3 {
			if _, err := r.Publish(ctx, p.ID); err != nil {
				t.Fatal(err)
			}
		}
		ids = append(ids, p.ID)
	}
	slugs := func(ps []Post) string {
		var out []string
		for _, p := range ps {
			out = append(out, p.Slug)
		}
		return strings.Join(out, ",")
	}
	all, _ := r.List(ctx, ListFilter{Lang: "en"})
	if slugs(all) != "d,c,b,a" {
		t.Errorf("admin list = %s", slugs(all))
	}
	pub, _ := r.List(ctx, ListFilter{Lang: "en", Status: content.Published})
	if slugs(pub) != "c,b,a" {
		t.Errorf("published list = %s", slugs(pub))
	}
	tagged, _ := r.List(ctx, ListFilter{Lang: "en", Status: content.Published, Tag: "go"})
	if slugs(tagged) != "c,a" {
		t.Errorf("tag list = %s", slugs(tagged))
	}
	page2, _ := r.List(ctx, ListFilter{Lang: "en", Status: content.Published, Limit: 2})
	before := page2[1].PublishedAt
	rest, _ := r.List(ctx, ListFilter{Lang: "en", Status: content.Published, Before: before})
	if slugs(page2) != "c,b" || slugs(rest) != "a" {
		t.Errorf("cursor: %s then %s", slugs(page2), slugs(rest))
	}

	if err := r.Delete(ctx, ids[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Get(ctx, ids[0]); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("deleted post still found: %v", err)
	}
	if revs := revisionCount(t, r, ids[0]); len(revs) != 0 {
		t.Errorf("revisions not cascaded: %d", len(revs))
	}
	if err := r.Delete(ctx, ids[0]); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestValidate(t *testing.T) {
	ok := Post{Entry: content.Entry{Kind: Kind, Slug: "ok", Lang: "en", Status: content.Draft, Visibility: content.Public}}
	if err := Validate(ok); err != nil {
		t.Errorf("valid post: %v", err)
	}
	bad := ok
	bad.Slug = "Bad Slug"
	bad.Lang = "de"
	bad.Summary = strings.Repeat("ş", 161)
	bad.CanonicalURL = "http://example.com"
	tone := "pink"
	bad.Tone = &tone
	bad.Status = content.Published
	err := Validate(bad)
	ve, isVE := err.(content.ValidationError)
	if !isVE {
		t.Fatalf("err = %v", err)
	}
	for _, f := range []string{"slug", "lang", "summary", "canonical_url", "tone", "title", "published_at"} {
		if ve[f] == "" {
			t.Errorf("missing error for %s", f)
		}
	}
	edge := ok
	edge.Summary = strings.Repeat("ğ", 160)
	if err := Validate(edge); err != nil {
		t.Errorf("160 runes should pass: %v", err)
	}
}

func TestReadingMinutes(t *testing.T) {
	for words, want := range map[int]int{0: 1, 1: 1, 220: 1, 221: 2, 2640: 12} {
		if got := ReadingMinutes(words); got != want {
			t.Errorf("ReadingMinutes(%d) = %d, want %d", words, got, want)
		}
	}
}

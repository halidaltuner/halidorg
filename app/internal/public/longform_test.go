package public

import (
	"context"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
)

func TestGardenAndRunbookPages(t *testing.T) {
	mux, db := newSite(t)
	ctx := context.Background()
	r := post.NewRepo(db)
	add := func(d post.Draft) post.Post {
		p, err := r.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if p, err = r.Publish(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
		return p
	}
	_, body := get(t, mux, "/en/garden")
	must(t, body, "Nothing in the garden yet.")
	add(post.Draft{Post: post.Post{Entry: content.Entry{Kind: post.Garden, Slug: "tending", Lang: "en", Title: "On tending notes", Summary: "Notes grow."}, Maturity: "evergreen"},
		Markdown: "Water them."})
	add(post.Draft{Post: post.Post{Entry: content.Entry{Kind: post.Garden, Slug: "idea", Lang: "en", Title: "Half an idea"}}, Markdown: "Maybe."})
	day := int64(1789948800)
	add(post.Draft{Post: post.Post{Entry: content.Entry{Kind: post.Runbook, Slug: "zfs-scrub", Lang: "en", Title: "Scrub a ZFS pool", Summary: "Monthly."},
		Platform: "TrueNAS 24", Risk: "med", VerifiedOn: &day, Prerequisites: "root shell\nA spare disk"}, Markdown: "Run `zpool scrub tank`."})

	_, body = get(t, mux, "/en/garden")
	must(t, body, `<h1>Digital garden</h1>`, `href="/en/garden?stage=evergreen" data-filter-key="evergreen">evergreen</a>`,
		`<a class="post-card" href="/en/garden/tending" data-filter-value="evergreen">`, `<span class="badge mat-seedling">seedling</span>`,
		`href="/en/garden/feed.xml"`)
	_, body = get(t, mux, "/en/garden?stage=seedling")
	if strings.Contains(body, "On tending notes") || !strings.Contains(body, `<meta name="robots" content="noindex">`) {
		t.Error("stage filter")
	}
	code, body := get(t, mux, "/en/garden/tending")
	if code != 200 {
		t.Fatalf("garden page %d", code)
	}
	must(t, body, `href="/en/garden">← digital garden`, `<span class="badge mat-evergreen">evergreen</span><span>tended `, `Water them.`,
		`<div class="cover tone-green`)

	_, body = get(t, mux, "/en/runbooks")
	must(t, body, `<a href="/en/runbooks/zfs-scrub">`, `<span>TrueNAS 24</span><span class="badge risk-med">medium</span><span>verified <time>2026-09-21</time></span>`)
	_, body = get(t, mux, "/en/runbooks/zfs-scrub")
	must(t, body, `<dt>platform</dt><dd>TrueNAS 24</dd>`, `<ul><li>root shell</li><li>A spare disk</li></ul>`, `<code>zpool scrub tank</code>`,
		`<div class="cover tone-navy`)

	// Each lives in its own section: not a post, not in the posts list.
	if code, _ := get(t, mux, "/en/posts/tending"); code != 404 {
		t.Errorf("garden page as a post: %d", code)
	}
	if code, _ := get(t, mux, "/en/garden/zfs-scrub"); code != 404 {
		t.Errorf("runbook in the garden: %d", code)
	}
	if _, body := get(t, mux, "/en/posts"); strings.Contains(body, "On tending notes") {
		t.Error("garden page in the posts list")
	}
}

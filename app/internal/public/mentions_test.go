package public

import (
	"context"
	"strings"
	"testing"

	"halid.org/engine/internal/content/post"
)

// Approved webmentions show under the post: replies and mentions as cards,
// likes counted; pending and rejected ones do not. Every page names the
// endpoint.
func TestMentions(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db)
	ctx := context.Background()
	p, err := post.NewRepo(db).GetPublished(ctx, "en", "htmx-admin")
	if err != nil {
		t.Fatal(err)
	}
	_, body := get(t, mux, "/en/posts/htmx-admin")
	must(t, body, `<link rel="webmention" href="/webmention">`)
	if strings.Contains(body, `id="mentions"`) {
		t.Error("mentions block shows with no mentions")
	}
	for i, m := range [][3]string{
		{"reply", "approved", "The 900 revisions detail is painfully familiar."},
		{"like", "approved", ""}, {"like", "approved", ""},
		{"reply", "pending", "Waiting <b>here</b>"},
		{"mention", "rejected", "Spam"},
	} {
		if _, err := db.Query(ctx, `INSERT INTO webmentions (id, target_entry_id, target_url, source_url, author, author_avatar, content, type, status, published_at, received_at, updated_at)
VALUES (?, ?, '', ?, 'Ada', ?, ?, ?, ?, 1792131120, 1, 1)`, "01WM0000000000000000000000"[:25]+string(rune('A'+i)), p.ID,
			"https://mastodon.social/@ada/"+string(rune('a'+i)), map[bool]string{true: "/avatars/0123456789abcdef01234567.png"}[i == 0], m[2], m[0], m[1]); err != nil {
			t.Fatal(err)
		}
	}
	_, body = get(t, mux, "/en/posts/htmx-admin")
	must(t, body, `<h3 class="mono">webmentions · 1 reply · 2 likes</h3>`,
		`<img class="wm-av u-photo" src="/avatars/0123456789abcdef01234567.png"`,
		`<a class="u-url" href="https://mastodon.social/@ada/a" rel="nofollow ugc noopener">mastodon.social</a> · <time>2026-10-16</time>`,
		`<p class="p-content">The 900 revisions detail is painfully familiar.</p>`)
	for _, not := range []string{"Waiting", "Spam"} {
		if strings.Contains(body, not) {
			t.Errorf("%q shows", not)
		}
	}
	_, body = get(t, mux, "/tr/posts/htmx-admin")
	must(t, body, `webmention&#39;lar · 1 yanıt · 2 beğeni`)
}

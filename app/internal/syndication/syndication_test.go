package syndication

import (
	"context"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func TestParseAndNetwork(t *testing.T) {
	got, err := Parse("https://x.com/halid/status/1\r\n\n  https://www.linkedin.com/posts/halid-1  \nhttps://x.com/halid/status/1\n")
	if err != nil || strings.Join(got, " ") != "https://x.com/halid/status/1 https://www.linkedin.com/posts/halid-1" {
		t.Fatalf("parse %q %v", got, err)
	}
	for _, bad := range []string{"x.com/halid", "javascript:alert(1)", "ftp://a.example/x"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if _, err := Parse(strings.Repeat("https://a.example/1\nhttps://a.example/2\n", 6)); err != nil {
		t.Errorf("duplicates counted: %v", err)
	}
	for raw, want := range map[string]string{
		"https://twitter.com/a/status/1": "x", "https://www.linkedin.com/feed/update/1": "linkedin",
		"https://tr.linkedin.com/posts/1": "linkedin", "https://bsky.app/profile/a/post/1": "bluesky",
		"https://news.ycombinator.com/item?id=1": "news.ycombinator.com",
	} {
		if got := Network(raw); got != want {
			t.Errorf("%s: %q", raw, got)
		}
	}
	if Label("x") != "X" || Label("news.ycombinator.com") != "news.ycombinator.com" {
		t.Error("labels")
	}
	s := NewShare("https://halid.org/en/posts/a b", "Hello & bye")
	if s.X != "https://x.com/intent/tweet?text=Hello%20%26%20bye&url=https%3A%2F%2Fhalid.org%2Fen%2Fposts%2Fa%20b" || !strings.HasSuffix(s.LinkedIn, "?url=https%3A%2F%2Fhalid.org%2Fen%2Fposts%2Fa%20b") {
		t.Errorf("share %+v", s)
	}
}

func TestSetAndList(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	e := content.Entry{Kind: "post", Slug: "a", Lang: "en", Title: "A", Status: content.Draft, Visibility: content.Public}
	if _, err := db.Batch(ctx, content.InsertEntry(&e, content.Body{MD: "x"}, content.OriginManual)...); err != nil {
		t.Fatal(err)
	}
	r := NewRepo(db)
	urls := []string{"https://x.com/a/status/1", "https://www.linkedin.com/posts/1"}
	if changed, err := r.Set(ctx, e.ID, urls); err != nil || !changed {
		t.Fatalf("set %v %v", changed, err)
	}
	if changed, _ := r.Set(ctx, e.ID, urls); changed {
		t.Error("same list reported as a change")
	}
	if changed, err := r.Set(ctx, e.ID, []string{urls[1], "https://bsky.app/p/1"}); err != nil || !changed {
		t.Fatalf("reset %v %v", changed, err)
	}
	got, _ := r.List(ctx, e.ID)
	if len(got) != 2 || got[0].Network != "linkedin" || got[1].Network != "bluesky" {
		t.Errorf("list %+v", got)
	}
	if _, err := r.Set(ctx, e.ID, nil); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.List(ctx, e.ID); len(got) != 0 {
		t.Errorf("cleared %+v", got)
	}
}

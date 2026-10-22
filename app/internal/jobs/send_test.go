package jobs_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/jobs"
)

// entry.changed queues webmention.send only when the published revision or
// URL differs from the last one sent, or a sent entry goes away.
func TestQueuesWebmentionSend(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	s, j, q := setup(t)
	ctx := context.Background()
	jobs.NewEntryJobs(s.DB, s.Search, s.Blob, jobs.NewWarmer(http.NewServeMux(), s.KV)).Register(j)
	posts := post.NewRepo(s.DB)
	p, err := posts.Create(ctx, post.Draft{Post: post.Post{Entry: content.Entry{Slug: "wm", Lang: "en", Title: "Links"}}, Markdown: "See [go](https://go.dev/)."})
	if err != nil {
		t.Fatal(err)
	}
	// run delivers entry.changed and returns the types it queued.
	run := func() []string {
		t.Helper()
		before := len(q.Sent)
		if err := jobs.Enqueue(ctx, j, p.ID, nil); err != nil {
			t.Fatal(err)
		}
		if code := deliver(j, q.Sent[len(q.Sent)-1], "1", true); code != 200 {
			t.Fatalf("job code %d", code)
		}
		var out []string
		for _, raw := range q.Sent[before+1:] {
			var m jobs.Message
			_ = json.Unmarshal(raw, &m)
			out = append(out, m.Type)
		}
		return out
	}
	if got := run(); len(got) != 0 {
		t.Fatalf("draft queued %v", got)
	}
	pub, err := posts.Publish(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := run(); len(got) != 1 || got[0] != jobs.TypeWebmentionSend {
		t.Fatalf("publish queued %v", got)
	}
	// As if the send ran for this revision: a plain save queues nothing.
	if _, err := s.DB.Query(ctx, `INSERT INTO webmention_sources (entry_id, source_url, revision_id, sent_at) VALUES (?, 'https://halid.org/en/posts/wm', ?, 1)`, p.ID, *pub.PublishedRevisionID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := posts.Save(ctx, post.Draft{Post: mustGet(t, posts, p.ID), Markdown: "Draft text."}, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	if got := run(); len(got) != 0 {
		t.Fatalf("save queued %v", got)
	}
	if _, err := posts.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if got := run(); len(got) != 1 {
		t.Fatalf("republish queued %v", got)
	}
}

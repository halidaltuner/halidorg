package jobs_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// drain delivers queued jobs until the queue is empty.
func drain(t *testing.T, j *jobs.Jobs, sent *[]json.RawMessage, from int) {
	t.Helper()
	for i := from; i < len(*sent); i++ {
		if code := deliver(j, (*sent)[i], "1", true); code != 200 {
			t.Fatalf("job %d: %d", i, code)
		}
	}
}

func searchRows(t *testing.T, db store.DB, where string) int {
	t.Helper()
	res, err := db.Query(context.Background(), `SELECT count(*) AS n FROM search_fts WHERE `+where)
	if err != nil {
		t.Fatal(err)
	}
	n, err := store.Scan[struct {
		N int `json:"n"`
	}](res)
	if err != nil {
		t.Fatal(err)
	}
	return n[0].N
}

// TestSearchRebuild: the rebuild empties the index, pages through every
// entry (more than one page here), and ends with photos and certificates.
func TestSearchRebuild(t *testing.T) {
	s, j, q := setup(t)
	ctx := context.Background()
	jobs.NewEntryJobs(s.DB, s.Search, s.Blob, jobs.NewWarmer(http.NewServeMux(), s.KV)).Register(j)
	posts := post.NewRepo(s.DB)
	for i := 0; i < 103; i++ {
		if _, err := posts.Create(ctx, post.Draft{Post: post.Post{Entry: content.Entry{Slug: fmt.Sprintf("p-%03d", i), Lang: "en", Title: fmt.Sprintf("Post %d", i)}}, Markdown: "Body."}); err != nil {
			t.Fatal(err)
		}
	}
	for _, st := range []string{
		`INSERT INTO search_fts (entry_id, kind, lang, status, visibility, slug, title, summary, body, tags) VALUES ('gone', 'post', 'en', 'published', 'public', 'gone', 'stale', '', '', '')`,
	} {
		if _, err := s.Search.Query(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	for _, st := range []string{
		`INSERT INTO media (id, r2_key, mime, created_at) VALUES ('m1', 'media/m1/a.jpg', 'image/jpeg', 1)`,
		`INSERT INTO photo_albums (id, slug, name_tr, name_en) VALUES ('a1', 'city', 'şehir', 'city')`,
		`INSERT INTO photos (id, album_id, media_id, title_tr, title_en, place, published, created_at, updated_at) VALUES ('ph1', 'a1', 'm1', 'Boğaz', '', 'İstanbul', 1, 1, 1)`,
		`INSERT INTO cert_issuers (id, slug, name, tone) VALUES ('i1', 'aws', 'Amazon Web Services', 'amber')`,
		`INSERT INTO certificates (id, issuer_id, name, issued_at, published, created_at, updated_at) VALUES ('c1', 'i1', 'Solutions Architect', 1, 1, 1, 1)`,
	} {
		if _, err := s.DB.Query(ctx, st); err != nil {
			t.Fatal(err)
		}
	}
	from := len(q.Sent)
	if err := jobs.EnqueueSearchRebuild(ctx, j); err != nil {
		t.Fatal(err)
	}
	drain(t, j, &q.Sent, from)
	if n := len(q.Sent) - from; n != 2 {
		t.Errorf("%d rebuild jobs, want 2 pages", n)
	}
	if n := searchRows(t, s.Search, `kind = 'post'`); n != 103 {
		t.Errorf("%d post rows, want 103", n)
	}
	if n := searchRows(t, s.Search, `entry_id = 'gone'`); n != 0 {
		t.Error("stale row survived the rebuild")
	}
	// A photo in both languages (the Turkish title for both), a certificate
	// by name and issuer.
	if n := searchRows(t, s.Search, `kind = 'photo' AND search_fts MATCH '"bogaz" "istanbul"'`); n != 2 {
		t.Errorf("photo rows %d", n)
	}
	if n := searchRows(t, s.Search, `kind = 'certificate' AND search_fts MATCH '"amazon"'`); n != 2 {
		t.Errorf("certificate rows %d", n)
	}

	// A gallery job alone replaces just the photo and certificate rows.
	if _, err := s.DB.Query(ctx, `UPDATE photos SET published = 0`); err != nil {
		t.Fatal(err)
	}
	from = len(q.Sent)
	if err := jobs.EnqueueSearchGallery(ctx, j); err != nil {
		t.Fatal(err)
	}
	drain(t, j, &q.Sent, from)
	if n := searchRows(t, s.Search, `kind = 'photo' AND status = 'draft'`); n != 2 {
		t.Errorf("unpublished photo rows %d", n)
	}
	if n := searchRows(t, s.Search, `kind = 'post'`); n != 103 {
		t.Errorf("gallery job touched posts: %d", n)
	}
}

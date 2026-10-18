package jobs_test

// Entry job tests live outside package jobs: they drive the post repository,
// and post imports jobs (its admin handlers enqueue).

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/content/post"
	"halid.org/engine/internal/content/project"
	"halid.org/engine/internal/content/snippet"
	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/render/og"
	"halid.org/engine/internal/search"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

func setup(t *testing.T) (store.Stores, *jobs.Jobs, *storetest.Queue) {
	t.Helper()
	s := storetest.Stores(t)
	q := s.Queue.(*storetest.Queue)
	return s, jobs.New(s.DB, q), q
}

func deliver(j *jobs.Jobs, msg json.RawMessage, attempt string, header bool) int {
	req := httptest.NewRequest(http.MethodPost, "/internal/jobs", bytes.NewReader(msg))
	if header {
		req.Header.Set(jobs.InternalHeader, "jobs")
	}
	req.Header.Set("X-Job-Attempt", attempt)
	rec := httptest.NewRecorder()
	mux := http.NewServeMux()
	j.Register(mux)
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// TestEntryChanged runs the publish flow end to end against the real
// schemas: publish enqueues, the job warms the affected pages and indexes
// the published text; a later delete purges and de-indexes.
func TestEntryChanged(t *testing.T) {
	s, j, q := setup(t)
	ctx := context.Background()
	kv := s.KV.(*storetest.KV)

	// A stand-in public site: home, and post pages for published posts.
	posts := post.NewRepo(s.DB)
	mux := http.NewServeMux()
	write := func(w http.ResponseWriter) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=0")
		_, _ = w.Write([]byte("<p>page</p>"))
	}
	mux.HandleFunc("GET /en/posts/{slug}", func(w http.ResponseWriter, r *http.Request) {
		if _, err := posts.GetPublished(r.Context(), "en", r.PathValue("slug")); err != nil {
			http.NotFound(w, r)
			return
		}
		write(w)
	})
	mux.HandleFunc("GET /en/{$}", func(w http.ResponseWriter, _ *http.Request) { write(w) })
	ej := jobs.NewEntryJobs(s.DB, s.Search, s.Blob, jobs.NewWarmer(mux, s.KV))
	ej.Register(j)

	p, err := posts.Create(ctx, post.Draft{
		Post:     post.Post{Entry: content.Entry{Slug: "fold", Lang: "en", Title: "Folding IŞIK"}},
		Markdown: "Searchable body about ışık.", Tags: []string{"search"},
	})
	if err != nil {
		t.Fatal(err)
	}
	prev, _ := jobs.RefOf(ctx, s.DB, p.ID)
	if _, err := posts.Publish(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	if err := jobs.Enqueue(ctx, j, p.ID, prev); err != nil {
		t.Fatal(err)
	}
	if code := deliver(j, q.Sent[0], "1", true); code != 200 {
		t.Fatalf("job code %d", code)
	}
	for _, k := range []string{"page:en:/en/posts/fold", "page:en:/en/"} {
		if _, ok := kv.Vals[k]; !ok {
			t.Errorf("missing %s; have %v", k, kv.Keys())
		}
	}
	hits := find(t, s.Search, "isik")
	if len(hits) != 1 || hits[0] != p.ID {
		t.Fatalf("search after publish = %v", hits)
	}
	blob := s.Blob.(*storetest.Blob)
	card, ok := blob.Objects["og/en/posts/fold.png"]
	if !ok || card.ContentType != "image/png" || !bytes.HasPrefix(card.Body, []byte("\x89PNG")) {
		t.Errorf("OG card missing or not a PNG; have %d objects", len(blob.Objects))
	}
	for _, l := range i18n.Supported {
		if _, ok := blob.Objects[og.SiteKey(l)]; !ok {
			t.Errorf("site card %s missing", og.SiteKey(l))
		}
	}

	// Edit after publish: public search still sees the published text.
	if _, _, err := posts.Save(ctx, post.Draft{Post: mustGet(t, posts, p.ID), Markdown: "Now about gölge only.", Tags: []string{"search"}}, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	_ = jobs.Enqueue(ctx, j, p.ID, nil)
	_ = deliver(j, q.Sent[1], "1", true)
	if hits := find(t, s.Search, "golge"); len(hits) != 0 {
		t.Errorf("unpublished edit is searchable: %v", hits)
	}

	// Delete: purge and de-index.
	prev, _ = jobs.RefOf(ctx, s.DB, p.ID)
	if err := posts.Delete(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	_ = jobs.Enqueue(ctx, j, p.ID, prev)
	if code := deliver(j, q.Sent[2], "1", true); code != 200 {
		t.Fatalf("delete job code %d", code)
	}
	if _, ok := kv.Vals["page:en:/en/posts/fold"]; ok {
		t.Error("deleted post still in KV")
	}
	if _, ok := blob.Objects["og/en/posts/fold.png"]; ok {
		t.Error("deleted post still has an OG card")
	}
	if hits := find(t, s.Search, "isik"); len(hits) != 0 {
		t.Errorf("deleted post still indexed: %v", hits)
	}
}

func mustGet(t *testing.T, r *post.Repo, id string) post.Post {
	t.Helper()
	p, err := r.Get(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func find(t *testing.T, db store.DB, q string) []string {
	t.Helper()
	st, ok := search.Parse(q).Stmt(search.Filter{})
	if !ok {
		return nil
	}
	res, err := db.Query(context.Background(), st.SQL, st.Args...)
	if err != nil {
		t.Fatal(err)
	}
	hits, _ := store.Scan[search.Hit](res)
	var ids []string
	for _, h := range hits {
		ids = append(ids, h.EntryID)
	}
	return ids
}

// TestBacklinks: publishing a post that links to another records the link,
// re-renders the target's page, and unpublishing clears it.
func TestBacklinks(t *testing.T) {
	s, j, q := setup(t)
	ctx := context.Background()
	kv := s.KV.(*storetest.KV)
	posts := post.NewRepo(s.DB)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /en/posts/{slug}", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "public, max-age=0")
		_, _ = w.Write([]byte("<p>page</p>"))
	})
	jobs.NewEntryJobs(s.DB, s.Search, s.Blob, jobs.NewWarmer(mux, s.KV)).Register(j)

	publish := func(slug, md string) string {
		p, err := posts.Create(ctx, post.Draft{Post: post.Post{Entry: content.Entry{Slug: slug, Lang: "en", Title: slug}}, Markdown: md})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := posts.Publish(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
		if err := jobs.Enqueue(ctx, j, p.ID, nil); err != nil {
			t.Fatal(err)
		}
		if code := deliver(j, q.Sent[len(q.Sent)-1], "1", true); code != 200 {
			t.Fatalf("job %d", code)
		}
		return p.ID
	}
	target := publish("target", "Plain.")
	delete(kv.Vals, "page:en:/en/posts/target")
	source := publish("source", "See [the target](/en/posts/target) and [itself](/en/posts/source).")

	bl, err := content.Backlinks(ctx, s.DB, target, true)
	if err != nil || len(bl) != 1 || bl[0].ID != source || bl[0].Context != "the target" {
		t.Fatalf("backlinks = %+v, %v", bl, err)
	}
	if _, ok := kv.Vals["page:en:/en/posts/target"]; !ok {
		t.Error("target page not re-rendered after a new backlink")
	}
	if self, _ := content.Backlinks(ctx, s.DB, source, false); len(self) != 0 {
		t.Errorf("self link recorded: %+v", self)
	}
}

// TestShortFormKinds: publishing a note, a snippet and a link renders a card
// for kinds with pages and indexes their kind fields (code, domain).
func TestShortFormKinds(t *testing.T) {
	s, j, q := setup(t)
	ctx := context.Background()
	jobs.NewEntryJobs(s.DB, s.Search, s.Blob, jobs.NewWarmer(http.NewServeMux(), s.KV)).Register(j)
	run := func(id string) {
		t.Helper()
		if err := jobs.Enqueue(ctx, j, id, nil); err != nil {
			t.Fatal(err)
		}
		if code := deliver(j, q.Sent[len(q.Sent)-1], "1", true); code != 200 {
			t.Fatalf("job %d", code)
		}
	}

	notes := note.NewRepo(s.DB)
	n, err := notes.Create(ctx, note.Draft{Note: note.Note{Entry: content.Entry{Slug: "til-fold", Lang: "en", Summary: "Folding ı"}}, Markdown: "Turkish dotless ı folds to i."})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notes.Publish(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	run(n.ID)

	snips := snippet.NewRepo(s.DB)
	sn, err := snips.Create(ctx, snippet.Draft{Snippet: snippet.Snippet{Entry: content.Entry{Slug: "shutdown", Lang: "en", Title: "graceful shutdown"},
		Files: []snippet.File{{Filename: "main.go", Code: "signal.NotifyContext(ctx, syscall.SIGTERM)"}}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := snips.Publish(ctx, sn.ID); err != nil {
		t.Fatal(err)
	}
	run(sn.ID)

	links := link.NewRepo(s.DB)
	l, err := links.Create(ctx, link.Draft{Link: link.Link{Entry: content.Entry{Slug: "grug", Lang: "en", Title: "Grug"}, URL: "https://grugbrain.dev/"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := links.Publish(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	run(l.ID)

	blob := s.Blob.(*storetest.Blob)
	for _, key := range []string{"og/en/notes/til-fold.png", "og/en/snippets/shutdown.png"} {
		if _, ok := blob.Objects[key]; !ok {
			t.Errorf("card %s missing", key)
		}
	}
	if _, ok := blob.Objects["og/en/links/grug.png"]; ok {
		t.Error("links have no pages and need no card")
	}
	for q, want := range map[string]string{"notifycontext": sn.ID, "s:sigterm": sn.ID, "grugbrain": l.ID, "l:grugbrain": l.ID, "n:dotless": n.ID} {
		if hits := find(t, s.Search, q); len(hits) != 1 || hits[0] != want {
			t.Errorf("search %q = %v, want %s", q, hits, want)
		}
	}
	if hits := find(t, s.Search, "n:grugbrain"); len(hits) != 0 {
		t.Errorf("n: prefix matched a link: %v", hits)
	}

	// Homelab builds are indexed under their own kind: h: finds them, pr: not.
	cat := "01K0000000000000000000HM01"
	projects := project.NewRepo(s.DB)
	for _, d := range []project.Draft{
		{Project: project.Project{Entry: content.Entry{Slug: "rack", Lang: "en", Title: "quiet rack"}, Type: project.Homelab, CategoryID: &cat}},
		{Project: project.Project{Entry: content.Entry{Slug: "radar", Lang: "en", Title: "quiet radar"}, Type: project.Software}},
	} {
		p, err := projects.Create(ctx, d)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := projects.Publish(ctx, p.ID); err != nil {
			t.Fatal(err)
		}
		run(p.ID)
	}
	if h, pr := find(t, s.Search, "h:quiet"), find(t, s.Search, "pr:quiet"); len(h) != 1 || len(pr) != 1 || h[0] == pr[0] {
		t.Errorf("h: %v, pr: %v", h, pr)
	}
}

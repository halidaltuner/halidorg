package jobs_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store/storetest"
)

type fakeIcons struct{ calls []string }

func (f *fakeIcons) Favicon(_ context.Context, host string) ([]byte, string, error) {
	f.calls = append(f.calls, host)
	if host == "noicon.dev" {
		return nil, "", errors.New("no favicon")
	}
	return []byte("\x00\x00\x01\x00ico"), "ico", nil
}

func TestLinkFavicons(t *testing.T) {
	s, j, q := setup(t)
	ctx := context.Background()
	icons := &fakeIcons{}
	jobs.NewEntryJobs(s.DB, s.Search, s.Blob, jobs.NewWarmer(http.NewServeMux(), s.KV)).WithFavicons(icons).Register(j)
	links := link.NewRepo(s.DB)
	publish := func(slug, url string) link.Link {
		l, err := links.Create(ctx, link.Draft{Link: link.Link{Entry: content.Entry{Slug: slug, Lang: "en", Title: slug}, URL: url}})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := links.Publish(ctx, l.ID); err != nil {
			t.Fatal(err)
		}
		if err := jobs.Enqueue(ctx, j, l.ID, nil); err != nil {
			t.Fatal(err)
		}
		if code := deliver(j, q.Sent[len(q.Sent)-1], "1", true); code != 200 {
			t.Fatalf("%s: job %d", slug, code)
		}
		got, _ := links.Get(ctx, l.ID)
		return got
	}
	a := publish("grug", "https://www.grugbrain.dev/")
	if a.FaviconURL != "/favicons/grugbrain.dev.ico" || len(icons.calls) != 1 || icons.calls[0] != "www.grugbrain.dev" {
		t.Fatalf("first link: %q, calls %v", a.FaviconURL, icons.calls)
	}
	obj, ok := s.Blob.(*storetest.Blob).Objects["favicons/grugbrain.dev.ico"]
	if !ok || obj.ContentType != "image/x-icon" {
		t.Fatalf("favicon not in R2: %+v", obj)
	}
	// Same domain: reused, not fetched again.
	if b := publish("grug-2", "https://grugbrain.dev/other"); b.FaviconURL != a.FaviconURL || len(icons.calls) != 1 {
		t.Errorf("second link: %q, calls %v", b.FaviconURL, icons.calls)
	}
	// No icon: the job still succeeds and the link keeps none.
	if c := publish("none", "https://noicon.dev/"); c.FaviconURL != "" {
		t.Errorf("no-icon link got %q", c.FaviconURL)
	}
}

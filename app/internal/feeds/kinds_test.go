package feeds

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/content/note"
	"halid.org/engine/internal/store/storetest"
)

func TestKindFeeds(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	ctx := context.Background()
	db := storetest.NewDB(t)
	mux := http.NewServeMux()
	New(db).Register(mux)

	notes := note.NewRepo(db)
	n, err := notes.Create(ctx, note.Draft{Note: note.Note{Entry: content.Entry{Slug: "til-1", Lang: "en", Summary: "Untitled TIL"}}, Markdown: "`go test -run`", Tags: []string{"go"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notes.Publish(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	links := link.NewRepo(db)
	l, err := links.Create(ctx, link.Draft{Link: link.Link{Entry: content.Entry{Slug: "grug", Lang: "en", Title: "Grug", Summary: "complexity <bad>"}, URL: "https://grugbrain.dev/"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := links.Publish(ctx, l.ID); err != nil {
		t.Fatal(err)
	}

	get := func(path string) (int, string, string) {
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
		return rec.Code, rec.Header().Get("Content-Type"), rec.Body.String()
	}
	code, ct, body := get("/en/notes/feed.xml")
	if code != 200 || !strings.HasPrefix(ct, "application/rss+xml") || !strings.Contains(body, "<link>https://halid.org/en/notes/til-1</link>") ||
		!strings.Contains(body, "<title>Untitled TIL</title>") || !strings.Contains(body, "halid.org · notes") {
		t.Fatalf("notes rss %d %s\n%s", code, ct, body)
	}
	if strings.Contains(body, "grugbrain") {
		t.Error("a link leaked into the notes feed")
	}
	_, _, body = get("/en/links/feed.json")
	var doc jsonFeedDoc
	if err := json.Unmarshal([]byte(body), &doc); err != nil || len(doc.Items) != 1 || doc.Items[0].URL != "https://grugbrain.dev/" ||
		doc.Items[0].ContentHTML != "<p>complexity &lt;bad&gt;</p>" || doc.FeedURL != "https://halid.org/en/links/feed.json" {
		t.Fatalf("links json %v %+v", err, doc)
	}
	if code, _, body := get("/en/snippets/atom.xml"); code != 200 || !strings.Contains(body, "halid.org · snippets") {
		t.Fatalf("empty snippets atom %d", code)
	}
	// Notes have pages: they and the list pages are in the sitemap; links
	// are not, and an uncategorized link makes no awesome page.
	if _, _, sm := get("/sitemap.xml"); !strings.Contains(sm, "<loc>https://halid.org/en/notes/til-1</loc>") ||
		!strings.Contains(sm, "<loc>https://halid.org/tr/snippets</loc>") || !strings.Contains(sm, "<loc>https://halid.org/tr/links</loc>") ||
		strings.Contains(sm, "grug") || strings.Contains(sm, "/awesome/") {
		t.Errorf("sitemap:\n%s", sm)
	}
	// The language feeds stay posts only.
	if _, _, body := get("/en/feed.xml"); strings.Contains(body, "til-1") || strings.Contains(body, "grugbrain") {
		t.Error("short-form entries in the posts feed")
	}
}

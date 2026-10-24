package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func TestWeeklyExport(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	ctx := context.Background()
	s := storetest.Stores(t)
	old := content.Clock
	content.Clock = func() time.Time { return time.Date(2026, 10, 25, 1, 41, 0, 0, time.UTC) }
	t.Cleanup(func() { content.Clock = old })

	at := int64(1792000000)
	e := content.Entry{Kind: "post", Slug: "hello", Lang: "en", Title: "Hello \"world\"", Summary: "Hi.", Status: content.Draft, Visibility: content.Public, PublishedAt: &at}
	stmts := content.InsertEntry(&e, content.Body{MD: "# Body\n\nText."}, content.OriginManual)
	if _, err := s.DB.Batch(ctx, stmts...); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO tags (id, slug, name_tr, name_en) VALUES ('t1', 'go', 'Go', 'Go')`,
		`INSERT INTO entry_tags (entry_id, tag_id) VALUES ('` + e.ID + `', 't1')`,
		`INSERT INTO media (id, r2_key, mime, alt_tr, alt_en, created_at, name, bytes) VALUES ('m1', 'media/m1/a.png', 'image/png', 'bir', 'one', 1, 'a.png', 42)`,
	} {
		if _, err := s.DB.Query(ctx, q); err != nil {
			t.Fatal(err)
		}
	}

	key, err := New(s.DB, s.Backups).Run(ctx)
	if err != nil || key != "exports/2026-10-25.zip" {
		t.Fatalf("run %q %v", key, err)
	}
	obj := s.Backups.(*storetest.Blob).Objects[key]
	if obj.ContentType != "application/zip" {
		t.Fatalf("content type %q", obj.ContentType)
	}
	z, err := zip.NewReader(bytes.NewReader(obj.Body), int64(len(obj.Body)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, f := range z.File {
		r, _ := f.Open()
		b, _ := io.ReadAll(r)
		files[f.Name] = string(b)
	}
	md := files["entries/en/post/hello.md"]
	for _, want := range []string{"---\nid: \"" + e.ID + "\"\n", `title: "Hello \"world\""`, `tags: ["go"]`, `published_at: "2026-10-14T17:46:40Z"`, "---\n# Body\n\nText.\n"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown missing %q:\n%s", want, md)
		}
	}
	var entries []map[string]any
	if err := json.Unmarshal([]byte(files["tables/entries.json"]), &entries); err != nil || len(entries) != 1 {
		t.Errorf("tables/entries.json: %v %d", err, len(entries))
	}
	if _, ok := files["tables/revisions.json"]; !ok {
		t.Error("revisions table missing")
	}
	if _, ok := files["tables/d1_migrations.json"]; ok {
		t.Error("migrations bookkeeping exported")
	}
	if !strings.Contains(files["media/manifest.json"], `"url": "https://halid.org/media/m1/a.png"`) || files["README.txt"] == "" {
		t.Errorf("manifest %s", files["media/manifest.json"])
	}
}

package snippet

import (
	"context"
	"errors"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

func TestSnippetFilesAndRevisions(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	r := NewRepo(db)
	link := "link"
	cat := content.Category{Kind: "snippet", Slug: "go", NameEN: "Go"}
	if err := content.CreateCategory(ctx, db, &cat); err != nil {
		t.Fatal(err)
	}
	wrong := content.Category{Kind: link, Slug: "go", NameEN: "Go links"}
	if err := content.CreateCategory(ctx, db, &wrong); err != nil {
		t.Fatal(err)
	}

	s, err := r.Create(ctx, Draft{Snippet: Snippet{Entry: content.Entry{Slug: "graceful", Lang: "en", Title: "go: graceful shutdown"}}})
	if err != nil {
		t.Fatal(err)
	}
	var v content.ValidationError
	if _, err := r.Publish(ctx, s.ID); !errors.As(err, &v) || v["files"] == "" {
		t.Fatalf("published without files: %v", err)
	}
	s.CategoryID = &wrong.ID
	if _, err := r.Save(ctx, Draft{Snippet: s}, content.OriginManual); !errors.As(err, &v) || v["category"] == "" {
		t.Fatalf("link category accepted for a snippet: %v", err)
	}
	s.CategoryID = &cat.ID
	s.Files = []File{{Filename: "main.go", Code: "package main\r\n"}, {Filename: "main.go", Code: "x"}}
	if _, err := r.Save(ctx, Draft{Snippet: s}, content.OriginManual); !errors.As(err, &v) || v["files.1.filename"] == "" {
		t.Fatalf("duplicate filename accepted: %v", err)
	}
	s.Files = []File{{Filename: "main.go", Code: "package main\r\n"}, {Filename: "Dockerfile", Code: "FROM scratch"}}
	got, err := r.Save(ctx, Draft{Snippet: s, Markdown: "Stop on SIGTERM."}, content.OriginManual)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Files) != 2 || got.Files[0].Language != "go" || got.Files[1].Language != "docker" || got.Files[0].Code != "package main\n" {
		t.Fatalf("files %+v", got.Files)
	}
	if _, err := r.Publish(ctx, s.ID); err != nil {
		t.Fatal(err)
	}

	// An edit after publishing changes the current files, not the public ones.
	got, _ = r.Get(ctx, s.ID)
	got.Files = []File{{Filename: "main.go", Code: "package main // v2\n"}}
	if _, err := r.Save(ctx, Draft{Snippet: got, Markdown: "v2"}, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	pub, err := r.GetPublished(ctx, "en", "graceful")
	if err != nil || len(pub.Files) != 2 || pub.Files[0].Code != "package main\n" {
		t.Fatalf("public files %+v %v", pub.Files, err)
	}
	cur, _ := r.Get(ctx, s.ID)
	rev, _ := content.GetRevision(ctx, db, *cur.CurrentRevisionID)
	if files := FilesOf(rev.DataJSON); len(files) != 1 || files[0].Code != "package main // v2\n" {
		t.Fatalf("revision snapshot %+v", files)
	}
	if err := r.Delete(ctx, s.ID); err != nil {
		t.Fatal(err)
	}
	res, _ := db.Query(ctx, `SELECT COUNT(*) AS n FROM snippet_files`)
	if rows, _ := store.Scan[struct {
		N int `json:"n"`
	}](res); len(rows) != 1 || rows[0].N != 0 {
		t.Error("files outlived the snippet")
	}
}

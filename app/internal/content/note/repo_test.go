package note

import (
	"context"
	"errors"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func TestNoteLifecycle(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	r := NewRepo(db)

	// Untitled is fine; an empty note cannot publish.
	n, err := r.Create(ctx, Draft{Note: Note{Entry: content.Entry{Slug: "til-1", Lang: "en"}, TIL: true}})
	if err != nil {
		t.Fatal(err)
	}
	var v content.ValidationError
	if _, err := r.Publish(ctx, n.ID); !errors.As(err, &v) || v["body"] == "" {
		t.Fatalf("empty note published: %v", err)
	}
	n.SourceURL = "http://insecure.example"
	if _, err := r.Save(ctx, Draft{Note: n, Markdown: "x"}, content.OriginManual); !errors.As(err, &v) || v["source_url"] == "" {
		t.Fatalf("http source accepted: %v", err)
	}
	n.SourceURL = "https://go.dev/blog"
	if _, err := r.Save(ctx, Draft{Note: n, Markdown: "`go test -run X` runs one test.", Tags: []string{"go"}}, content.OriginManual); err != nil {
		t.Fatal(err)
	}
	p, err := r.Publish(ctx, n.ID)
	if err != nil || p.Status != content.Published || p.PublishedAt == nil || !p.TIL || p.SourceURL != "https://go.dev/blog" {
		t.Fatalf("publish %+v %v", p, err)
	}
	if got, err := r.GetPublished(ctx, "en", "til-1"); err != nil || got.ID != n.ID {
		t.Fatalf("GetPublished %v", err)
	}
	if list, _ := r.List(ctx, "en", 10); len(list) != 1 {
		t.Fatalf("list %d", len(list))
	}
	// Same slug, same language: refused; another language is fine.
	if _, err := r.Create(ctx, Draft{Note: Note{Entry: content.Entry{Slug: "til-1", Lang: "en"}}}); !errors.As(err, &v) || v["slug"] == "" {
		t.Fatalf("duplicate slug: %v", err)
	}
	if _, err := r.Create(ctx, Draft{Note: Note{Entry: content.Entry{Slug: "til-1", Lang: "tr"}}}); err != nil {
		t.Fatal(err)
	}
	if err := r.Archive(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := r.GetPublished(ctx, "en", "til-1"); !errors.Is(err, content.ErrNotFound) {
		t.Fatal("archived note still public")
	}
	if err := r.Delete(ctx, n.ID); err != nil {
		t.Fatal(err)
	}
}

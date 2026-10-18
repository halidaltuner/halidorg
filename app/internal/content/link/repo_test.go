package link

import (
	"context"
	"errors"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func TestLinkLifecycle(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	r := NewRepo(db)
	cat := content.Category{Kind: Kind, Slug: "reading", NameTR: "okuma"}
	if err := content.CreateCategory(ctx, db, &cat); err != nil {
		t.Fatal(err)
	}
	var v content.ValidationError
	if _, err := r.Create(ctx, Draft{Link: Link{Entry: content.Entry{Slug: "grug", Lang: "en", Title: "Grug"}, URL: "http://grugbrain.dev"}}); !errors.As(err, &v) || v["url"] == "" {
		t.Fatalf("http url accepted: %v", err)
	}
	l, err := r.Create(ctx, Draft{Link: Link{Entry: content.Entry{Slug: "grug", Lang: "en", Title: "The Grug Brained Developer", Summary: "complexity very bad"},
		URL: "https://www.GrugBrain.dev/#grug-on-complexity", CategoryID: &cat.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if l.Domain != "grugbrain.dev" || l.ReadStatus != Unread {
		t.Fatalf("link %+v", l)
	}
	if _, err := r.Publish(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
	if err := r.SetRead(ctx, l.ID, true); err != nil {
		t.Fatal(err)
	}
	if got, _ := r.List(ctx, ListFilter{Lang: "en", ReadStatus: Read}); len(got) != 1 || got[0].ReadStatus != Read || got[0].Status != content.Published {
		t.Fatalf("read list %+v", got)
	}
	if got, _ := r.List(ctx, ListFilter{Lang: "en", ReadStatus: Unread}); len(got) != 0 {
		t.Fatalf("unread list %+v", got)
	}
	revs, _ := content.Revisions(ctx, db, l.ID, 10)
	if len(revs) != 1 {
		t.Errorf("marking read wrote a revision: %d", len(revs))
	}
	if err := r.Delete(ctx, l.ID); err != nil {
		t.Fatal(err)
	}
}

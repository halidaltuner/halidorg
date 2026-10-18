package fav

import (
	"context"
	"errors"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func TestFavLifecycle(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	r := NewRepo(db)
	var v content.ValidationError
	six := 6
	if _, err := r.Create(ctx, Draft{Fav: Fav{Entry: content.Entry{Slug: "ddia", Lang: "en", Title: "DDIA"}, Type: "tome", Rating: &six}}); !errors.As(err, &v) || v["fav_type"] == "" || v["rating"] == "" {
		t.Fatalf("bad type and rating accepted: %v", err)
	}
	five := 5
	f, err := r.Create(ctx, Draft{Fav: Fav{Entry: content.Entry{Slug: "ddia", Lang: "en", Title: "Designing Data-Intensive Applications"},
		Type: "book", Author: "Martin Kleppmann", Rating: &five}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.Publish(ctx, f.ID); err != nil {
		t.Fatal(err)
	}
	books, _ := r.List(ctx, "en", "book")
	tools, _ := r.List(ctx, "en", "tool")
	if len(books) != 1 || len(tools) != 0 || *books[0].Rating != 5 || books[0].Author != "Martin Kleppmann" {
		t.Fatalf("books %+v tools %+v", books, tools)
	}
	if all, _ := r.List(ctx, "en", ""); len(all) != 1 {
		t.Fatal("unfiltered list")
	}
}

package devseed

import (
	"context"
	"testing"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/link"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store/storetest"
)

// 4 posts, 3 notes, 2 snippets, 5 links, 4 favs.
const samplesTotal = 18

func TestSeedIsIdempotent(t *testing.T) {
	ctx := context.Background()
	s := storetest.Stores(t)
	j := jobs.New(s.DB, s.Queue)
	n, err := Seed(ctx, s.DB, j)
	if err != nil || n != samplesTotal {
		t.Fatalf("first seed: %d %v", n, err)
	}
	if got := len(s.Queue.(*storetest.Queue).Sent); got != samplesTotal {
		t.Errorf("queued %d jobs, want %d", got, samplesTotal)
	}
	n, err = Seed(ctx, s.DB, j)
	if err != nil || n != 0 {
		t.Fatalf("second seed: %d %v", n, err)
	}
	// Existing samples are re-rendered, not created again.
	if got := len(s.Queue.(*storetest.Queue).Sent); got != 2*samplesTotal {
		t.Errorf("queued %d jobs after reseed, want %d", got, 2*samplesTotal)
	}

	// Categories once, the child under its parent; read status as given.
	cats, _ := content.Categories(ctx, s.DB, link.Kind)
	if len(cats) != 3 || cats[1].ParentID == nil || *cats[1].ParentID != cats[0].ID {
		t.Fatalf("categories %+v", cats)
	}
	read, _ := link.NewRepo(s.DB).List(ctx, link.ListFilter{Lang: "en", ReadStatus: link.Read})
	if len(read) != 2 {
		t.Errorf("read links: %d", len(read))
	}
}

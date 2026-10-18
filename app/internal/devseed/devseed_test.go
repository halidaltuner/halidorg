package devseed

import (
	"context"
	"testing"

	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store/storetest"
)

func TestSeedIsIdempotent(t *testing.T) {
	s := storetest.Stores(t)
	j := jobs.New(s.DB, s.Queue)
	n, err := Seed(context.Background(), s.DB, j)
	if err != nil || n != 4 {
		t.Fatalf("first seed: %d %v", n, err)
	}
	if got := len(s.Queue.(*storetest.Queue).Sent); got != 4 {
		t.Errorf("queued %d jobs, want 4", got)
	}
	n, err = Seed(context.Background(), s.DB, j)
	if err != nil || n != 0 {
		t.Fatalf("second seed: %d %v", n, err)
	}
	// Existing samples are re-rendered, not created again.
	if got := len(s.Queue.(*storetest.Queue).Sent); got != 8 {
		t.Errorf("queued %d jobs after reseed, want 8", got)
	}
}

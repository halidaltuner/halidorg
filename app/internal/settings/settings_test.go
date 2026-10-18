package settings

import (
	"context"
	"testing"

	"halid.org/engine/internal/store/storetest"
)

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := storetest.NewDB(t)
	var h Homelab
	if err := Get(ctx, db, HomelabKey, &h); err != nil || h != (Homelab{}) {
		t.Fatalf("missing key: %+v %v", h, err)
	}
	for _, want := range []Homelab{{Nodes: "3", Idle: "38 W", Uptime: "99.9%"}, {Nodes: "4"}} {
		if err := Set(ctx, db, HomelabKey, want); err != nil {
			t.Fatal(err)
		}
		var got Homelab
		if err := Get(ctx, db, HomelabKey, &got); err != nil || got != want {
			t.Fatalf("got %+v %v, want %+v", got, err, want)
		}
	}
}

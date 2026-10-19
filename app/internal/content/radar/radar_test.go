package radar

import (
	"context"
	"errors"
	"math"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func marks(bs []Blip) map[string]string {
	out := map[string]string{}
	for _, b := range bs {
		m := "same"
		switch {
		case b.New:
			m = "new"
		case b.Moved:
			m = "moved from " + b.Prev
		case b.Prev == "":
			m = "first"
		}
		out[b.Name] = b.Ring + " " + m
	}
	return out
}

func eq(t *testing.T, what string, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: got %v, want %v", what, got, want)
		return
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s: %s = %q, want %q", what, k, got[k], v)
		}
	}
}

// TestEditions: new, moved and unchanged across three editions, and what
// freezing publishes.
func TestEditions(t *testing.T) {
	db := storetest.NewDB(t)
	ctx := context.Background()
	r := NewRepo(db)
	now := time.Unix(1_790_000_000, 0)
	old := content.Clock
	content.Clock = func() time.Time { now = now.Add(time.Minute); return now }
	t.Cleanup(func() { content.Clock = old })

	add := func(name, q, ring, tr, en string) Item {
		i, err := r.Create(ctx, Item{Name: name, Quadrant: q, Ring: ring, NoteTR: tr, NoteEN: en})
		if err != nil {
			t.Fatal(err)
		}
		return i
	}
	set := func(i *Item, ring string, off bool) {
		i.Ring, i.Off = ring, off
		x, err := r.Save(ctx, *i)
		if err != nil {
			t.Fatal(err)
		}
		*i = x
	}
	freeze := func(label string) Frozen {
		f, err := r.Freeze(ctx, label)
		if err != nil {
			t.Fatalf("freeze %s: %v", label, err)
		}
		return f
	}

	if _, err := r.Freeze(ctx, "2026-04"); err == nil {
		t.Error("froze an empty radar")
	}
	goItem := add("Go", "languages", "adopt", "Sunucu tarafı için varsayılan.", "Default for anything server-side.")
	zig := add("Zig", "languages", "hold", "", "Not yet.")
	ts := add("TypeScript", "languages", "adopt", "", "")
	if _, err := r.Edition(ctx, ""); !errors.Is(err, ErrNoEdition) {
		t.Errorf("edition before any freeze: %v", err)
	}
	f1 := freeze("2026-04")
	if len(f1.Entries) != 6 {
		t.Errorf("first freeze wrote %d entries, want 6", len(f1.Entries))
	}

	// Second edition: Zig and TypeScript move, Gleam is new, Go stays.
	set(&zig, "assess", false)
	set(&ts, "trial", false)
	add("Gleam", "languages", "assess", "", "Watching the BEAM story.")
	p, err := r.Pending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "pending", marks(p.Blips), map[string]string{"Go": "adopt same", "Zig": "assess moved from hold", "TypeScript": "trial moved from adopt", "Gleam": "assess new"})
	if _, err := r.Freeze(ctx, "2026-04"); err == nil {
		t.Error("froze the same label twice")
	}
	if _, err := r.Freeze(ctx, "2025-10"); err == nil {
		t.Error("froze an older label")
	}
	if _, err := r.Freeze(ctx, "2026-13"); err == nil {
		t.Error("froze a bad label")
	}
	freeze("2026-10")

	// Third: TypeScript leaves, Helix arrives, Gleam and Go stay.
	set(&ts, "trial", true)
	add("Helix", "tools", "adopt", "", "")
	freeze("2027-04")

	v1, err := r.Edition(ctx, "2026-04")
	if err != nil {
		t.Fatal(err)
	}
	eq(t, "2026-04", marks(v1.Blips), map[string]string{"Go": "adopt first", "Zig": "hold first", "TypeScript": "adopt first"})
	v2, _ := r.Edition(ctx, "2026-10")
	eq(t, "2026-10", marks(v2.Blips), map[string]string{"Go": "adopt same", "Zig": "assess moved from hold", "TypeScript": "trial moved from adopt", "Gleam": "assess new"})
	v3, _ := r.Edition(ctx, "")
	if v3.Edition.Label != "2027-04" || len(v3.Editions) != 3 {
		t.Errorf("latest = %s of %d", v3.Edition.Label, len(v3.Editions))
	}
	eq(t, "2027-04", marks(v3.Blips), map[string]string{"Go": "adopt same", "Zig": "assess same", "Gleam": "assess same", "Helix": "adopt new"})
	if len(v3.Dropped) != 1 || v3.Dropped[0].Name != "TypeScript" {
		t.Errorf("dropped = %+v", v3.Dropped)
	}
	// Listed by ring, then quadrant, then name.
	if v3.Blips[0].Name != "Go" || v3.Blips[1].Name != "Helix" {
		t.Errorf("order: %s, %s", v3.Blips[0].Name, v3.Blips[1].Name)
	}

	// The entries carry the name and the note in their language.
	res, err := db.Query(ctx, `SELECT lang, slug, title, summary, status FROM entries WHERE kind = 'radar_item' AND translation_group_id = ? ORDER BY lang`, goItem.Group)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 || string(res.Rows[1]) != `{"lang":"tr","slug":"go","status":"published","summary":"Sunucu tarafı için varsayılan.","title":"Go"}` {
		t.Errorf("go entries: %s", res.Rows)
	}

	// TypeScript's history: on, moved, then off.
	h, err := r.HistoryOf(ctx, ts.Group, "en")
	if err != nil {
		t.Fatal(err)
	}
	if len(h) != 3 || !h[0].Off || h[0].Label != "2027-04" || h[1].Ring != "trial" || h[1].Prev != "adopt" || h[2].Ring != "adopt" || h[2].Prev != "" {
		t.Errorf("history = %+v", h)
	}
	// Helix has only the edition it joined.
	hx, _ := r.HistoryOf(ctx, v3.Blips[1].Group, "en")
	if len(hx) != 1 {
		t.Errorf("helix history = %+v", hx)
	}

	// Deleting an item keeps the editions it was on.
	if err := r.Delete(ctx, zig.Group); err != nil {
		t.Fatal(err)
	}
	if ids, _ := r.EntryIDs(ctx, zig.Group); len(ids) != 0 {
		t.Errorf("zig entries left: %v", ids)
	}
	v2, _ = r.Edition(ctx, "2026-10")
	if marks(v2.Blips)["Zig"] != "assess moved from hold" {
		t.Error("deleting an item changed an old edition")
	}
	if err := r.Delete(ctx, zig.Group); !errors.Is(err, content.ErrNotFound) {
		t.Errorf("second delete: %v", err)
	}
}

func TestCreateValidatesAndSlugs(t *testing.T) {
	db := storetest.NewDB(t)
	ctx := context.Background()
	r := NewRepo(db)
	if _, err := r.Create(ctx, Item{Name: "", Quadrant: "languages", Ring: "adopt"}); err == nil {
		t.Error("no name")
	}
	if _, err := r.Create(ctx, Item{Name: "Go", Quadrant: "frameworks", Ring: "adopt"}); err == nil {
		t.Error("unknown quadrant")
	}
	a, err := r.Create(ctx, Item{Name: "SQLite + Litestream", Quadrant: "platforms", Ring: "trial"})
	if err != nil || a.Slug != "sqlite-litestream" {
		t.Fatalf("slug %q, %v", a.Slug, err)
	}
	b, err := r.Create(ctx, Item{Name: "SQLite Litestream", Quadrant: "platforms", Ring: "trial"})
	if err != nil || b.Slug != "sqlite-litestream-2" {
		t.Errorf("second slug %q, %v", b.Slug, err)
	}
	// A rename keeps the slug.
	a.Name = "Litestream"
	if a, err = r.Save(ctx, a); err != nil || a.Slug != "sqlite-litestream" || a.Name != "Litestream" {
		t.Errorf("rename: %+v %v", a, err)
	}
}

func TestNextLabel(t *testing.T) {
	now := time.Date(2026, 10, 19, 0, 0, 0, 0, time.UTC)
	for _, c := range []struct {
		latest, want string
	}{{"", "2026-10"}, {"2026-04", "2026-10"}, {"2026-10", "2026-11"}, {"2026-12", "2027-01"}} {
		var eds []Edition
		if c.latest != "" {
			eds = []Edition{{Label: c.latest}}
		}
		if got := nextLabel(now, eds); got != c.want {
			t.Errorf("after %q: %s, want %s", c.latest, got, c.want)
		}
	}
}

// TestLayout: every blip lands inside its quadrant and ring, none overlap,
// and a blip keeps its spot when others join.
func TestLayout(t *testing.T) {
	for run := 0; run < 10; run++ {
		layoutOnce(t)
		if t.Failed() {
			return
		}
	}
}

func layoutOnce(t *testing.T) {
	var bs []Blip
	n := 0
	for qi, q := range Quadrants {
		for ri, r := range Rings {
			// The inner rings hold fewer: adopt has room for three a quadrant.
			count := 3 + ri + qi%2
			if ri == 0 {
				count = 3
			}
			for k := 0; k < count; k++ {
				n++
				bs = append(bs, Blip{Group: content.NewID(), Quadrant: q, Ring: r})
			}
		}
	}
	pts := Layout(bs)
	if len(pts) != n {
		t.Fatalf("%d points for %d blips", len(pts), n)
	}
	for i, b := range bs {
		p := pts[b.Group]
		dx, dy := p.X-Center, p.Y-Center
		rad := math.Hypot(dx, dy)
		ri := RingIndex(b.Ring)
		if rad < RingRadii[ri] || rad > RingRadii[ri+1] {
			t.Errorf("%s/%s at radius %.1f", b.Quadrant, b.Ring, rad)
		}
		// languages top right, techniques top left, tools bottom left, platforms bottom right
		right, top := dx > 0, dy < 0
		want := map[string][2]bool{"languages": {true, true}, "techniques": {false, true}, "tools": {false, false}, "platforms": {true, false}}[b.Quadrant]
		if right != want[0] || top != want[1] {
			t.Errorf("%s blip at (%.0f, %.0f)", b.Quadrant, p.X, p.Y)
		}
		for _, o := range bs[i+1:] {
			if q := pts[o.Group]; math.Hypot(p.X-q.X, p.Y-q.Y) < 16 {
				t.Errorf("overlap at (%.0f, %.0f)", p.X, p.Y)
			}
		}
	}
	// Same input, same picture.
	again := Layout(bs)
	for g, p := range pts {
		if again[g] != p {
			t.Fatal("layout is not deterministic")
		}
	}
	more := append(append([]Blip{}, bs...), Blip{Group: content.NewID(), Quadrant: "tools", Ring: "hold"})
	moved := 0
	for g, p := range Layout(more) {
		if old, ok := pts[g]; ok && old != p {
			moved++
		}
	}
	// Only blips it displaces (and theirs) step aside; most stay put.
	if moved > len(bs)/5 {
		t.Errorf("one new blip moved %d of %d others", moved, len(bs))
	}
}

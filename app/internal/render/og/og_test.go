package og

import (
	"bytes"
	"flag"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/*.png")

var cases = map[string]Card{
	"short": {Kind: "post", Title: "Notes on an htmx-first admin", Date: "2026-10-16", Meta: "halid.org · 12 min"},
	"long": {Kind: "post", Title: "Running a personal publishing engine on Cloudflare Containers with Go, D1, R2, KV and Queues, rendered on publish and served from the edge",
		Date: "2026-10-16", Meta: "halid.org · 18 min"},
	"tr": {Kind: "note", Title: "Ğüşıöç İstanbul'da ışık, gölge ve ölçüm", Date: "2026-10-18", Meta: "halid.org · 4 dk"},
}

// TestSnapshots compares each card with its golden PNG. Glyph rasterizing
// can differ by a few levels between CPUs (FMA), so pixels may drift
// slightly; a layout change moves thousands of pixels and fails.
func TestSnapshots(t *testing.T) {
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			got, err := Render(c)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join("testdata", name+".png")
			if *update {
				if err := os.WriteFile(path, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (go test ./internal/render/og/ -args -update)", err)
			}
			a, b := decode(t, got), decode(t, want)
			if a.Bounds() != b.Bounds() || a.Bounds().Dx() != Width || a.Bounds().Dy() != Height {
				t.Fatalf("size %v, want %v", a.Bounds(), b.Bounds())
			}
			if n := differing(a, b, 24); n > Width*Height/500 {
				t.Errorf("%d pixels differ from %s (after a deliberate change: go test -args -update, then bump Version)", n, path)
			}
		})
	}
}

func decode(t *testing.T, b []byte) image.Image {
	t.Helper()
	img, err := png.Decode(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func differing(a, b image.Image, tol int) int {
	n := 0
	r := a.Bounds()
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			r1, g1, b1, _ := a.At(x, y).RGBA()
			r2, g2, b2, _ := b.At(x, y).RGBA()
			if abs(int(r1>>8)-int(r2>>8)) > tol || abs(int(g1>>8)-int(g2>>8)) > tol || abs(int(b1>>8)-int(b2>>8)) > tol {
				n++
			}
		}
	}
	return n
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func titleFace(t *testing.T) face {
	t.Helper()
	fs, err := loadFonts()
	if err != nil {
		t.Fatal(err)
	}
	f, err := newFace(fs.sans600, titleSize, titleTrack)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func TestWrap(t *testing.T) {
	f := titleFace(t)
	max := px(titleMax)

	long := wrap(f, cases["long"].Title, max, titleLines)
	if len(long) != titleLines || !strings.HasSuffix(long[2], "…") {
		t.Errorf("long title: %q, want 3 lines ending in …", long)
	}
	for _, l := range long {
		if f.width(l) > max {
			t.Errorf("line %q is %v wide, over %v", l, f.width(l), max)
		}
	}

	// Balance: two lines of similar width instead of a full line and a widow.
	short := wrap(f, cases["short"].Title, max, titleLines)
	if len(short) != 2 {
		t.Fatalf("short title: %q", short)
	}
	if d := f.width(short[0]) - f.width(short[1]); d < -px(260) || d > px(260) {
		t.Errorf("unbalanced: %q", short)
	}

	// One word wider than the title box is broken between characters.
	word := wrap(f, strings.Repeat("ş", 60), max, titleLines)
	for _, l := range word {
		if f.width(l) > max {
			t.Errorf("unbroken word line %q", l)
		}
	}

	if got := wrap(f, "  ", max, titleLines); got != nil {
		t.Errorf("blank title: %q", got)
	}
}

func TestHash(t *testing.T) {
	c := cases["short"]
	again := cases["short"]
	if c.Hash() != again.Hash() || len(c.Hash()) != 12 {
		t.Fatalf("hash %q", c.Hash())
	}
	d := c
	d.Title += "!"
	if d.Hash() == c.Hash() {
		t.Error("title change kept the hash")
	}
}

func TestNoKindRow(t *testing.T) {
	if _, err := Render(Card{Title: "halid.org", Meta: "halid.org"}); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkRender(b *testing.B) {
	for b.Loop() {
		if _, err := Render(cases["long"]); err != nil {
			b.Fatal(err)
		}
	}
}

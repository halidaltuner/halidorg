package presentation

import (
	"strings"
	"testing"
)

func TestOutlineByClass(t *testing.T) {
	deck := `<!doctype html><html><head><style>.slide h2 { color: red } /* <section> */</style>
<script>const s = "<section class='slide'><h1>not a slide</h1></section>";</script></head><body>
<!-- <div class="slide"><h1>commented out</h1></div> -->
<div class="slide title"><h1>htmx in <em>production</em></h1><p>GoTürkiye</p></div>
<div class='slide'><div class="cols"><div><h2>Why leave the SPA</h2></div></div><h2>second heading</h2></div>
<div class=slide><p>no heading</p></div>
<div class="slide"><h3>Fish &amp; chips
  and   more</h3><section class="slide"><h2>nested, not counted</h2></section></div>
<section><h2>a plain section is not a slide here</h2></section>
</body></html>`
	n, out := Outline([]byte(deck))
	if n != 4 {
		t.Errorf("count %d", n)
	}
	want := []Slide{{1, "htmx in production"}, {2, "Why leave the SPA"}, {4, "Fish & chips and more"}}
	if len(out) != len(want) {
		t.Fatalf("outline %+v", out)
	}
	for i := range want {
		if out[i] != want[i] {
			t.Errorf("%d: %+v, want %+v", i, out[i], want[i])
		}
	}
}

func TestOutlineBySection(t *testing.T) {
	deck := `<html><body><div class="reveal"><div class="slides">
<section><h1>One</h1></section>
<section><section><h2>Two a</h2></section><section><h2>Two b</h2></section></section>
<section data-x="1"><h2>` + strings.Repeat("x", 200) + `</h2></section>
</div></div></body></html>`
	n, out := Outline([]byte(deck))
	if n != 3 || len(out) != 3 || out[1] != (Slide{2, "Two a"}) || out[2].N != 3 || len([]rune(out[2].Title)) != 120 {
		t.Fatalf("%d %+v", n, out)
	}
	if n, out := Outline([]byte(`<html><body><p>just a page</p></body></html>`)); n != 0 || out != nil {
		t.Errorf("no slides: %d %+v", n, out)
	}
}

func TestOutlineNeverPanics(t *testing.T) {
	deck := []byte(`<div class="slide"><h1>a</h1></div><section><!-- x <h2>`)
	for i := range deck {
		Outline(deck[:i])
	}
	Outline([]byte("<script>"))
	Outline([]byte("<style>body{}"))
	Outline([]byte{0xff, '<', 'h', '1', '>'})
}

func TestCheckDeck(t *testing.T) {
	if err := CheckDeck([]byte("<!doctype html><title>x</title>")); err != nil {
		t.Errorf("html: %v", err)
	}
	for _, bad := range [][]byte{nil, []byte("%PDF-1.4"), []byte("plain text"), {0xff, 0xfe, '<', 'h'}} {
		if err := CheckDeck(bad); err != ErrDeckType {
			t.Errorf("%q: %v", bad, err)
		}
	}
	if err := CheckDeck(make([]byte, MaxDeckBytes+1)); err != ErrDeckLarge {
		t.Errorf("large: %v", err)
	}
}

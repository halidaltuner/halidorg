package diff

import (
	"math/rand"
	"reflect"
	"strings"
	"testing"
)

// apply rebuilds b from a and the script, proving the script is correct.
func apply(a, b []string, ops []Op) []string {
	var out []string
	for _, op := range ops {
		switch op.Kind {
		case Equal:
			out = append(out, a[op.A])
		case Insert:
			out = append(out, b[op.B])
		}
	}
	return out
}

func TestLinesRebuilds(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	words := []string{"a", "b", "c", "d", "e"}
	gen := func() []string {
		n := r.Intn(12)
		s := make([]string, n)
		for i := range s {
			s[i] = words[r.Intn(len(words))]
		}
		return s
	}
	for i := 0; i < 500; i++ {
		a, b := gen(), gen()
		ops := Lines(a, b)
		got := apply(a, b, ops)
		if len(got) == 0 {
			got = nil
		}
		want := b
		if len(want) == 0 {
			want = nil
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("a=%v b=%v ops=%v got %v", a, b, ops, got)
		}
		// Deleted lines plus kept lines must cover a, in order.
		var seen []int
		for _, op := range ops {
			if op.A >= 0 {
				seen = append(seen, op.A)
			}
		}
		for j, x := range seen {
			if x != j {
				t.Fatalf("a not covered in order: %v", ops)
			}
		}
	}
}

func TestLinesIsMinimal(t *testing.T) {
	a := strings.Split("a b c a b b a", " ")
	b := strings.Split("c b a b a c", " ")
	edits := 0
	for _, op := range Lines(a, b) {
		if op.Kind != Equal {
			edits++
		}
	}
	if edits != 5 { // the classic Myers example: D = 5
		t.Errorf("edits = %d, want 5", edits)
	}
}

func TestSideBySide(t *testing.T) {
	rows := SideBySide("# Title\nold line\nkept\nremoved\n", "# Title\nnew line\nkept\nadded one\nadded two\n")
	want := []Row{
		{Kind: "eq", LNo: 1, RNo: 1, Left: "# Title", Right: "# Title"},
		{Kind: "chg", LNo: 2, RNo: 2, Left: "old line", Right: "new line"},
		{Kind: "eq", LNo: 3, RNo: 3, Left: "kept", Right: "kept"},
		{Kind: "chg", LNo: 4, RNo: 4, Left: "removed", Right: "added one"},
		{Kind: "add", RNo: 5, Right: "added two"},
	}
	if !reflect.DeepEqual(rows, want) {
		t.Errorf("got %+v", rows)
	}
	if a, r := Stats(rows); a != 3 || r != 2 {
		t.Errorf("stats +%d -%d", a, r)
	}
	if rows := SideBySide("", ""); rows != nil {
		t.Errorf("empty: %+v", rows)
	}
}

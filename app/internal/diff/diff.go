// Package diff compares two texts line by line (Myers' O(ND) algorithm) and
// lays the result out side by side for the admin revisions view.
package diff

import "strings"

// Kind is what happened to a line.
type Kind int

// Line kinds.
const (
	Equal Kind = iota
	Delete
	Insert
)

// Op is one line of an edit script: A and B are 0-based indexes into the
// old and new lines, -1 where the side has no line.
type Op struct {
	Kind Kind
	A, B int
}

// Lines returns the shortest edit script turning a into b.
func Lines(a, b []string) []Op {
	n, m := len(a), len(b)
	max := n + m
	if max == 0 {
		return nil
	}
	off := max
	v := make([]int, 2*max+2)
	var trace [][]int
	for d := 0; d <= max; d++ {
		trace = append(trace, append([]int(nil), v...))
		for k := -d; k <= d; k += 2 {
			var x int
			if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
				x = v[off+k+1] // down: insert
			} else {
				x = v[off+k-1] + 1 // right: delete
			}
			y := x - k
			for x < n && y < m && a[x] == b[y] {
				x, y = x+1, y+1
			}
			v[off+k] = x
			if x >= n && y >= m {
				return backtrack(trace, a, b, off, d)
			}
		}
	}
	return nil // unreachable: d = n+m always finishes
}

func backtrack(trace [][]int, a, b []string, off, d int) []Op {
	x, y := len(a), len(b)
	var rev []Op
	for ; d > 0; d-- {
		v := trace[d]
		k := x - y
		var pk int
		if k == -d || (k != d && v[off+k-1] < v[off+k+1]) {
			pk = k + 1
		} else {
			pk = k - 1
		}
		px := v[off+pk]
		py := px - pk
		for x > px && y > py {
			x, y = x-1, y-1
			rev = append(rev, Op{Equal, x, y})
		}
		if x == px {
			y--
			rev = append(rev, Op{Insert, -1, y})
		} else {
			x--
			rev = append(rev, Op{Delete, x, -1})
		}
	}
	for x > 0 && y > 0 {
		x, y = x-1, y-1
		rev = append(rev, Op{Equal, x, y})
	}
	out := make([]Op, len(rev))
	for i, op := range rev {
		out[len(rev)-1-i] = op
	}
	return out
}

// Row is one row of a side-by-side view. A line number is 0 where that
// side is empty.
type Row struct {
	Kind        string // "eq", "del", "add", "chg"
	LNo, RNo    int
	Left, Right string
}

// Split splits text into lines; a final newline does not add an empty line.
func Split(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// SideBySide pairs deletions with the insertions that follow them as
// changed rows, so an edited line sits next to its new version.
func SideBySide(oldText, newText string) []Row {
	a, b := Split(oldText), Split(newText)
	ops := Lines(a, b)
	var rows []Row
	for i := 0; i < len(ops); {
		if ops[i].Kind == Equal {
			rows = append(rows, Row{Kind: "eq", LNo: ops[i].A + 1, RNo: ops[i].B + 1, Left: a[ops[i].A], Right: b[ops[i].B]})
			i++
			continue
		}
		var dels, ins []Op
		for i < len(ops) && ops[i].Kind == Delete {
			dels = append(dels, ops[i])
			i++
		}
		for i < len(ops) && ops[i].Kind == Insert {
			ins = append(ins, ops[i])
			i++
		}
		for j := 0; j < len(dels) || j < len(ins); j++ {
			r := Row{}
			if j < len(dels) {
				r.LNo, r.Left = dels[j].A+1, a[dels[j].A]
			}
			if j < len(ins) {
				r.RNo, r.Right = ins[j].B+1, b[ins[j].B]
			}
			switch {
			case r.LNo > 0 && r.RNo > 0:
				r.Kind = "chg"
			case r.LNo > 0:
				r.Kind = "del"
			default:
				r.Kind = "add"
			}
			rows = append(rows, r)
		}
	}
	return rows
}

// Stats counts added and removed lines.
func Stats(rows []Row) (added, removed int) {
	for _, r := range rows {
		switch r.Kind {
		case "add":
			added++
		case "del":
			removed++
		case "chg":
			added++
			removed++
		}
	}
	return
}

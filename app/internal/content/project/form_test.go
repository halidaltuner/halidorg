package project

import "testing"

func TestMoney(t *testing.T) {
	for in, want := range map[string]int64{"27000": 2700000, "27.000": 2700000, "149,90": 14990, "1.234,56": 123456,
		"1,234.56": 123456, "₺ 3.900": 390000, "0,5": 50, "12.5": 1250} {
		if got, ok := ParseMoney(in); !ok || got != want {
			t.Errorf("ParseMoney(%q) = %d %v, want %d", in, got, ok, want)
		}
	}
	for _, bad := range []string{"", "abc", "-5", "1e5"} {
		if _, ok := ParseMoney(bad); ok {
			t.Errorf("ParseMoney(%q) accepted", bad)
		}
	}
	for v, want := range map[int64]string{2700000: "27000", 14990: "149.90", 5: "0.05"} {
		if got := FormatMoney(&v); got != want {
			t.Errorf("FormatMoney(%d) = %q", v, got)
		}
	}
	m := ParseMonth("2025-11")
	if m == nil || FormatMonth(m) != "2025-11" || ParseMonth("nope") != nil {
		t.Errorf("month %v", m)
	}
}

func TestCompactMoney(t *testing.T) {
	for _, c := range []struct {
		minor int64
		cur   string
		want  string
	}{{4120000, "TRY", "₺41.2k"}, {270000000, "TRY", "₺2.7M"}, {14990, "EUR", "€149.90"}, {98000, "CHF", "980 CHF"}} {
		if got := CompactMoney(c.minor, c.cur); got != c.want {
			t.Errorf("CompactMoney(%d, %s) = %q, want %q", c.minor, c.cur, got, c.want)
		}
	}
}

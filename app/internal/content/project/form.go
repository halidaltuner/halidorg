package project

import (
	"strconv"
	"strings"
	"time"
)

// ParseMoney reads an amount as typed ("27000", "27.000", "149,90",
// "1.234,56", "1,234.56") into minor units. The last '.' or ',' followed by
// one or two digits is the decimal mark; any other is a thousands mark.
func ParseMoney(s string) (int64, bool) {
	s = strings.Map(func(r rune) rune {
		if r == ' ' || r == ' ' || r == '₺' || r == '$' || r == '€' {
			return -1
		}
		return r
	}, s)
	if s == "" {
		return 0, false
	}
	whole, frac := s, ""
	if i := strings.LastIndexAny(s, ".,"); i >= 0 && len(s)-i-1 <= 2 && len(s)-i-1 > 0 {
		whole, frac = s[:i], s[i+1:]
	}
	whole = strings.NewReplacer(".", "", ",", "").Replace(whole)
	if whole == "" {
		whole = "0"
	}
	if len(frac) == 1 {
		frac += "0"
	}
	if frac == "" {
		frac = "00"
	}
	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w < 0 || w > 1e13 {
		return 0, false
	}
	f, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, false
	}
	return w*100 + f, true
}

// FormatMoney writes minor units back for a form field: "27000", "149.90".
// A negative value (an amount that did not parse) shows as empty.
func FormatMoney(v *int64) string {
	if v == nil || *v < 0 {
		return ""
	}
	s := strconv.FormatInt(*v/100, 10)
	if c := *v % 100; c != 0 {
		s += "." + strconv.FormatInt(100+c, 10)[1:]
	}
	return s
}

// ParseMonth reads a month field ("2025-11") as the first day, unix UTC.
func ParseMonth(s string) *int64 {
	t, err := time.Parse("2006-01", strings.TrimSpace(s))
	if err != nil {
		return nil
	}
	u := t.Unix()
	return &u
}

// FormatMonth writes unix seconds back for a month field.
func FormatMonth(v *int64) string {
	if v == nil {
		return ""
	}
	return time.Unix(*v, 0).UTC().Format("2006-01")
}

// ParseDay reads a date field ("2026-10-09") as midnight, unix UTC.
func ParseDay(s string) int64 {
	t, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return t.Unix()
}

// FormatDay writes unix seconds back for a date field.
func FormatDay(v int64) string {
	if v <= 0 {
		return ""
	}
	return time.Unix(v, 0).UTC().Format("2006-01-02")
}

var symbols = map[string]string{"TRY": "₺", "EUR": "€", "USD": "$", "GBP": "£"}

// CompactMoney is an amount for the parts list and the fact strip:
// "₺27.0k", "₺1.2M", "€149.90", "980 CHF".
func CompactMoney(minor int64, currency string) string {
	whole := float64(minor) / 100
	var s string
	switch {
	case whole >= 1e6:
		s = strconv.FormatFloat(whole/1e6, 'f', 1, 64) + "M"
	case whole >= 1e3:
		s = strconv.FormatFloat(whole/1e3, 'f', 1, 64) + "k"
	case minor%100 != 0:
		s = strconv.FormatFloat(whole, 'f', 2, 64)
	default:
		s = strconv.FormatInt(minor/100, 10)
	}
	if sym, ok := symbols[currency]; ok {
		return sym + s
	}
	return s + " " + currency
}

// Package search builds the documents and queries for the FTS5 index that
// lives in the separate SEARCH database (ADR-010). The same Fold runs at
// index and query time, so both sides tokenize Turkish text identically.
package search

import (
	"strings"
	"unicode"
)

// Fold lowercases s for indexing and querying. The dotted and dotless i
// family (I, İ, ı, i) all become "i": unicode61 lowercases I to i and leaves
// ı and İ alone, so "IŞIK", "ışık" and "Işık" would otherwise index as three
// different words. Other diacritics (ş, ğ, ç, ö, ü) are left for the
// tokenizer's remove_diacritics 2 to strip.
func Fold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case 'I', 'İ', 'ı':
			b.WriteRune('i')
		case '̇': // combining dot above, left behind by decomposed İ
		default:
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

package search

import (
	"context"
	"testing"

	"halid.org/engine/internal/store"
	"halid.org/engine/internal/store/storetest"
)

func TestFold(t *testing.T) {
	cases := map[string]string{
		"İstanbul": "istanbul",
		"ISTANBUL": "istanbul",
		"ıstanbul": "istanbul",
		"IŞIK":     "işik",
		"ışık":     "işik",
		"Işık":     "işik",
		"İ̇":       "i", // decomposed İ (I + combining dot)
		"Çğöşü":    "çğöşü",
		"Go 1.26":  "go 1.26",
	}
	for in, want := range cases {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		in, match, kind string
	}{
		{"", "", ""},
		{"   ", "", ""},
		{"htmx", `"htmx"*`, ""},
		{"go htm", `"go" "htm"*`, ""},
		{"pr: radar", `"radar"*`, "project"},
		{"h:rack", `"rack"*`, "project"},
		{"S: fold", `"fold"*`, "snippet"},
		{"x: foo", `"x" "foo"*`, ""}, // unknown prefix is plain text
		{`ISTANBUL "OR" NEAR(a b)`, `"istanbul" "or" "near" "a" "b"*`, ""},
		{"p:", "", "post"},
		{"Işık*", `"işik"*`, ""},
	}
	for _, c := range cases {
		q := Parse(c.in)
		if q.Match != c.match || q.Kind != c.kind {
			t.Errorf("Parse(%q) = {%q %q}, want {%q %q}", c.in, q.Match, q.Kind, c.match, c.kind)
		}
	}
}

func index(t *testing.T, db store.DB, docs ...Doc) {
	t.Helper()
	for _, d := range docs {
		if _, err := db.Batch(context.Background(), d.Upsert()...); err != nil {
			t.Fatal(err)
		}
	}
}

func search(t *testing.T, db store.DB, raw string, f Filter) []Hit {
	t.Helper()
	st, ok := Parse(raw).Stmt(f)
	if !ok {
		return nil
	}
	res, err := db.Query(context.Background(), st.SQL, st.Args...)
	if err != nil {
		t.Fatal(err)
	}
	hits, err := store.Scan[Hit](res)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

func ids(hits []Hit) []string {
	out := make([]string, len(hits))
	for i, h := range hits {
		out[i] = h.EntryID
	}
	return out
}

func TestSearchAgainstFTS5(t *testing.T) {
	db := storetest.NewSearchDB(t)
	pub := func(id, lang, title, body string) Doc {
		return Doc{EntryID: id, Kind: "post", Lang: lang, Status: "published", Visibility: "public", Slug: id, Title: title, Body: body}
	}
	index(t, db,
		pub("a", "tr", "IŞIK ve gölge", "Fotoğrafta ışık ölçümü"),
		pub("b", "tr", "İstanbul notları", "Şehirde bir hafta"),
		pub("c", "en", "Notes on htmx", "Fragments and out-of-band swaps"),
		Doc{EntryID: "d", Kind: "post", Lang: "en", Status: "draft", Visibility: "public", Slug: "d", Title: "htmx draft"},
		Doc{EntryID: "e", Kind: "snippet", Lang: "en", Status: "published", Visibility: "public", Slug: "e", Title: "htmx snippet"},
	)

	// Every spelling of ışık finds the same post.
	for _, q := range []string{"ışık", "IŞIK", "isik", "Işık", "ISIK"} {
		if got := ids(search(t, db, q, Filter{})); len(got) != 1 || got[0] != "a" {
			t.Errorf("%q: got %v, want [a]", q, got)
		}
	}
	// Dotted capital and plain ASCII meet.
	for _, q := range []string{"istanbul", "İSTANBUL", "ıstan"} {
		if got := ids(search(t, db, q, Filter{})); len(got) != 1 || got[0] != "b" {
			t.Errorf("%q: got %v, want [b]", q, got)
		}
	}
	// ş, ğ, ö are stripped by the tokenizer on both sides.
	if got := ids(search(t, db, "sehir", Filter{})); len(got) != 1 || got[0] != "b" {
		t.Errorf("sehir: got %v", got)
	}

	// Public search hides drafts; admin search shows them.
	if got := ids(search(t, db, "htmx", Filter{})); len(got) != 2 {
		t.Errorf("public htmx: got %v, want c and e", got)
	}
	if got := ids(search(t, db, "htmx", Filter{Drafts: true})); len(got) != 3 {
		t.Errorf("admin htmx: got %v, want c, d, e", got)
	}
	// Kind prefix and language filter.
	if got := ids(search(t, db, "s: htmx", Filter{})); len(got) != 1 || got[0] != "e" {
		t.Errorf("s: htmx: got %v", got)
	}
	if got := ids(search(t, db, "notlari", Filter{Lang: "en"})); len(got) != 0 {
		t.Errorf("lang filter: got %v", got)
	}
	// Title outranks body.
	index(t, db, pub("f", "en", "Swaps", "nothing here"))
	if got := ids(search(t, db, "swaps", Filter{})); len(got) != 2 || got[0] != "f" {
		t.Errorf("rank: got %v, want f first", got)
	}

	// Re-indexing replaces the row, delete removes it.
	index(t, db, pub("c", "en", "Renamed", "gone"))
	if got := ids(search(t, db, "fragments", Filter{})); len(got) != 0 {
		t.Errorf("after reindex: got %v", got)
	}
	if _, err := db.Query(context.Background(), Delete("a").SQL, Delete("a").Args...); err != nil {
		t.Fatal(err)
	}
	if got := ids(search(t, db, "isik", Filter{})); len(got) != 0 {
		t.Errorf("after delete: got %v", got)
	}
}

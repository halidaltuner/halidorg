package jobs

import (
	"slices"
	"testing"
)

func TestRelatedChanged(t *testing.T) {
	prev := &Ref{Kind: "note", Lang: "en", Slug: "til", Title: "", Summary: "Folding", Listed: true, Tags: []string{"go"}}
	now := *prev
	if RelatedChanged(Change{Now: &now, Prev: prev}) {
		t.Error("a body edit changed related blocks")
	}
	retagged, renamed, unlisted := now, now, now
	retagged.Tags = []string{"go", "unicode"}
	renamed.Summary = "Folding ı"
	unlisted.Listed = false
	for name, n := range map[string]*Ref{"tags": &retagged, "summary": &renamed, "unlisted": &unlisted, "deleted": nil} {
		if !RelatedChanged(Change{Now: n, Prev: prev}) {
			t.Errorf("%s: related unchanged", name)
		}
	}
	if RelatedChanged(Change{Now: &Ref{Kind: "note", Lang: "en", Slug: "x", Listed: true}}) {
		t.Error("an untagged entry is nobody's related")
	}
	got := Paths(Change{Now: &retagged, Prev: prev, Related: []Ref{{Kind: "term", Lang: "en", Slug: "ulid"}, {Kind: "post", Lang: "en", Slug: "ghost"}}})
	for _, want := range []string{"/en/glossary/ulid", "/en/posts/ghost"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
}

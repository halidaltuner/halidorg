package jobs

import (
	"slices"
	"testing"
)

func TestSnippetRawPathsAndPeers(t *testing.T) {
	prev := &Ref{Kind: "snippet", Lang: "en", Slug: "graceful", Title: "go: graceful", Listed: true, Files: []string{"main.go", "my file.sh"}}
	now := *prev
	now.Files = []string{"main.go"}
	c := Change{Now: &now, Prev: prev, Peers: []Ref{{Kind: "snippet", Lang: "en", Slug: "prune"}}}
	got := Paths(c)
	for _, want := range []string{"/en/snippets/graceful/raw/main.go", "/en/snippets/graceful/raw/my%20file.sh", "/en/snippets/prune"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}

	// A code-only edit keeps the list as it is; title, first file and
	// listing changes alter it.
	if PeersChanged(c) {
		t.Error("code edit changed the list")
	}
	renamed := now
	renamed.Title = "go: graceful shutdown"
	newFirst := now
	newFirst.Files = []string{"server.go"}
	unlisted := now
	unlisted.Listed = false
	for name, n := range map[string]*Ref{"title": &renamed, "first file": &newFirst, "unlisted": &unlisted, "deleted": nil} {
		if !PeersChanged(Change{Now: n, Prev: prev}) {
			t.Errorf("%s: list unchanged", name)
		}
	}
	if !PeersChanged(Change{Now: &now}) {
		t.Error("new snippet: list unchanged")
	}
	if PeersChanged(Change{Now: &Ref{Kind: "note", Lang: "en", Slug: "x", Listed: true}}) {
		t.Error("notes have no peer list")
	}
}

func TestLinkAwesomePaths(t *testing.T) {
	prev := &Ref{Kind: "link", Lang: "en", Slug: "grug", Listed: true, Categories: []string{"go", "programming"}}
	now := *prev
	now.Categories = []string{"design"}
	got := Paths(Change{Now: &now, Prev: prev})
	for _, want := range []string{"/en/links", "/en/awesome", "/en/awesome/go", "/en/awesome/programming", "/en/awesome/design", "/en/links/feed.xml"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %s in %v", want, got)
		}
	}
	if slices.Contains(got, "/en/links/grug") {
		t.Error("links have no item pages")
	}
}

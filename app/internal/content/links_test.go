package content

import (
	"reflect"
	"testing"
)

func TestExtractLinks(t *testing.T) {
	body := `<p>See <a href="/en/posts/htmx-admin">the <em>htmx</em> admin</a>,
<a href="https://halid.org/tr/posts/ghosttan-ayrilmak#x">Ghost</a>, again <a href="/en/posts/htmx-admin/">dup</a>,
<a href="/en/tags/go">a tag</a>, <a href="https://example.com/en/posts/x">elsewhere</a>,
<a class="x" href="/en/posts/Bad_Slug">bad</a> and <a href="/en/posts/a?ref=1">query</a>.
Other kinds: <a href="/en/glossary/ulid">ULID</a>, <a href="/tr/notes/til">til</a>, <a href="/en/projects/kromatik/log/day-1">a log</a>,
<a href="/en/photos/x">photos</a>.</p>`
	got := ExtractLinks(body, "https://halid.org")
	want := []Link{
		{Lang: "en", Section: "posts", Slug: "htmx-admin", Text: "the htmx admin"},
		{Lang: "tr", Section: "posts", Slug: "ghosttan-ayrilmak", Text: "Ghost"},
		{Lang: "en", Section: "posts", Slug: "a", Text: "query"},
		{Lang: "en", Section: "glossary", Slug: "ulid", Text: "ULID"},
		{Lang: "tr", Section: "notes", Slug: "til", Text: "til"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

func TestSlugify(t *testing.T) {
	for in, want := range map[string]string{
		"Ghost'tan ayrılmak: İlk adımlar":   "ghosttan-ayrilmak-ilk-adimlar",
		"  Notes on an htmx-first admin!! ": "notes-on-an-htmx-first-admin",
		"ÇĞÖŞÜ":                             "cgosu",
		"***":                               "",
	} {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

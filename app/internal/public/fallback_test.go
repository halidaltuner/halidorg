package public

import (
	"strings"
	"testing"
)

// Phase 7.3: a list shows entries with no translation in its language,
// tagged and linking to their own page; translated ones only once.
func TestListFallback(t *testing.T) {
	mux, db := newSite(t)
	seed(t, db)
	_, body := get(t, mux, "/en/posts")
	must(t, body, `href="/tr/posts/only-tr"`, `<h3 lang="tr">Sadece Türkçe <span class="lang-tag mono" title="tr">tr</span></h3>`,
		`href="/en/posts/ghost"`)
	if strings.Contains(body, "ghosttan-ayrilmak") {
		t.Error("a translated post shows twice")
	}
	_, body = get(t, mux, "/tr/posts")
	must(t, body, `href="/en/posts/htmx-admin"`, `href="/tr/posts/ghosttan-ayrilmak"`)
	if strings.Contains(body, `href="/en/posts/ghost"`) {
		t.Error("the en original shows next to its tr translation")
	}
}

package public

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"halid.org/engine/internal/store"
	"halid.org/engine/web"
)

// jsBudget is the script a content page may load up front (ROADMAP Phase
// 7.5): uncompressed bytes of the files its <script> tags name. Mermaid's
// chunks load on demand near a diagram and are not counted (docs/mermaid.md).
const jsBudget = 30 << 10

var scriptSrc = regexp.MustCompile(`<script[^>]+src="([^"]+)"`)

// TestJSBudget renders the public pages with content and adds up the
// scripts each one loads (each seed on its own site: they share slugs).
func TestJSBudget(t *testing.T) {
	groups := []struct {
		seed  func(*testing.T, store.DB)
		pages []string
	}{
		{seed, []string{"/en/", "/en/stream", "/en/posts", "/en/posts/htmx-admin", "/en/garden", "/en/runbooks", "/en/series", "/en/search?q=htmx"}},
		{seedShort, []string{"/en/notes", "/en/snippets", "/en/glossary", "/en/reading"}},
		{seedCuration, []string{"/en/links", "/en/favs", "/en/awesome"}},
		{seedProjects, []string{"/en/projects", "/en/homelab", "/en/certificates"}},
		{func(t *testing.T, db store.DB) { seedPresentations(t, db) }, []string{"/en/presentations"}},
		{seedRadar, []string{"/en/radar"}},
		{func(t *testing.T, db store.DB) { seedPhotos(t, db) }, []string{"/en/photos"}},
	}
	for _, g := range groups {
		mux, db := newSite(t)
		g.seed(t, db)
		for _, p := range g.pages {
			checkBudget(t, mux, p)
		}
	}
}

func checkBudget(t *testing.T, mux *http.ServeMux, p string) {
	t.Helper()
	code, body := get(t, mux, p)
	if code != 200 {
		t.Errorf("%s: %d", p, code)
		return
	}
	raw, gz := 0, 0
	var names []string
	for _, m := range scriptSrc.FindAllStringSubmatch(body, -1) {
		u, err := url.Parse(m[1])
		if err != nil || u.Host != "" {
			t.Errorf("%s: script from elsewhere: %s", p, m[1])
			continue
		}
		b, err := web.Static.ReadFile(strings.TrimPrefix(u.Path, "/"))
		if err != nil {
			t.Errorf("%s: %s: %v", p, u.Path, err)
			continue
		}
		var z bytes.Buffer
		w, _ := gzip.NewWriterLevel(&z, gzip.BestCompression)
		_, _ = w.Write(b)
		_ = w.Close()
		raw, gz = raw+len(b), gz+z.Len()
		names = append(names, u.Path[strings.LastIndex(u.Path, "/")+1:])
	}
	if raw > jsBudget {
		t.Errorf("%s loads %d bytes of JS (budget %d): %v", p, raw, jsBudget, names)
	}
	t.Logf("%-20s %6d B, %5d B gzip  %v", p, raw, gz, names)
}

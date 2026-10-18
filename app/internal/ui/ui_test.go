package ui

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func same(string) string { return "/x" }

func TestNavShowsOnlyEnabledSections(t *testing.T) {
	c := NewChrome("en", "posts", false, same)
	if len(c.Nav) != 1 || c.Nav[0].Slug != "posts" || !c.Nav[0].Active || c.Nav[0].Href != "/en/posts" {
		t.Fatalf("nav %+v", c.Nav)
	}
	if len(c.More) != 0 {
		t.Fatalf("more %+v, want none until those sections exist", c.More)
	}
	if !c.Search {
		t.Error("the palette ships in Phase 1: public pages load it")
	}
}

func TestNavAllForStyleguide(t *testing.T) {
	c := NewChrome("tr", "radar", true, same)
	if len(c.Nav) != len(nav) || len(c.More) != len(more) {
		t.Fatalf("got %d+%d items", len(c.Nav), len(c.More))
	}
	for _, n := range c.Nav {
		if n.Active != (n.Slug == "radar") {
			t.Errorf("%s active=%v", n.Slug, n.Active)
		}
	}
	if c.Nav[0].Label != "yazılar" || c.More[0].Href != "/tr/notes" {
		t.Errorf("labels %q %q", c.Nav[0].Label, c.More[0].Href)
	}
}

func TestDeployIsShortSHA(t *testing.T) {
	t.Setenv("DEPLOY_SHA", "a3f91c2deadbeef")
	if got := Deploy(); got != "a3f91c2" {
		t.Errorf("got %q", got)
	}
	t.Setenv("DEPLOY_SHA", "")
	if got := Deploy(); got != "" {
		t.Errorf("got %q", got)
	}
}

func render(t *testing.T, name string, data any) string {
	t.Helper()
	tpl, err := Parse("test", "templates/partials/base.html")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, name, data); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}

func TestHeader(t *testing.T) {
	out := render(t, "header", NewChrome("en", "posts", false, same))
	for _, want := range []string{
		`class="wordmark" href="/en/"`,
		`<a href="/en/posts" data-s="posts" aria-current="page">posts</a>`,
		`<details class="nav-sheet">`,
		`data-palette hidden`,
		`class="lang-switch"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s in\n%s", want, out)
		}
	}
	if strings.Contains(out, "nav-more") {
		t.Error("empty more menu must not render")
	}
	if out = render(t, "header", NewChrome("en", "", true, same)); !strings.Contains(out, `<details class="nav-more">`) {
		t.Error("more menu missing with all sections")
	}
}

func TestFooterOmitsEmptyFields(t *testing.T) {
	out := render(t, "footer", Chrome{Lang: "en"})
	for _, absent := range []string{"tagline", "site-footer-links", "deploy"} {
		if strings.Contains(out, absent) {
			t.Errorf("empty footer rendered %q", absent)
		}
	}
	out = render(t, "footer", Chrome{Lang: "en", Footer: Footer{
		Tagline: "t", Deploy: "abc1234", Links: []Link{{Label: "admin", Href: "/admin", Muted: true}},
	}})
	for _, want := range []string{`<span class="tagline">t</span>`, `<a href="/admin" class="muted">admin</a>`, `<span class="sha">abc1234</span>`} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s", want)
		}
	}
}

func TestCover(t *testing.T) {
	fallback := render(t, "cover", map[string]any{"tone": "rose", "radius": 2})
	if !strings.Contains(fallback, `class="cover tone-rose r-2"`) || !strings.Contains(fallback, "cover-stripes") || strings.Contains(fallback, "<img") {
		t.Errorf("fallback: %s", fallback)
	}
	img := render(t, "cover", map[string]any{"image": "/media/a.jpg", "alt": "rack"})
	if !strings.Contains(img, `class="cover tone-cyan"`) || !strings.Contains(img, `<img src="/media/a.jpg" alt="rack"`) || strings.Contains(img, "cover-stripes") {
		t.Errorf("image: %s", img)
	}
}

func TestSmallComponents(t *testing.T) {
	cases := map[string]struct {
		data map[string]any
		want string
	}{
		"chip":         {map[string]any{"label": "go", "href": "?tag=go", "on": true, "tone": "amber"}, `<a class="chip tone-amber" href="?tag=go" aria-current="true">go</a>`},
		"tag-pill":     {map[string]any{"label": "htmx"}, `<span class="tag-pill tone-cyan">htmx</span>`},
		"status-badge": {map[string]any{"label": "draft", "kind": "info"}, `<span class="badge info">draft</span>`},
		"callout":      {map[string]any{"kind": "tip", "body": "x"}, `<aside class="callout tip"><div class="callout-label">tip</div>x</aside>`},
	}
	for name, c := range cases {
		if got := render(t, name, c.data); got != c.want {
			t.Errorf("%s:\n got %s\nwant %s", name, got, c.want)
		}
	}
}

func TestDictRejectsOddArgs(t *testing.T) {
	if _, err := dict("a"); err == nil {
		t.Error("want error")
	}
	if _, err := dict(1, 2); err == nil {
		t.Error("want error")
	}
}

func TestDeployHeader(t *testing.T) {
	before := DeploySHA()
	t.Cleanup(func() { deploySHA.Store(before) })
	serve := func(sha string) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Deploy-Sha", sha)
		DeployHeader(http.NotFoundHandler()).ServeHTTP(httptest.NewRecorder(), req)
	}
	serve("8c1d3b768056eed540e884fc712ec7bfbbe9e81f")
	if DeploySHA() != "8c1d3b768056eed540e884fc712ec7bfbbe9e81f" || Deploy() != "8c1d3b7" {
		t.Fatalf("deploy = %q / %q", DeploySHA(), Deploy())
	}
	for _, bad := range []string{"", "<script>", "not-a-sha", "ABCDEF1"} {
		serve(bad)
		if DeploySHA() != "8c1d3b768056eed540e884fc712ec7bfbbe9e81f" {
			t.Errorf("%q replaced the deploy", bad)
		}
	}
}

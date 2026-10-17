package render

import (
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files")

// TestGolden renders every testdata/*.md and compares with the .html next
// to it. Run `go test ./internal/render -update` after an intended change
// and review the diff.
func TestGolden(t *testing.T) {
	files, _ := filepath.Glob("testdata/*.md")
	if len(files) == 0 {
		t.Fatal("no fixtures")
	}
	for _, f := range files {
		t.Run(filepath.Base(f), func(t *testing.T) {
			src, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			lang := "en"
			if strings.HasSuffix(f, ".tr.md") {
				lang = "tr"
			}
			out, err := Markdown(src, Options{Lang: lang})
			if err != nil {
				t.Fatal(err)
			}
			golden := strings.TrimSuffix(f, ".md") + ".html"
			if *update {
				if err := os.WriteFile(golden, []byte(out.HTML), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (run with -update)", err)
			}
			if out.HTML != string(want) {
				t.Errorf("%s differs from %s; run with -update and review the diff", f, golden)
			}
		})
	}
}

func TestPostDocument(t *testing.T) {
	src, _ := os.ReadFile("testdata/post.md")
	out, err := Markdown(src, Options{Lang: "en"})
	if err != nil {
		t.Fatal(err)
	}
	h := out.HTML

	wantTOC := []Heading{
		{2, "why-not-an-spa", "Why not an SPA"},
		{2, "autosave-meets-revisions", "Autosave meets revisions"},
		{3, "neden-spa-degil-iste-cevap", "Neden SPA değil? İşte cevap"},
		{2, "why-not-an-spa-2", "Why not an SPA"},
	}
	if len(out.TOC) != len(wantTOC) {
		t.Fatalf("toc = %+v", out.TOC)
	}
	for i := range wantTOC {
		if out.TOC[i] != wantTOC[i] {
			t.Errorf("toc[%d] = %+v, want %+v", i, out.TOC[i], wantTOC[i])
		}
	}
	if !strings.HasPrefix(out.TOCJSON(), `[{"level":2,"id":"why-not-an-spa"`) {
		t.Errorf("toc json = %s", out.TOCJSON())
	}

	contains := []string{
		// side note next to its paragraph, reference points at it
		`<div class="sn-row">`,
		`<sup class="sn-ref"><a href="#sn-1" role="doc-noteref">1</a></sup>`,
		`<aside class="sn" id="sn-1"><span class="sn-num">1</span> `,
		// callouts use the partial's markup
		`<aside class="callout tip"><div class="callout-label">tip</div>`,
		`<aside class="callout warning"><div class="callout-label">dikkat</div>`,
		// light block with filename bar
		`<figure class="code code-light">`,
		`<span class="code-file">internal/admin/revisions.go</span>`,
		// two tabs fences become one dark group
		`<figure class="code code-dark" data-code-group id="cg1">`,
		`aria-selected="true" data-lang="html">editor.html</button>`,
		`aria-selected="false" data-lang="go">handler.go</button>`,
		`<span class="ln">1</span>`,
		// terminal: attribute cwd, inline cwd, output lines
		`<span class="term-cwd">~/halid</span> <span class="term-ps">$</span> go test`,
		`<span class="term-cwd">/tmp</span> <span class="term-ps">$</span> ls`,
		`<span class="term-out">ok  	halid/internal/admin	0.412s</span>`,
		// footnote outside a top-level paragraph stays at the end
		`<section class="footnotes" role="doc-endnotes">`,
		`<li id="fn-2">`,
		`<pre class="mermaid">graph TD; A--&gt;B`,
		`<h3 id="neden-spa-degil-iste-cevap">`,
	}
	for _, c := range contains {
		if !strings.Contains(h, c) {
			t.Errorf("missing %q", c)
		}
	}
	for _, c := range []string{"[!TIP]", "[!WARNING]", "fnref-1", `<blockquote>
<p>Explicit`} {
		if strings.Contains(h, c) {
			t.Errorf("unexpected %q", c)
		}
	}
	if !strings.Contains(h, "<blockquote>\n<p>Just a plain quote.</p>") {
		t.Error("plain blockquote was turned into something else")
	}
	if strings.Count(h, `data-code-group`) != 1 {
		t.Error("expected exactly one code group")
	}
	if !out.Mermaid {
		t.Error("mermaid flag not set")
	}
	if !strings.Contains(out.Text, "func save()") || !strings.Contains(out.Text, "Neden SPA değil") {
		t.Errorf("plain text misses code or headings: %q", out.Text)
	}
	if out.Words < 60 || out.Words > 120 {
		t.Errorf("words = %d", out.Words)
	}
}

func TestLabelsFollowLang(t *testing.T) {
	src := []byte("```go\nx\n```\n")
	tr, _ := Markdown(src, Options{Lang: "tr"})
	en, _ := Markdown(src, Options{Lang: "xx"})
	if !strings.Contains(tr.HTML, ">kopyala</button>") || !strings.Contains(en.HTML, ">copy</button>") {
		t.Errorf("labels: tr=%q en=%q", tr.HTML, en.HTML)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Neden SPA değil?":      "neden-spa-degil",
		"İstanbul'da Işık":      "istanbul-da-isik",
		"  --Go 1.26!-- ":       "go-1-26",
		"日本語":                   "",
		"Çağrı Öğütücü Şüphesi": "cagri-ogutucu-suphesi",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRawHTMLAndEmpty(t *testing.T) {
	out, err := Markdown([]byte(""), Options{})
	if err != nil || out.HTML != "" || out.TOCJSON() != "[]" {
		t.Errorf("empty: %+v %v", out, err)
	}
	out, _ = Markdown([]byte("<div class=\"x\">ghost card</div>\n"), Options{})
	if !strings.Contains(out.HTML, `<div class="x">ghost card</div>`) {
		t.Errorf("raw html dropped: %q", out.HTML)
	}
}

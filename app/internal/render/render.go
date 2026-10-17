// Package render turns Markdown into the HTML stored on a revision
// (ADR-002). Markup follows design/Post.dc.html and the class names in
// web/static/css/app.css. Authoring syntax is documented in doc.go.
package render

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// Options tune one render.
type Options struct {
	// Lang is the entry language ("tr" or "en"); it picks the labels of the
	// code block buttons. Anything else falls back to English.
	Lang string
}

// Heading is one TOC entry (h2 and h3).
type Heading struct {
	Level int    `json:"level"`
	ID    string `json:"id"`
	Text  string `json:"text"`
}

// Output is a rendered document.
type Output struct {
	HTML    string
	TOC     []Heading
	Text    string // plain text for the search index, code included
	Words   int    // prose words, code excluded; feeds reading time
	Mermaid bool   // the page needs the Mermaid island (Phase 7)
}

// TOCJSON is the value for revisions.toc_json.
func (o Output) TOCJSON() string {
	if len(o.TOC) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(o.TOC)
	return string(b)
}

// state is shared by the transformers and the node renderer of one call.
type state struct {
	opts      Options
	toc       []Heading
	ids       map[string]int
	sidenoted map[int]bool
	groups    int
	mermaid   bool
}

func (s *state) label(key string) string {
	l, ok := labels[s.opts.Lang]
	if !ok {
		l = labels["en"]
	}
	return l[key]
}

var labels = map[string]map[string]string{
	"en": {"copy": "copy", "wrap": "wrap", "terminal": "terminal"},
	"tr": {"copy": "kopyala", "wrap": "kaydır", "terminal": "terminal"},
}

// Markdown renders src. Raw HTML is passed through: the admin is the only
// author (ADR-005) and Ghost imports carry HTML cards (CONTENT_MODEL.md §7).
func Markdown(src []byte, opts Options) (Output, error) {
	st := &state{opts: opts, ids: map[string]int{}, sidenoted: map[int]bool{}}
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote),
		goldmark.WithParserOptions(parser.WithASTTransformers(
			util.Prioritized(calloutTransformer{}, 500),
			util.Prioritized(headingTransformer{st}, 600),
			util.Prioritized(codeGroupTransformer{}, 700),
			// After the footnote extension's own transformer (999).
			util.Prioritized(sidenoteTransformer{st}, 1000),
		)),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			renderer.WithNodeRenderers(util.Prioritized(&nodeRenderer{st: st}, 100)),
		),
	)
	doc := md.Parser().Parse(text.NewReader(src))
	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, src, doc); err != nil {
		return Output{}, err
	}
	plain, words := plainText(doc, src)
	return Output{
		HTML:    buf.String(),
		TOC:     st.toc,
		Text:    plain,
		Words:   words,
		Mermaid: st.mermaid,
	}, nil
}

// inlineText is the visible text of an inline subtree.
func inlineText(n ast.Node, src []byte) string {
	var b strings.Builder
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := c.(type) {
		case *ast.Text:
			b.Write(v.Segment.Value(src))
			if v.SoftLineBreak() || v.HardLineBreak() {
				b.WriteByte(' ')
			}
		case *ast.String:
			b.Write(v.Value)
		case *east.FootnoteLink:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.TrimSpace(b.String())
}

// plainText walks the final tree: prose words are counted, code is kept in
// the text (snippets and posts are searchable by code) but not counted.
func plainText(doc ast.Node, src []byte) (string, int) {
	var parts []string
	words := 0
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.FencedCodeBlock, *ast.CodeBlock:
			parts = append(parts, string(v.Lines().Value(src)))
			return ast.WalkSkipChildren, nil
		case *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		case *ast.Paragraph, *ast.Heading, *ast.TextBlock, *east.TableCell:
			t := inlineText(v, src)
			if t != "" {
				parts = append(parts, t)
				words += len(strings.FieldsFunc(t, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return strings.Join(parts, "\n"), words
}

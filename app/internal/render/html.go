package render

import (
	"bufio"
	"bytes"
	"fmt"
	"html"
	"regexp"
	"strconv"
	"strings"

	"github.com/alecthomas/chroma/v2"
	chromahtml "github.com/alecthomas/chroma/v2/formatters/html"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
)

// nodeRenderer overrides headings, fences and footnotes, and renders the
// node kinds the transformers add. Registered at priority 100 so it wins
// over goldmark's defaults (1000) and the footnote renderer (500).
type nodeRenderer struct{ st *state }

func (r *nodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindHeading, r.heading)
	reg.Register(ast.KindFencedCodeBlock, r.fence)
	reg.Register(kindCallout, r.callout)
	reg.Register(kindCodeGroup, r.codeGroup)
	reg.Register(kindEmbed, r.embed)
	reg.Register(kindSidenoteRow, r.sidenoteRow)
	reg.Register(kindSidenote, r.sidenote)
	reg.Register(east.KindFootnoteLink, r.footnoteLink)
	reg.Register(east.KindFootnoteBacklink, r.footnoteBacklink)
	reg.Register(east.KindFootnoteList, r.footnoteList)
	reg.Register(east.KindFootnote, r.footnote)
}

func (r *nodeRenderer) heading(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	h := n.(*ast.Heading)
	id, _ := h.AttributeString("id")
	ids, _ := id.([]byte)
	if entering {
		fmt.Fprintf(w, `<h%d id="%s">`, h.Level, html.EscapeString(string(ids)))
		return ast.WalkContinue, nil
	}
	if h.Level == 2 || h.Level == 3 {
		fmt.Fprintf(w, `<a class="h-anchor" href="#%s" aria-hidden="true" tabindex="-1">#</a>`, html.EscapeString(string(ids)))
	}
	fmt.Fprintf(w, "</h%d>\n", h.Level)
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) callout(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	c := n.(*callout)
	if entering {
		label := c.Label
		if label == "" {
			label = c.Variant
		}
		fmt.Fprintf(w, `<aside class="callout %s"><div class="callout-label">%s</div>`+"\n", c.Variant, html.EscapeString(label))
		return ast.WalkContinue, nil
	}
	w.WriteString("</aside>\n")
	return ast.WalkContinue, nil
}

// --- Footnotes and side notes ----------------------------------------------

func fnRefID(idx, ref int) string {
	if ref == 0 {
		return "fnref-" + strconv.Itoa(idx)
	}
	return "fnref-" + strconv.Itoa(idx) + "-" + strconv.Itoa(ref+1)
}

func (r *nodeRenderer) footnoteLink(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	l := n.(*east.FootnoteLink)
	i := strconv.Itoa(l.Index)
	if r.st.sidenoted[l.Index] {
		fmt.Fprintf(w, `<sup class="sn-ref"><a href="#sn-%s" role="doc-noteref">%s</a></sup>`, i, i)
		return ast.WalkContinue, nil
	}
	fmt.Fprintf(w, `<sup class="fn-ref" id="%s"><a href="#fn-%s" role="doc-noteref">%s</a></sup>`, fnRefID(l.Index, l.RefIndex), i, i)
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) footnoteBacklink(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		b := n.(*east.FootnoteBacklink)
		fmt.Fprintf(w, ` <a class="fn-back" href="#%s" role="doc-backlink">↩</a>`, fnRefID(b.Index, b.RefIndex))
	}
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) footnoteList(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.WriteString(`<section class="footnotes" role="doc-endnotes"><ol>` + "\n")
	} else {
		w.WriteString("</ol></section>\n")
	}
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) footnote(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		fmt.Fprintf(w, `<li id="fn-%d">`+"\n", n.(*east.Footnote).Index)
	} else {
		w.WriteString("</li>\n")
	}
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) sidenoteRow(w util.BufWriter, _ []byte, _ ast.Node, entering bool) (ast.WalkStatus, error) {
	if entering {
		w.WriteString(`<div class="sn-row">` + "\n")
	} else {
		w.WriteString("</div>\n")
	}
	return ast.WalkContinue, nil
}

func (r *nodeRenderer) sidenote(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	s := n.(*sidenote)
	if entering {
		fmt.Fprintf(w, `<aside class="sn" id="sn-%d"><span class="sn-num">%d</span> `, s.Index, s.Index)
	} else {
		w.WriteString("</aside>\n")
	}
	return ast.WalkContinue, nil
}

// --- Code -------------------------------------------------------------------

func codeText(n ast.Node, src []byte) string {
	var b bytes.Buffer
	lines := n.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		b.Write(seg.Value(src))
	}
	return b.String()
}

func (r *nodeRenderer) fence(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	fi, _ := fenceOf(n, src)
	code := codeText(n, src)
	switch fi.Lang {
	case "terminal", "console", "shell-session":
		r.terminal(w, fi, code)
	case "mermaid":
		r.st.mermaid = true
		fmt.Fprintf(w, `<pre class="mermaid">%s</pre>`+"\n", html.EscapeString(code))
	default:
		r.light(w, fi, code)
	}
	return ast.WalkSkipChildren, nil
}

func (r *nodeRenderer) light(w util.BufWriter, fi fenceInfo, code string) {
	head := fi.Attrs["file"]
	if head == "" {
		head = fi.Lang
	}
	w.WriteString(`<figure class="code code-light">` + "\n")
	fmt.Fprintf(w, `<div class="code-bar"><span class="code-file">%s</span><span class="code-tools">`, html.EscapeString(head))
	fmt.Fprintf(w, `<button type="button" class="code-btn" data-code-wrap hidden>%s</button>`, r.st.label("wrap"))
	fmt.Fprintf(w, `<button type="button" class="code-btn code-copy" data-code-copy hidden>%s</button></span></div>`+"\n", r.st.label("copy"))
	fmt.Fprintf(w, `<pre class="chroma"><code%s>`, langAttr(fi.Lang))
	highlight(w, fi.Lang, code, false)
	w.WriteString("</code></pre>\n</figure>\n")
}

func (r *nodeRenderer) codeGroup(w util.BufWriter, src []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkSkipChildren, nil
	}
	r.st.groups++
	gid := "cg" + strconv.Itoa(r.st.groups)
	type tab struct {
		fi   fenceInfo
		code string
	}
	var tabs []tab
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		fi, _ := fenceOf(c, src)
		tabs = append(tabs, tab{fi, codeText(c, src)})
	}
	if len(tabs) == 0 {
		return ast.WalkSkipChildren, nil
	}
	fmt.Fprintf(w, `<figure class="code code-dark" data-code-group id="%s">`+"\n", gid)
	w.WriteString(`<div class="code-bar"><div class="code-tabs" role="tablist">`)
	for i, t := range tabs {
		name := t.fi.Attrs["file"]
		if name == "" {
			name = t.fi.Lang
		}
		fmt.Fprintf(w, `<button type="button" class="code-tab" role="tab" id="%s-%d-tab" aria-controls="%s-%d" aria-selected="%t" data-lang="%s">%s</button>`,
			gid, i, gid, i, i == 0, html.EscapeString(t.fi.Lang), html.EscapeString(name))
	}
	fmt.Fprintf(w, `</div><span class="code-tools"><span class="code-lang">%s</span>`, html.EscapeString(tabs[0].fi.Lang))
	fmt.Fprintf(w, `<button type="button" class="code-btn code-copy" data-code-copy hidden>%s</button></span></div>`+"\n", r.st.label("copy"))
	for i, t := range tabs {
		lines := !t.fi.Flags["nolines"]
		fmt.Fprintf(w, `<pre class="chroma code-panel" role="tabpanel" id="%s-%d" aria-labelledby="%s-%d-tab"><code%s>`, gid, i, gid, i, langAttr(t.fi.Lang))
		highlight(w, t.fi.Lang, t.code, lines)
		w.WriteString("</code></pre>\n")
	}
	w.WriteString("</figure>\n")
	return ast.WalkSkipChildren, nil
}

var promptLine = regexp.MustCompile(`^(?:(\S+) )?\$ (.*)$`)

func (r *nodeRenderer) terminal(w util.BufWriter, fi fenceInfo, code string) {
	w.WriteString(`<figure class="code code-term">` + "\n")
	fmt.Fprintf(w, `<div class="term-bar" aria-hidden="true"><span class="term-dot r"></span><span class="term-dot a"></span><span class="term-dot g"></span><span class="term-title">%s</span></div>`+"\n", r.st.label("terminal"))
	w.WriteString(`<pre class="term"><code>`)
	lines := strings.Split(strings.TrimRight(code, "\n"), "\n")
	for i, ln := range lines {
		if i > 0 {
			w.WriteString("\n")
		}
		if m := promptLine.FindStringSubmatch(ln); m != nil {
			cwd := m[1]
			if cwd == "" {
				cwd = fi.Attrs["cwd"]
			}
			w.WriteString(`<span class="term-cmd">`)
			if cwd != "" {
				fmt.Fprintf(w, `<span class="term-cwd">%s</span> `, html.EscapeString(cwd))
			}
			fmt.Fprintf(w, `<span class="term-ps">$</span> %s</span>`, html.EscapeString(m[2]))
			continue
		}
		fmt.Fprintf(w, `<span class="term-out">%s</span>`, html.EscapeString(ln))
	}
	w.WriteString("</code></pre>\n</figure>\n")
}

func langAttr(lang string) string {
	if lang == "" {
		return ""
	}
	return ` class="language-` + html.EscapeString(lang) + `"`
}

// highlight writes chroma spans with class names; colors live in app.css.
func highlight(w util.BufWriter, lang, code string, lineNumbers bool) {
	lexer := lexers.Get(lang)
	if lexer == nil && lang == "" {
		// A fence without a language: guess from the code, as the editor does.
		lexer = lexers.Analyse(code)
	}
	if lexer == nil {
		lexer = lexers.Fallback
	}
	lexer = chroma.Coalesce(lexer)
	it, err := lexer.Tokenise(nil, code)
	if err != nil {
		w.WriteString(html.EscapeString(code))
		return
	}
	f := chromahtml.New(chromahtml.WithClasses(true), chromahtml.PreventSurroundingPre(true))
	if !lineNumbers {
		if f.Format(w, styles.Fallback, it) != nil {
			w.WriteString(html.EscapeString(code))
		}
		return
	}
	// chroma drops its line wrappers when it does not own the <pre>, so
	// number the lines here: one span per line, number outside the text so
	// copying the code skips it (user-select in app.css).
	for i, line := range chroma.SplitTokensIntoLines(it.Tokens()) {
		fmt.Fprintf(w, `<span class="line"><span class="ln">%d</span><span class="cl">`, i+1)
		if f.Format(w, styles.Fallback, chroma.Literator(line...)) != nil {
			for _, t := range line {
				w.WriteString(html.EscapeString(t.Value))
			}
		}
		w.WriteString("</span></span>")
	}
}

// Code highlights one source file with numbered lines (snippet pages). The
// markup is the same as a code group panel's, so the chroma classes in
// app.css color it.
func Code(lang, code string) string {
	var b bytes.Buffer
	w := bufio.NewWriter(&b)
	highlight(w, lang, strings.TrimSuffix(code, "\n"), true)
	_ = w.Flush()
	return b.String()
}

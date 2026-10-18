package render

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"

	"github.com/yuin/goldmark/ast"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

// --- Callouts ---------------------------------------------------------------

var calloutLine = regexp.MustCompile(`^\[!(NOTE|TIP|WARNING|DANGER|CAUTION|IMPORTANT)\][ \t]*(.*?)[ \t]*$`)

var calloutKinds = map[string]string{
	"NOTE": "note", "IMPORTANT": "note", "TIP": "tip",
	"WARNING": "warning", "DANGER": "danger", "CAUTION": "danger",
}

type calloutTransformer struct{}

func (calloutTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var quotes []*ast.Blockquote
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if q, ok := n.(*ast.Blockquote); ok && entering {
			quotes = append(quotes, q)
		}
		return ast.WalkContinue, nil
	})
	for _, q := range quotes {
		p, ok := q.FirstChild().(*ast.Paragraph)
		if !ok || p.Lines().Len() == 0 {
			continue
		}
		first := p.Lines().At(0)
		m := calloutLine.FindSubmatch(bytes.TrimRight(first.Value(src), "\r\n"))
		if m == nil {
			continue
		}
		c := &callout{Variant: calloutKinds[string(m[1])], Label: string(m[2])}
		// Drop the inline nodes of the [!KIND] line.
		for in := p.FirstChild(); in != nil; {
			next := in.NextSibling()
			t, isText := in.(*ast.Text)
			if isText && t.Segment.Start >= first.Stop {
				break
			}
			p.RemoveChild(p, in)
			in = next
		}
		if p.ChildCount() == 0 {
			q.RemoveChild(q, p)
		} else if t, ok := p.FirstChild().(*ast.Text); ok {
			// Leading whitespace of the next line.
			t.Segment = t.Segment.TrimLeftSpace(src)
		}
		for ch := q.FirstChild(); ch != nil; {
			next := ch.NextSibling()
			c.AppendChild(c, ch)
			ch = next
		}
		q.Parent().ReplaceChild(q.Parent(), q, c)
	}
}

var kindCallout = ast.NewNodeKind("Callout")

// callout is a blockquote that started with [!KIND].
type callout struct {
	ast.BaseBlock
	Variant string // note, tip, warning, danger
	Label   string
}

func (n *callout) Kind() ast.NodeKind         { return kindCallout }
func (n *callout) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

// --- Headings ---------------------------------------------------------------

type headingTransformer struct{ st *state }

func (h headingTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		hd, ok := n.(*ast.Heading)
		if !ok || !entering {
			return ast.WalkContinue, nil
		}
		label := inlineText(hd, src)
		id := h.st.uniqueID(slugify(label))
		hd.SetAttributeString("id", []byte(id))
		if hd.Level == 2 || hd.Level == 3 {
			h.st.toc = append(h.st.toc, Heading{Level: hd.Level, ID: id, Text: label})
		}
		return ast.WalkSkipChildren, nil
	})
}

func (s *state) uniqueID(base string) string {
	if base == "" {
		base = "section"
	}
	n := s.ids[base]
	s.ids[base] = n + 1
	if n == 0 {
		return base
	}
	return base + "-" + strconv.Itoa(n+1)
}

var asciiFold = strings.NewReplacer(
	"ş", "s", "Ş", "s", "ç", "c", "Ç", "c", "ğ", "g", "Ğ", "g",
	"ö", "o", "Ö", "o", "ü", "u", "Ü", "u", "ı", "i", "İ", "i", "I", "i",
	"â", "a", "î", "i", "û", "u", "é", "e", "è", "e", "à", "a",
)

// slugify makes a heading id: "Neden SPA değil?" -> "neden-spa-degil".
func slugify(s string) string {
	s = strings.ToLower(asciiFold.Replace(s))
	var b strings.Builder
	dash := false
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		case b.Len() > 0 && !dash:
			b.WriteByte('-')
			dash = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}

// --- Code groups ------------------------------------------------------------

var kindCodeGroup = ast.NewNodeKind("CodeGroup")

// codeGroup holds the fences of one dark block; each child is a fence.
type codeGroup struct {
	ast.BaseBlock
}

func (n *codeGroup) Kind() ast.NodeKind         { return kindCodeGroup }
func (n *codeGroup) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

// fenceInfo is a parsed info string: language, key=value pairs and flags.
type fenceInfo struct {
	Lang  string
	Attrs map[string]string
	Flags map[string]bool
}

func parseInfo(raw string) fenceInfo {
	fi := fenceInfo{Attrs: map[string]string{}, Flags: map[string]bool{}}
	for i, f := range strings.Fields(raw) {
		if k, v, ok := strings.Cut(f, "="); ok {
			fi.Attrs[strings.ToLower(k)] = strings.Trim(v, `"'`)
			continue
		}
		if i == 0 {
			fi.Lang = strings.ToLower(f)
			continue
		}
		fi.Flags[strings.ToLower(f)] = true
	}
	if fi.Attrs["file"] == "" && fi.Attrs["title"] != "" {
		fi.Attrs["file"] = fi.Attrs["title"]
	}
	return fi
}

func fenceOf(n ast.Node, src []byte) (fenceInfo, bool) {
	f, ok := n.(*ast.FencedCodeBlock)
	if !ok {
		return fenceInfo{}, false
	}
	if f.Info == nil {
		return fenceInfo{}, true
	}
	return parseInfo(string(f.Info.Segment.Value(src))), true
}

type codeGroupTransformer struct{}

func (codeGroupTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var parents []ast.Node
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering && n.Type() != ast.TypeInline && n.HasChildren() {
			parents = append(parents, n)
		}
		return ast.WalkContinue, nil
	})
	for _, p := range parents {
		for n := p.FirstChild(); n != nil; {
			fi, ok := fenceOf(n, src)
			if !ok || !(fi.Flags["tabs"] || fi.Flags["dark"]) || isSpecial(fi.Lang) {
				n = n.NextSibling()
				continue
			}
			g := &codeGroup{}
			p.InsertBefore(p, n, g)
			tabs := fi.Flags["tabs"]
			for n != nil {
				next := n.NextSibling()
				g.AppendChild(g, n)
				n = next
				if !tabs {
					break
				}
				nfi, ok := fenceOf(n, src)
				if !ok || !nfi.Flags["tabs"] || isSpecial(nfi.Lang) {
					break
				}
			}
		}
	}
}

func isSpecial(lang string) bool {
	switch lang {
	case "terminal", "console", "shell-session", "mermaid":
		return true
	}
	return false
}

// --- Side notes -------------------------------------------------------------

var (
	kindSidenoteRow = ast.NewNodeKind("SidenoteRow")
	kindSidenote    = ast.NewNodeKind("Sidenote")
)

// sidenoteRow wraps a paragraph and the side notes it references.
type sidenoteRow struct{ ast.BaseBlock }

func (n *sidenoteRow) Kind() ast.NodeKind         { return kindSidenoteRow }
func (n *sidenoteRow) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

type sidenote struct {
	ast.BaseBlock
	Index int
}

func (n *sidenote) Kind() ast.NodeKind { return kindSidenote }
func (n *sidenote) Dump(src []byte, level int) {
	ast.DumpHelper(n, src, level, map[string]string{"Index": strconv.Itoa(n.Index)}, nil)
}

type sidenoteTransformer struct{ st *state }

func (s sidenoteTransformer) Transform(doc *ast.Document, _ text.Reader, _ parser.Context) {
	var list *east.FootnoteList
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		if l, ok := n.(*east.FootnoteList); ok {
			list = l
		}
	}
	if list == nil {
		return
	}
	notes := map[int]*east.Footnote{}
	for n := list.FirstChild(); n != nil; n = n.NextSibling() {
		if fn, ok := n.(*east.Footnote); ok {
			notes[fn.Index] = fn
		}
	}
	for n := doc.FirstChild(); n != nil; {
		next := n.NextSibling()
		p, ok := n.(*ast.Paragraph)
		if !ok {
			n = next
			continue
		}
		var refs []int
		_ = ast.Walk(p, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
			if l, ok := c.(*east.FootnoteLink); ok && entering && notes[l.Index] != nil && !s.st.sidenoted[l.Index] {
				refs = append(refs, l.Index)
				s.st.sidenoted[l.Index] = true
			}
			return ast.WalkContinue, nil
		})
		if len(refs) > 0 {
			row := &sidenoteRow{}
			doc.InsertBefore(doc, p, row)
			row.AppendChild(row, p)
			for _, idx := range refs {
				fn := notes[idx]
				sn := &sidenote{Index: idx}
				for ch := fn.FirstChild(); ch != nil; {
					nx := ch.NextSibling()
					sn.AppendChild(sn, ch)
					ch = nx
				}
				stripBacklinks(sn)
				list.RemoveChild(list, fn)
				row.AppendChild(row, sn)
			}
		}
		n = next
	}
	if list.ChildCount() == 0 {
		doc.RemoveChild(doc, list)
	}
}

func stripBacklinks(n ast.Node) {
	var bl []ast.Node
	_ = ast.Walk(n, func(c ast.Node, entering bool) (ast.WalkStatus, error) {
		if _, ok := c.(*east.FootnoteBacklink); ok && entering {
			bl = append(bl, c)
		}
		return ast.WalkContinue, nil
	})
	for _, b := range bl {
		b.Parent().RemoveChild(b.Parent(), b)
	}
}

package render

import (
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

// --- Embeds -----------------------------------------------------------------
//
// A paragraph holding nothing but one link becomes a block of its own:
//
//	https://www.youtube.com/watch?v=dQw4w9WgXcQ   a YouTube player
//	[CV](/media/2026/10/cv.pdf)                     a download card
//
// Anywhere else (inside a sentence, a list item with text) the link stays a
// link, so the Markdown reads the same in any other renderer.

var kindEmbed = ast.NewNodeKind("Embed")

type embed struct {
	ast.BaseBlock
	Video string // YouTube video id
	File  string // /media/… URL of a PDF
	Name  string // the file link's text
}

func (n *embed) Kind() ast.NodeKind         { return kindEmbed }
func (n *embed) Dump(src []byte, level int) { ast.DumpHelper(n, src, level, nil, nil) }

var youtubeID = regexp.MustCompile(`^[A-Za-z0-9_-]{11}$`)

// YouTubeID returns the video id of a YouTube watch, short, shorts or embed
// URL, or "".
func YouTubeID(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Host), "www.")
	host = strings.TrimPrefix(host, "m.")
	var id string
	switch host {
	case "youtu.be":
		id = strings.Trim(u.Path, "/")
	case "youtube.com", "youtube-nocookie.com":
		switch {
		case u.Path == "/watch":
			id = u.Query().Get("v")
		case strings.HasPrefix(u.Path, "/shorts/"), strings.HasPrefix(u.Path, "/embed/"), strings.HasPrefix(u.Path, "/live/"):
			id = path.Base(u.Path)
		}
	}
	if !youtubeID.MatchString(id) {
		return ""
	}
	return id
}

func isMediaPDF(dest string) bool {
	return strings.HasPrefix(dest, "/media/") && strings.HasSuffix(strings.ToLower(dest), ".pdf")
}

type embedTransformer struct{}

func (embedTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	src := reader.Source()
	var found []*ast.Paragraph
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if p, ok := n.(*ast.Paragraph); ok && entering {
			if _, ok := p.Parent().(*ast.Document); ok && p.ChildCount() == 1 {
				found = append(found, p)
			}
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	for _, p := range found {
		var e *embed
		switch l := p.FirstChild().(type) {
		case *ast.AutoLink:
			if id := YouTubeID(string(l.URL(src))); id != "" {
				e = &embed{Video: id}
			}
		case *ast.Link:
			dest := string(l.Destination)
			if id := YouTubeID(dest); id != "" && inlineText(l, src) == dest {
				e = &embed{Video: id}
			} else if isMediaPDF(dest) {
				e = &embed{File: dest, Name: inlineText(l, src)}
			}
		}
		if e != nil {
			p.Parent().ReplaceChild(p.Parent(), p, e)
		}
	}
}

func (r *nodeRenderer) embed(w util.BufWriter, _ []byte, n ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	e := n.(*embed)
	if e.Video != "" {
		// The no-cookie host sets no cookies until the reader plays the video;
		// loading="lazy" keeps the player off pages until it is near view.
		fmt.Fprintf(w, `<figure class="embed embed-video"><iframe src="https://www.youtube-nocookie.com/embed/%s" title="%s" loading="lazy" allow="encrypted-media; picture-in-picture; fullscreen" allowfullscreen referrerpolicy="strict-origin-when-cross-origin"></iframe></figure>`+"\n",
			e.Video, r.st.label("video"))
		return ast.WalkSkipChildren, nil
	}
	name := e.Name
	if name == "" {
		name = path.Base(e.File)
	}
	fmt.Fprintf(w, `<p class="file-card"><a href="%s" download><span class="file-kind mono">PDF</span><span class="file-name">%s</span><span class="file-dl mono">%s ↓</span></a></p>`+"\n",
		html.EscapeString(e.File), html.EscapeString(name), r.st.label("download"))
	return ast.WalkSkipChildren, nil
}

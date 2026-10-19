// Package og renders the 1200x630 Open Graph card (design/OG Template.dc.html)
// as a PNG: dark band with a grid texture, a kind dot with kind and date in
// mono, the title, and the wordmark with its aberration next to the meta.
package og

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"strings"
	"sync"
	"unicode/utf8"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Version changes whenever the card's look changes, so every hash (and with
// it every og:image URL) changes too and social caches fetch the new card.
const Version = "1"

// Width and Height are the card size in pixels.
const (
	Width  = 1200
	Height = 630
)

// Card is what the template takes as props.
type Card struct {
	Kind  string // post, note, snippet, link, fav, project, build log; "" hides the top row
	Title string
	Date  string // YYYY-MM-DD
	Meta  string // e.g. "halid.org · 12 min"
}

// Hash identifies the rendered image: same card and Version, same PNG.
func (c Card) Hash() string {
	sum := sha256.Sum256([]byte(strings.Join([]string{Version, c.Kind, c.Title, c.Date, c.Meta}, "\x00")))
	return hex.EncodeToString(sum[:6])
}

// Template colors. The OG image is a dark band (DESIGN.md §1); these are the
// band tokens from tokens.css, which a PNG renderer cannot read.
var (
	bg      = rgb(0x0B, 0x12, 0x20)
	grid    = rgb(0x17, 0x22, 0x3A)
	text    = rgb(0xE6, 0xEB, 0xF2)
	muted   = rgb(0x8D, 0x98, 0xAB)
	rose    = rgb(0xFB, 0x71, 0x85)
	cyan    = rgb(0x22, 0xD3, 0xEE)
	kindDot = map[string]color.RGBA{
		"post":         cyan,
		"note":         rgb(0xA5, 0xB4, 0xFC),
		"snippet":      rgb(0xFB, 0xBF, 0x24),
		"link":         muted,
		"fav":          rose,
		"project":      rgb(0x4A, 0xDE, 0x80),
		"presentation": rose,
		"garden":       rgb(0x4A, 0xDE, 0x80),
		"runbook":      rgb(0xA5, 0xB4, 0xFC),
		"build log":    rgb(0x4A, 0xDE, 0x80), // a project's log: the project dot
	}
)

func rgb(r, g, b uint8) color.RGBA { return color.RGBA{r, g, b, 255} }

// alpha scales c to a premultiplied color at opacity a.
func alpha(c color.RGBA, a float64) color.RGBA {
	return color.RGBA{uint8(float64(c.R)*a + .5), uint8(float64(c.G)*a + .5), uint8(float64(c.B)*a + .5), uint8(255*a + .5)}
}

// Layout from the template, in CSS pixels.
const (
	padX, padY    = 80, 72
	kindSize      = 26
	kindGap       = 16
	dotSize       = 14
	titleSize     = 76
	titleLH       = 1.08
	titleTrack    = -0.025 // em
	titleMax      = 960
	titleLines    = 3
	wordmarkSize  = 44
	wordmarkTrack = -0.02 // em
	aberration    = 3
	metaSize      = 24
	gridStep      = 48
)

//go:embed fonts/*.ttf
var fontFS embed.FS

type fonts struct {
	sans600, mono400, mono700 *opentype.Font
}

var loadFonts = sync.OnceValues(func() (*fonts, error) {
	parse := func(name string) (*opentype.Font, error) {
		b, err := fontFS.ReadFile("fonts/" + name)
		if err != nil {
			return nil, err
		}
		return opentype.Parse(b)
	}
	var f fonts
	var err error
	if f.sans600, err = parse("IBMPlexSans-SemiBold.ttf"); err != nil {
		return nil, err
	}
	if f.mono400, err = parse("JetBrainsMono-Regular.ttf"); err != nil {
		return nil, err
	}
	if f.mono700, err = parse("JetBrainsMono-Bold.ttf"); err != nil {
		return nil, err
	}
	return &f, nil
})

// face is a font at one size with CSS letter-spacing.
type face struct {
	font.Face
	track fixed.Int26_6
}

func newFace(f *opentype.Font, size, trackEm float64) (face, error) {
	ff, err := opentype.NewFace(f, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingNone})
	if err != nil {
		return face{}, err
	}
	return face{Face: ff, track: fixed.Int26_6(size * trackEm * 64)}, nil
}

// width is the advance of s as CSS lays it out: kerning plus letter-spacing
// after every glyph.
func (f face) width(s string) fixed.Int26_6 {
	var w fixed.Int26_6
	prev := rune(-1)
	for _, r := range s {
		if prev >= 0 {
			w += f.Kern(prev, r)
		}
		a, _ := f.GlyphAdvance(r)
		w += a + f.track
		prev = r
	}
	return w
}

// draw paints s with its left edge at x and baseline at y.
func (f face) draw(dst draw.Image, s string, x, y fixed.Int26_6, c color.Color) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: f.Face, Dot: fixed.Point26_6{X: x, Y: y}}
	prev := rune(-1)
	for _, r := range s {
		if prev >= 0 {
			d.Dot.X += f.Kern(prev, r)
		}
		d.DrawString(string(r))
		d.Dot.X += f.track
		prev = r
	}
}

// box returns the CSS line box of a line-height lh (in px) and the baseline
// offset from its top: half-leading spreads the extra space evenly.
func (f face) baseline(lh float64) fixed.Int26_6 {
	m := f.Metrics()
	content := m.Ascent + m.Descent
	return fixed.Int26_6((lh*64-float64(content))/2) + m.Ascent
}

// normal is line-height: normal, the font's ascent + descent + line gap.
func (f face) normal() float64 {
	m := f.Metrics()
	return float64(m.Height) / 64
}

func px(v float64) fixed.Int26_6 { return fixed.Int26_6(v * 64) }

// Render draws c as a PNG.
func Render(c Card) ([]byte, error) {
	fs, err := loadFonts()
	if err != nil {
		return nil, err
	}
	kindF, err := newFace(fs.mono400, kindSize, 0)
	if err != nil {
		return nil, err
	}
	titleF, err := newFace(fs.sans600, titleSize, titleTrack)
	if err != nil {
		return nil, err
	}
	markF, err := newFace(fs.mono700, wordmarkSize, wordmarkTrack)
	if err != nil {
		return nil, err
	}
	metaF, err := newFace(fs.mono400, metaSize, 0)
	if err != nil {
		return nil, err
	}

	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	drawGrid(img)

	// Top row: dot, kind, "·", date; vertically centered on the text line.
	top := float64(padY)
	if c.Kind != "" {
		lh := kindF.normal()
		base := px(top) + kindF.baseline(lh)
		dc := kindDot[c.Kind]
		if dc.A == 0 {
			dc = kindDot["post"]
		}
		fillCircle(img, padX+dotSize/2.0, top+lh/2, dotSize/2.0, dc)
		x := px(padX + dotSize + kindGap)
		for i, part := range []string{c.Kind, "·", c.Date} {
			if part == "" {
				continue
			}
			if i > 0 {
				x += px(kindGap)
			}
			kindF.draw(img, part, x, base, muted)
			x += kindF.width(part)
		}
		top += lh
	}

	// Bottom row: wordmark left, meta right, aligned on their box bottoms.
	bottom := float64(Height - padY)
	markLH := markF.normal()
	markTop := bottom - markLH
	markBase := px(markTop) + markF.baseline(markLH)
	markX := px(padX)
	word := "halid"
	markF.draw(img, word, markX-px(aberration), markBase, alpha(rose, .75))
	markF.draw(img, word, markX+px(aberration), markBase, alpha(cyan, .75))
	markF.draw(img, word, markX, markBase, text)
	markF.draw(img, ".", markX+markF.width(word), markBase, rose)
	if c.Meta != "" {
		metaLH := metaF.normal()
		metaBase := px(bottom-metaLH) + metaF.baseline(metaLH)
		metaF.draw(img, c.Meta, px(Width-padX)-metaF.width(c.Meta), metaBase, muted)
	}

	// Title: centered between the rows (margin: auto 0).
	lines := wrap(titleF, c.Title, px(titleMax), titleLines)
	lineH := titleSize * titleLH
	blockTop := top + (markTop-top-lineH*float64(len(lines)))/2
	for i, l := range lines {
		base := px(blockTop+lineH*float64(i)) + titleF.baseline(lineH)
		titleF.draw(img, l, px(padX), base, text)
	}

	var buf bytes.Buffer
	enc := png.Encoder{CompressionLevel: png.DefaultCompression}
	if err := enc.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("og: encode: %w", err)
	}
	return buf.Bytes(), nil
}

// drawGrid paints 1px lines every 48px at half opacity, like the template's
// two linear-gradients over a 48px background-size.
func drawGrid(img *image.RGBA) {
	line := image.NewUniform(alpha(grid, .5))
	for x := 0; x < Width; x += gridStep {
		draw.Draw(img, image.Rect(x, 0, x+1, Height), line, image.Point{}, draw.Over)
	}
	for y := 0; y < Height; y += gridStep {
		draw.Draw(img, image.Rect(0, y, Width, y+1), line, image.Point{}, draw.Over)
	}
}

// fillCircle draws an anti-aliased disc by coverage sampling.
func fillCircle(img *image.RGBA, cx, cy, r float64, c color.RGBA) {
	const n = 4 // 4x4 samples per pixel
	for y := int(cy - r - 1); y <= int(cy+r+1); y++ {
		for x := int(cx - r - 1); x <= int(cx+r+1); x++ {
			hit := 0
			for sy := 0; sy < n; sy++ {
				for sx := 0; sx < n; sx++ {
					dx := float64(x) + (float64(sx)+.5)/n - cx
					dy := float64(y) + (float64(sy)+.5)/n - cy
					if dx*dx+dy*dy <= r*r {
						hit++
					}
				}
			}
			if hit > 0 {
				a := float64(hit) / (n * n)
				draw.Draw(img, image.Rect(x, y, x+1, y+1), image.NewUniform(alpha(c, a)), image.Point{}, draw.Over)
			}
		}
	}
}

// wrap breaks s into lines no wider than max, balanced like
// text-wrap: balance, and cut to at most maxLines with an ellipsis.
func wrap(f face, s string, max fixed.Int26_6, maxLines int) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return nil
	}
	lines := greedy(f, words, max)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] = ellipsize(f, lines[maxLines-1]+"…", max)
		return lines
	}
	// Balance: the narrowest width that still needs the same number of lines.
	lo, hi := fixed.Int26_6(0), max
	for hi-lo > 64 {
		mid := (lo + hi) / 2
		if len(greedy(f, words, mid)) <= len(lines) {
			hi = mid
		} else {
			lo = mid
		}
	}
	return greedy(f, words, hi)
}

// greedy fills each line with as many words as fit. A word wider than max
// is broken between characters.
func greedy(f face, words []string, max fixed.Int26_6) []string {
	var lines []string
	cur := ""
	for _, w := range words {
		try := w
		if cur != "" {
			try = cur + " " + w
		}
		if f.width(try) <= max {
			cur = try
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		for f.width(w) > max && utf8.RuneCountInString(w) > 1 {
			head := fit(f, w, max)
			lines = append(lines, head)
			w = w[len(head):]
		}
		cur = w
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	return lines
}

// fit returns the longest prefix of w (at least one character) within max.
func fit(f face, w string, max fixed.Int26_6) string {
	end := 0
	for i, r := range w {
		next := i + utf8.RuneLen(r)
		if end > 0 && f.width(w[:next]) > max {
			break
		}
		end = next
	}
	return w[:end]
}

// ellipsize shortens s (which ends in "…") until it fits, dropping whole
// words first so the cut does not land inside one.
func ellipsize(f face, s string, max fixed.Int26_6) string {
	body := strings.TrimSuffix(s, "…")
	for f.width(body+"…") > max {
		if i := strings.LastIndexByte(body, ' '); i > 0 {
			body = body[:i]
			continue
		}
		_, n := utf8.DecodeLastRuneInString(body)
		if n == 0 {
			break
		}
		body = body[:len(body)-n]
	}
	return strings.TrimRight(body, " ,.;:-") + "…"
}

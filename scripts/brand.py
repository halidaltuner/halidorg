#!/usr/bin/env python3
"""Draws the brand assets (Phase 7.4) from the wordmark in DESIGN.md: "halid"
in JetBrains Mono 700, letter-spacing -0.03em, and a "." in the accent rose.

Writes, from the vendored font, no network:
  app/web/static/brand/wordmark.svg       light: ink letters, rose dot
  app/web/static/brand/wordmark-dark.svg  dark bands: light letters, light rose dot
  app/web/static/brand/monogram.svg       "h." on the dark tile, the favicon
  app/web/static/brand/maskable.svg       the tile with the safe-zone padding
  app/web/templates/partials/wordmark.html  the inline SVG the templates use

The PNG sizes and favicon.ico are rendered from the SVGs by
scripts/brand-png.mjs (Chromium). Needs fonttools and brotli:
  python3 -m venv /tmp/v && /tmp/v/bin/pip install "fonttools[woff]==4.60.1" brotli
  /tmp/v/bin/python scripts/brand.py && node scripts/brand-png.mjs
"""
import os
from fontTools.ttLib import TTFont
from fontTools.pens.svgPathPen import SVGPathPen
from fontTools.pens.boundsPen import BoundsPen
from fontTools.pens.transformPen import TransformPen

ROOT = os.path.join(os.path.dirname(os.path.abspath(__file__)), "..")
FONT = os.path.join(ROOT, "app/web/static/fonts/jetbrains-mono-latin-700-normal.woff2")
OUT = os.path.join(ROOT, "app/web/static/brand")

# Colors are the tokens' values (tokens.css): these files are used outside
# the site's CSS (feeds, browsers, OS icons), where var() does not exist.
INK, ROSE = "#0E1525", "#E11D48"  # --text, --accent-2
DK_BG, DK_TEXT, DK_ROSE, DK_CYAN = "#0B1220", "#E6EBF2", "#FB7185", "#22D3EE"

font = TTFont(FONT)
gs = font.getGlyphSet()
cmap = font.getBestCmap()
upem = font["head"].unitsPerEm


def glyph(ch, x, scale=1.0):
    """SVG path data for ch with its origin at x (font units), y flipped."""
    name = cmap[ord(ch)]
    pen = SVGPathPen(gs)
    gs[name].draw(TransformPen(pen, (scale, 0, 0, -scale, x, 0)))
    return pen.getCommands(), gs[name].width * scale


def bounds(ch, x):
    bp = BoundsPen(gs)
    gs[cmap[ord(ch)]].draw(TransformPen(bp, (1, 0, 0, -1, x, 0)))
    return bp.bounds


def run(text, x=0.0):
    """Path data and bounds for text set with the wordmark's tracking."""
    track = -0.03 * upem
    d, box = [], [1e9, 1e9, -1e9, -1e9]
    for ch in text:
        g, w = glyph(ch, x)
        d.append(g)
        b = bounds(ch, x)
        if b:
            box = [min(box[0], b[0]), min(box[1], b[1]), max(box[2], b[2]), max(box[3], b[3])]
        x += w + track
    return " ".join(d), box, x


letters, lb, x = run("halid")
dot, db, _ = run(".", x)
x0, y0 = lb[0], min(lb[1], db[1])
x1, y1 = max(lb[2], db[2]), max(lb[3], db[3])
pad = 20
vb = f"{x0 - pad:.0f} {y0 - pad:.0f} {x1 - x0 + 2 * pad:.0f} {y1 - y0 + 2 * pad:.0f}"
ratio = (x1 - x0 + 2 * pad) / (y1 - y0 + 2 * pad)

os.makedirs(OUT, exist_ok=True)


def write(path, s):
    with open(path, "w") as f:
        f.write(s)


def wordmark(fg, dotc):
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="{vb}" role="img" aria-label="halid.">'
            f'<path fill="{fg}" d="{letters}"/><path fill="{dotc}" d="{dot}"/></svg>\n')


write(os.path.join(OUT, "wordmark.svg"), wordmark(INK, ROSE))
write(os.path.join(OUT, "wordmark-dark.svg"), wordmark(DK_TEXT, DK_ROSE))

# The inline partial: currentColor letters and a .dot path the CSS colors,
# so the header, footer and login keep their light and dark variants and
# the aberration (a drop-shadow filter) without loading the font.
write(os.path.join(ROOT, "app/web/templates/partials/wordmark.html"),
      "{{/* wordmark-svg: the SVG wordmark (scripts/brand.py, DESIGN.md §41).\n"
      "   Generated; sized by .wm in app.css (height in em, width from the viewBox). */}}\n"
      '{{define "wordmark-svg"}}<svg class="wm" xmlns="http://www.w3.org/2000/svg" viewBox="' + vb + '"'
      ' aria-hidden="true" focusable="false">'
      f'<path fill="currentColor" d="{letters}"/><path class="wm-dot" d="{dot}"/></svg>{{{{end}}}}\n')

# Monogram: "h." centered on the dark tile, with the wordmark's aberration
# (rose left, cyan right) behind the letter.
h, hb, hx = run("h")
d2, d2b, _ = run(".", hx)
mx0, my0, mx1, my1 = hb[0], min(hb[1], d2b[1]), max(hb[2], d2b[2]), max(hb[3], d2b[3])
mw, mh = mx1 - mx0, my1 - my0


def tile(size, inset, radius):
    s = (size - 2 * inset) / max(mw, mh)
    tx = inset + ((size - 2 * inset) - mw * s) / 2 - mx0 * s
    ty = inset + ((size - 2 * inset) - mh * s) / 2 - my0 * s
    off = size * 0.022
    g = f'transform="translate({tx:.2f} {ty:.2f}) scale({s:.5f})"'
    return (f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {size} {size}">'
            f'<rect width="{size}" height="{size}" rx="{radius}" fill="{DK_BG}"/>'
            f'<g transform="translate({-off:.2f} 0)" opacity=".8"><path fill="{DK_ROSE}" {g} d="{h}"/></g>'
            f'<g transform="translate({off:.2f} 0)" opacity=".8"><path fill="{DK_CYAN}" {g} d="{h}"/></g>'
            f'<path fill="{DK_TEXT}" {g} d="{h}"/><path fill="{DK_ROSE}" {g} d="{d2}"/></svg>\n')


write(os.path.join(OUT, "monogram.svg"), tile(64, 9, 12))
write(os.path.join(OUT, "maskable.svg"), tile(512, 112, 0))  # 80% safe zone, the OS crops
write(os.path.join(OUT, "apple.svg"), tile(180, 30, 0))  # iOS rounds the corners itself
print("wordmark", vb, f"ratio {ratio:.3f}")

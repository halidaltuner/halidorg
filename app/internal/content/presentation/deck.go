package presentation

import (
	"bytes"
	"errors"
	"html"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

// MaxDeckBytes is the largest deck upload: one self-contained HTML file
// with its images inlined.
const MaxDeckBytes = 25 << 20

// Deck upload errors the admin shows.
var (
	ErrDeckType  = errors.New("presentation: the deck must be one HTML file")
	ErrDeckLarge = errors.New("presentation: the deck is too large")
)

// CheckDeck accepts an HTML document of at most MaxDeckBytes.
func CheckDeck(data []byte) error {
	if len(data) > MaxDeckBytes {
		return ErrDeckLarge
	}
	if len(data) == 0 || !utf8.Valid(data) || !strings.HasPrefix(http.DetectContentType(data), "text/html") {
		return ErrDeckType
	}
	return nil
}

var (
	tagRE   = regexp.MustCompile(`<(/?)([a-zA-Z][a-zA-Z0-9-]*)([^>]*)>`)
	classRE = regexp.MustCompile(`(?i)\bclass\s*=\s*("[^"]*"|'[^']*'|[^\s>]+)`)
	spaceRE = regexp.MustCompile(`\s+`)
)

// tag is one start or end tag of a document, in order.
type tag struct {
	end        bool
	name       string
	slide      bool // has the class "slide"
	start, pos int  // where the tag begins and where it ends
}

// tags lists the tags of doc, skipping comments and the insides of script,
// style and template elements, where '<' means nothing.
func tags(doc string) []tag {
	var out []tag
	for i := 0; i < len(doc); {
		if strings.HasPrefix(doc[i:], "<!--") {
			j := strings.Index(doc[i:], "-->")
			if j < 0 {
				break
			}
			i += j + 3
			continue
		}
		m := tagRE.FindStringSubmatchIndex(doc[i:])
		if m == nil {
			break
		}
		if m[0] > 0 && strings.Contains(doc[i:i+m[0]], "<!--") {
			i += strings.Index(doc[i:], "<!--")
			continue
		}
		t := tag{end: m[3] > m[2], name: strings.ToLower(doc[i+m[4] : i+m[5]]), start: i + m[0], pos: i + m[1]}
		if c := classRE.FindStringSubmatch(doc[i+m[6] : i+m[7]]); c != nil {
			t.slide = hasWord(strings.Trim(c[1], `"'`), "slide")
		}
		out = append(out, t)
		i = t.pos
		if !t.end && (t.name == "script" || t.name == "style" || t.name == "template") {
			j := strings.Index(strings.ToLower(doc[i:]), "</"+t.name)
			if j < 0 {
				break
			}
			i += j
		}
	}
	return out
}

func hasWord(s, w string) bool {
	for _, f := range strings.Fields(s) {
		if f == w {
			return true
		}
	}
	return false
}

// Outline reads a deck's slides: the outermost elements with the class
// "slide", else the outermost <section> elements, and the first h1, h2 or
// h3 of each as its title. Slides without a heading count but have no
// outline row. A deck with neither has 0 slides.
func Outline(deck []byte) (count int, outline []Slide) {
	doc := string(bytes.ToValidUTF8(deck, nil))
	ts := tags(doc)
	byClass := false
	for _, t := range ts {
		if !t.end && t.slide {
			byClass = true
			break
		}
	}
	isSlide := func(t tag) bool {
		if byClass {
			return t.slide
		}
		return t.name == "section"
	}
	depth, name := 0, "" // inside a slide: nesting of its tag name
	heading := ""        // inside a heading of the current slide: its name
	titled, hStart := false, 0
	for _, t := range ts {
		if depth == 0 {
			if !t.end && isSlide(t) {
				count++
				depth, name, titled, heading = 1, t.name, false, ""
			}
			continue
		}
		switch {
		case !titled && heading == "" && !t.end && (t.name == "h1" || t.name == "h2" || t.name == "h3"):
			heading, hStart = t.name, t.pos
		case heading != "" && t.end && t.name == heading:
			if title := text(doc[hStart:t.start]); title != "" && len(outline) < 200 {
				outline = append(outline, Slide{N: count, Title: title})
			}
			heading, titled = "", true
		}
		if t.name == name {
			if t.end {
				depth--
			} else {
				depth++
			}
		}
	}
	return count, outline
}

// text is the visible text of an HTML fragment on one line, at most 120
// characters.
func text(frag string) string {
	s := strings.TrimSpace(spaceRE.ReplaceAllString(html.UnescapeString(tagRE.ReplaceAllString(frag, " ")), " "))
	if r := []rune(s); len(r) > 120 {
		s = strings.TrimSpace(string(r[:119])) + "…"
	}
	return s
}

// Bridge is the script a deck pastes before </body> so the page around it
// can show "n / total" and jump to slides (ADR-007). It answers the page's
// postMessage {deck: "go", slide: n}, "next", "prev" and "hello" with
// {deck: "state", slide, total}. A deck that defines window.halidDeck =
// {go(n), current(), total()} is driven through it; otherwise slides are
// the .slide (else section) elements, shown by scrolling when several are
// visible, else stepped with arrow-key presses.
const Bridge = `<script>
/* halid.org deck bridge: the page around the deck shows the slide number
   and jumps to slides from its outline. Paste before </body>. */
(function () {
  if (window.parent === window) return;
  var d = window.halidDeck || {};
  function slides() {
    var s = document.querySelectorAll('.slide');
    return s.length ? s : document.querySelectorAll('section');
  }
  function visible(el) { return el.offsetWidth > 0 && getComputedStyle(el).visibility !== 'hidden'; }
  function total() { return d.total ? d.total() : slides().length; }
  function current() {
    if (d.current) return d.current();
    var s = slides(), shown = [], best = 1, top = Infinity;
    for (var i = 0; i < s.length; i++) {
      if (/\b(active|current|present)\b/.test(s[i].className)) return i + 1;
      if (visible(s[i])) shown.push(i);
    }
    if (shown.length === 1) return shown[0] + 1;
    shown.forEach(function (i) {
      var t = Math.abs(s[i].getBoundingClientRect().top);
      if (t < top) { top = t; best = i + 1; }
    });
    return best;
  }
  function key(k) { document.dispatchEvent(new KeyboardEvent('keydown', { key: k, code: k, bubbles: true })); }
  function go(n) {
    n = Math.max(1, Math.min(total(), n));
    if (d.go) return d.go(n);
    var s = slides(), shown = 0;
    for (var i = 0; i < s.length; i++) if (visible(s[i])) shown++;
    if (shown > 1) { if (s[n - 1]) s[n - 1].scrollIntoView(); return; }
    for (var c = current(), guard = 0; c !== n && guard < 2000; guard++) {
      key(n > c ? 'ArrowRight' : 'ArrowLeft');
      var next = current();
      if (next === c) break;
      c = next;
    }
  }
  var last = 0;
  function tell() {
    var n = current();
    if (n === last) return;
    last = n;
    window.parent.postMessage({ deck: 'state', slide: n, total: total() }, '*');
  }
  function later() { setTimeout(tell, 60); }
  window.addEventListener('message', function (e) {
    var m = e.data;
    if (e.source !== window.parent || !m || typeof m.deck !== 'string') return;
    if (m.deck === 'go') go(m.slide | 0);
    if (m.deck === 'next') go(current() + 1);
    if (m.deck === 'prev') go(current() - 1);
    if (m.deck === 'hello') last = 0;
    later();
  });
  ['keyup', 'click', 'scroll', 'hashchange'].forEach(function (t) { window.addEventListener(t, later, true); });
  later();
})();
</script>`

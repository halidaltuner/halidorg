package webmention

import (
	"bytes"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"golang.org/x/net/html"
	"willnorris.com/go/microformats"
)

// Parsed is what a source page says about the target.
type Parsed struct {
	Linked    bool   // the source links to the target; without it there is no mention
	Type      string // reply, like, repost or mention
	Author    string
	AuthorURL string
	Photo     string // the author's avatar URL on the source's site
	Content   string // plain text, at most MaxContent runes
	Published *int64
}

// MaxContent bounds the text kept from a reply or mention.
const MaxContent = 500

// Parse reads a fetched source. base is its URL after redirects; target is
// the page it should link to. HTML is checked for a link and read for its
// h-entry (microformats2); any other text only needs to contain the target.
func Parse(body []byte, contentType string, base *url.URL, target string) Parsed {
	p := Parsed{Type: Mention}
	want := normalize(target)
	if want == "" {
		return p
	}
	if !isHTML(contentType, body) {
		p.Linked = bytes.Contains(body, []byte(target))
		p.Author = Host(base.String())
		return p
	}
	doc, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return p
	}
	p.Linked = linksTo(doc, base, want)
	if !p.Linked {
		return p
	}
	data := microformats.ParseNode(doc, base)
	entry, typ := pickEntry(data.Items, want)
	if typ != "" {
		p.Type = typ
	}
	if entry != nil {
		p.Author, p.AuthorURL, p.Photo = card(first(entry.Properties["author"]))
		p.Content = text(entry)
		if s, ok := first(entry.Properties["published"]).(string); ok {
			p.Published = parseTime(s)
		}
	}
	if p.Author == "" && p.Photo == "" {
		// The page's own h-card (a profile in the header or the sidebar).
		for _, it := range data.Items {
			if has(it.Type, "h-card") {
				p.Author, p.AuthorURL, p.Photo = card(it)
				break
			}
		}
	}
	if p.Author == "" {
		p.Author = Host(base.String())
	}
	return p
}

func isHTML(contentType string, body []byte) bool {
	ct := strings.ToLower(contentType)
	if strings.Contains(ct, "html") {
		return true
	}
	if ct != "" && !strings.HasPrefix(ct, "application/octet-stream") {
		return false
	}
	head := strings.ToLower(string(body[:min(len(body), 512)]))
	return strings.Contains(head, "<html") || strings.Contains(head, "<!doctype html")
}

// normalize makes two spellings of one URL compare equal: no fragment, a
// lowercase host, no trailing slash, http and https alike.
func normalize(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	path := strings.TrimRight(u.EscapedPath(), "/")
	s := strings.ToLower(u.Host) + path
	if u.RawQuery != "" {
		s += "?" + u.RawQuery
	}
	return s
}

// linksTo reports whether a link or embed in doc points at want.
func linksTo(doc *html.Node, base *url.URL, want string) bool {
	attrs := map[string]string{"a": "href", "link": "href", "img": "src", "video": "src", "audio": "src", "source": "src"}
	found := false
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if found {
			return
		}
		if n.Type == html.ElementNode {
			if key, ok := attrs[n.Data]; ok {
				for _, a := range n.Attr {
					if a.Key != key {
						continue
					}
					if u, err := base.Parse(strings.TrimSpace(a.Val)); err == nil && normalize(u.String()) == want {
						found = true
						return
					}
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(doc)
	return found
}

// responses are the h-entry properties that make a mention a reply, like or
// repost, in the order they win.
var responses = []struct{ prop, typ string }{
	{"in-reply-to", Reply}, {"like-of", Like}, {"repost-of", Repost},
}

// pickEntry finds the h-entry about the target: one replying to, liking or
// reposting it, else the first h-entry on the page (a mention).
func pickEntry(items []*microformats.Microformat, want string) (*microformats.Microformat, string) {
	var entries []*microformats.Microformat
	var collect func([]*microformats.Microformat)
	collect = func(list []*microformats.Microformat) {
		for _, it := range list {
			if has(it.Type, "h-entry") {
				entries = append(entries, it)
			}
			collect(it.Children)
		}
	}
	collect(items)
	for _, e := range entries {
		for _, r := range responses {
			for _, v := range e.Properties[r.prop] {
				if normalize(urlOf(v)) == want {
					return e, r.typ
				}
			}
		}
	}
	if len(entries) > 0 {
		return entries[0], ""
	}
	return nil, ""
}

// urlOf is the URL a property value names: a plain u-* value, or an
// embedded h-cite's url.
func urlOf(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case *microformats.Microformat:
		if u, ok := first(x.Properties["url"]).(string); ok {
			return u
		}
		return x.Value
	case map[string]string:
		return x["value"]
	}
	return ""
}

// card reads an author: an h-card's name, url and photo, or a plain name
// or URL.
func card(v any) (name, link, photo string) {
	switch x := v.(type) {
	case string:
		if u, err := url.Parse(x); err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
			return Host(x), x, ""
		}
		return clean(x, 80), "", ""
	case *microformats.Microformat:
		name = clean(str(first(x.Properties["name"])), 80)
		if name == "" {
			name = clean(x.Value, 80)
		}
		link = str(first(x.Properties["url"]))
		photo = str(first(x.Properties["photo"]))
		if name == link || name == "" {
			name = Host(link)
		}
		return name, link, photo
	}
	return "", "", ""
}

func str(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case map[string]string:
		return x["value"]
	case *microformats.Microformat:
		return x.Value
	}
	return ""
}

// text is the entry's words: its content, else its summary, else its name.
func text(e *microformats.Microformat) string {
	for _, prop := range []string{"content", "summary", "name"} {
		if s := clean(str(first(e.Properties[prop])), MaxContent); s != "" {
			return s
		}
	}
	return ""
}

// clean collapses whitespace and cuts s to max runes.
func clean(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

var timeLayouts = []string{time.RFC3339, "2006-01-02T15:04:05-0700", "2006-01-02T15:04:05", "2006-01-02 15:04:05-07:00", "2006-01-02 15:04:05", "2006-01-02"}

func parseTime(s string) *int64 {
	s = strings.TrimSpace(s)
	for _, l := range timeLayouts {
		if t, err := time.Parse(l, s); err == nil {
			u := t.UTC().Unix()
			return &u
		}
	}
	return nil
}

func first(vs []any) any {
	if len(vs) == 0 {
		return nil
	}
	return vs[0]
}

func has(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// Host is the site a URL is on, without www.: what the post page shows as
// the source.
func Host(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}

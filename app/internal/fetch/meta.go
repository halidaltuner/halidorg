package fetch

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Meta is what Quick Capture shows for a pasted URL.
type Meta struct {
	URL         string // after redirects
	Title       string
	Description string
}

// maxMetaPage is how much of a page is read for its <head>.
const maxMetaPage = 512 << 10

var (
	titleTag = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)
	metaTag  = regexp.MustCompile(`(?is)<meta\b[^>]*>`)
	metaAttr = regexp.MustCompile(`(?is)\b(name|property|content)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
	spaces   = regexp.MustCompile(`\s+`)
)

// ErrNotHTML means the URL answered with something other than a page.
var ErrNotHTML = errors.New("fetch: not an html page")

// Meta fetches the first part of a page and reads its title and
// description, Open Graph first. The checks of Get apply: public http(s)
// hosts only, a timeout, and a size cap.
func (c *Client) Meta(ctx context.Context, raw string) (Meta, error) {
	res, err := c.GetPrefix(ctx, raw, maxMetaPage)
	if err != nil {
		return Meta{}, err
	}
	if res.Status != http.StatusOK {
		return Meta{URL: res.URL.String()}, fmt.Errorf("fetch: status %d", res.Status)
	}
	ct := strings.ToLower(res.ContentType)
	if ct != "" && !strings.Contains(ct, "html") {
		return Meta{URL: res.URL.String()}, ErrNotHTML
	}
	m := ParseMeta(res.Body)
	m.URL = res.URL.String()
	return m, nil
}

// ParseMeta reads title and description from a page's head: og:title, else
// twitter:title, else <title>; og:description, else description.
func ParseMeta(body []byte) Meta {
	head := body
	if i := strings.Index(strings.ToLower(string(body)), "</head>"); i > 0 {
		head = body[:i]
	}
	props := map[string]string{}
	for _, tag := range metaTag.FindAll(head, -1) {
		var key, content string
		has := false
		for _, m := range metaAttr.FindAllSubmatch(tag, -1) {
			v := string(m[2]) + string(m[3]) + string(m[4])
			switch strings.ToLower(string(m[1])) {
			case "name", "property":
				key = strings.ToLower(strings.TrimSpace(v))
			case "content":
				content, has = v, true
			}
		}
		if key != "" && has {
			if _, seen := props[key]; !seen {
				props[key] = content
			}
		}
	}
	title := first(props["og:title"], props["twitter:title"])
	if title == "" {
		if m := titleTag.FindSubmatch(head); m != nil {
			title = string(m[1])
		}
	}
	return Meta{
		Title:       clean(title, 200),
		Description: clean(first(props["og:description"], props["description"], props["twitter:description"]), 300),
	}
}

func first(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// clean decodes entities, folds whitespace, drops invalid UTF-8 and cuts at
// max runes on a word boundary.
func clean(s string, max int) string {
	s = strings.ToValidUTF8(html.UnescapeString(s), "")
	s = strings.TrimSpace(spaces.ReplaceAllString(s, " "))
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)[:max]
	cut := string(r)
	if i := strings.LastIndex(cut, " "); i > max/2 {
		cut = cut[:i]
	}
	return strings.TrimRight(cut, " ,.;:") + "…"
}

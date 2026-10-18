package fetch

import (
	"context"
	"errors"
	"html"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ErrNoFavicon means the site offers no usable icon.
var ErrNoFavicon = errors.New("fetch: no favicon")

// MaxFavicon bounds the icon size; a favicon is a few KB.
const MaxFavicon = 100 << 10

// maxPage is how much of the home page is read to find <link rel=icon>.
const maxPage = 512 << 10

var (
	linkTag = regexp.MustCompile(`(?is)<link\b[^>]*>`)
	attr    = regexp.MustCompile(`(?is)\b(rel|href|type)\s*=\s*(?:"([^"]*)"|'([^']*)'|([^\s>]+))`)
)

// iconTypes are the image types a favicon may have and the extension it is
// stored under. SVG is not one: media is raster only (CLAUDE.md).
var iconTypes = map[string]string{
	"image/png": "png", "image/jpeg": "jpg", "image/gif": "gif", "image/webp": "webp", "image/x-icon": "ico",
}

// IconExt sniffs b and returns the extension to store it under, or "".
func IconExt(b []byte) string {
	return iconTypes[http.DetectContentType(b)]
}

// Favicon finds the icon of the site at domain: the <link rel="icon"> of
// its home page (shortcut and apple-touch icons too), else /favicon.ico.
// The bytes are sniffed; anything but a raster image is refused.
func (c *Client) Favicon(ctx context.Context, domain string) ([]byte, string, error) {
	base := &url.URL{Scheme: "https", Host: domain, Path: "/"}
	var candidates []string
	if res, err := c.Get(ctx, base.String(), maxPage); err == nil && res.Status == http.StatusOK {
		candidates = iconLinks(res.URL, res.Body)
	} else if errors.Is(err, ErrBlocked) {
		return nil, "", err
	}
	candidates = append(candidates, base.ResolveReference(&url.URL{Path: "/favicon.ico"}).String())
	for _, u := range candidates {
		res, err := c.Get(ctx, u, MaxFavicon)
		if err != nil || res.Status != http.StatusOK {
			continue
		}
		if ext := IconExt(res.Body); ext != "" {
			return res.Body, ext, nil
		}
	}
	return nil, "", ErrNoFavicon
}

// iconLinks returns the icon URLs a page declares, raster ones first in
// the order icon, shortcut icon, apple-touch-icon.
func iconLinks(page *url.URL, body []byte) []string {
	head := body
	if i := strings.Index(strings.ToLower(string(body)), "</head>"); i > 0 {
		head = body[:i]
	}
	rank := map[string]int{"icon": 0, "shortcut icon": 1, "apple-touch-icon": 2, "apple-touch-icon-precomposed": 3}
	found := make([][]string, 4)
	for _, tag := range linkTag.FindAll(head, -1) {
		var rel, href, typ string
		for _, m := range attr.FindAllSubmatch(tag, -1) {
			v := html.UnescapeString(string(m[2]) + string(m[3]) + string(m[4]))
			switch strings.ToLower(string(m[1])) {
			case "rel":
				rel = strings.ToLower(strings.Join(strings.Fields(v), " "))
			case "href":
				href = strings.TrimSpace(v)
			case "type":
				typ = strings.ToLower(v)
			}
		}
		r, ok := rank[rel]
		if !ok || href == "" || typ == "image/svg+xml" || strings.HasSuffix(strings.ToLower(strings.SplitN(href, "?", 2)[0]), ".svg") {
			continue
		}
		u, err := page.Parse(href)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") {
			continue
		}
		found[r] = append(found[r], u.String())
	}
	var out []string
	for _, f := range found {
		out = append(out, f...)
	}
	return out
}

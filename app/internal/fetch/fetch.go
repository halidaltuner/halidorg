// Package fetch gets URLs that came from content (favicons now, Quick
// Capture titles in Phase 2.4) without letting them reach anything private.
//
// The container's only internal host is bridge.internal, which the Worker
// intercepts (ADR-001). Requests here never go to it: hosts ending in
// .internal or .localhost and IP literals are refused before dialing, and
// the dialer refuses private, loopback, link-local and other non-public
// addresses after DNS, so a public name that resolves inward fails too.
// Every redirect is checked the same way.
package fetch

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"syscall"
	"time"
)

// ErrBlocked is returned for URLs the client will not fetch.
var ErrBlocked = errors.New("fetch: url not allowed")

// ErrTooLarge is returned when a body exceeds the limit.
var ErrTooLarge = errors.New("fetch: body too large")

// Client fetches public http(s) URLs with a timeout and a size limit.
type Client struct {
	hc *http.Client
	// allow reports whether an address may be dialed. Public addresses
	// only, except in tests.
	allow func(netip.Addr) bool
}

// UserAgent identifies the fetcher to sites.
const UserAgent = "halid.org-fetch/1 (+https://halid.org)"

// New returns a client with the given timeout per request, redirects included.
func New(timeout time.Duration) *Client {
	c := &Client{allow: Public}
	d := &net.Dialer{Timeout: timeout, Control: c.control}
	tr := &http.Transport{
		Proxy:                 nil, // never through a proxy from the environment
		DialContext:           d.DialContext,
		TLSHandshakeTimeout:   timeout,
		ResponseHeaderTimeout: timeout,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
	}
	c.hc = &http.Client{
		Timeout:   timeout,
		Transport: tr,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("%w: too many redirects", ErrBlocked)
			}
			return Check(req.URL)
		},
	}
	return c
}

// Check refuses URLs that are not plain http(s) to a named public host on
// the default ports.
func Check(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.Opaque != "" {
		return fmt.Errorf("%w: scheme", ErrBlocked)
	}
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || !strings.Contains(host, ".") || host == "localhost" ||
		strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".local") {
		return fmt.Errorf("%w: host %q", ErrBlocked, host)
	}
	if _, err := netip.ParseAddr(host); err == nil {
		return fmt.Errorf("%w: ip literal", ErrBlocked)
	}
	if p := u.Port(); p != "" && p != "80" && p != "443" {
		return fmt.Errorf("%w: port %s", ErrBlocked, p)
	}
	return nil
}

// nonPublic are ranges netip's predicates do not cover.
var nonPublic = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),       // "this network"
	netip.MustParsePrefix("100.64.0.0/10"),   // carrier-grade NAT
	netip.MustParsePrefix("192.0.0.0/24"),    // IETF protocol assignments
	netip.MustParsePrefix("192.0.2.0/24"),    // documentation
	netip.MustParsePrefix("198.18.0.0/15"),   // benchmarking
	netip.MustParsePrefix("198.51.100.0/24"), // documentation
	netip.MustParsePrefix("203.0.113.0/24"),  // documentation
	netip.MustParsePrefix("240.0.0.0/4"),     // reserved, broadcast
	netip.MustParsePrefix("64:ff9b::/96"),    // NAT64 can map to any IPv4
	netip.MustParsePrefix("2001:db8::/32"),   // documentation
}

// Public reports whether a is a public unicast address.
func Public(a netip.Addr) bool {
	a = a.Unmap()
	if !a.IsValid() || a.IsLoopback() || a.IsPrivate() || a.IsUnspecified() || a.IsLinkLocalUnicast() ||
		a.IsLinkLocalMulticast() || a.IsInterfaceLocalMulticast() || a.IsMulticast() {
		return false
	}
	for _, p := range nonPublic {
		if p.Contains(a) {
			return false
		}
	}
	return true
}

// control runs after DNS, on the address actually dialed.
func (c *Client) control(_, address string, _ syscall.RawConn) error {
	ap, err := netip.ParseAddrPort(address)
	if err != nil {
		return fmt.Errorf("%w: %s", ErrBlocked, address)
	}
	if !c.allow(ap.Addr()) {
		return fmt.Errorf("%w: address %s", ErrBlocked, ap.Addr())
	}
	return nil
}

// Response is a fetched body.
type Response struct {
	URL         *url.URL // after redirects
	Status      int
	ContentType string // from the server; sniff before trusting it
	Header      http.Header
	Body        []byte
}

// Get fetches raw and reads at most max bytes of the body. A larger body
// is ErrTooLarge. Any status is returned; callers decide what is usable.
func (c *Client) Get(ctx context.Context, raw string, max int64) (*Response, error) {
	return c.get(ctx, raw, max, false)
}

// GetPrefix is Get that keeps the first max bytes of a larger body instead
// of failing: enough for a page's <head>.
func (c *Client) GetPrefix(ctx context.Context, raw string, max int64) (*Response, error) {
	return c.get(ctx, raw, max, true)
}

// PostForm posts form, url-encoded, to raw and reads at most max bytes of
// the answer (a larger one is cut, not an error). Redirects are followed
// as GETs, each checked like Get's.
func (c *Client) PostForm(ctx context.Context, raw string, form url.Values, max int64) (*Response, error) {
	return c.do(ctx, http.MethodPost, raw, form, max, true)
}

func (c *Client) get(ctx context.Context, raw string, max int64, prefix bool) (*Response, error) {
	return c.do(ctx, http.MethodGet, raw, nil, max, prefix)
}

func (c *Client) do(ctx context.Context, method, raw string, form url.Values, max int64, prefix bool) (*Response, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrBlocked, err)
	}
	if err := Check(u); err != nil {
		return nil, err
	}
	var body io.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	}
	req, err := http.NewRequestWithContext(ctx, method, u.String(), body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "*/*")
	if form != nil {
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	res, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.ContentLength > max && !prefix {
		return nil, ErrTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(res.Body, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		if !prefix {
			return nil, ErrTooLarge
		}
		data = data[:max]
	}
	return &Response{URL: res.Request.URL, Status: res.StatusCode, ContentType: res.Header.Get("Content-Type"), Header: res.Header, Body: data}, nil
}

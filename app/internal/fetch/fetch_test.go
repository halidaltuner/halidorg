package fetch

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCheck(t *testing.T) {
	ok := []string{"https://grugbrain.dev/", "http://example.com/a?b=c", "https://example.com:443/x"}
	bad := []string{
		"file:///etc/passwd", "gopher://example.com/", "ftp://example.com/", "javascript:alert(1)",
		"http://localhost/", "http://foo.localhost/", "http://bridge.internal/d1/query", "http://printer.local/",
		"http://127.0.0.1/", "http://[::1]/", "http://10.0.0.1/", "http://169.254.169.254/latest/meta-data",
		"http://user:pw@example.com/", "http://example.com:8080/", "http://intranet/", "https:example.com",
	}
	for _, s := range ok {
		u, _ := url.Parse(s)
		if err := Check(u); err != nil {
			t.Errorf("%s refused: %v", s, err)
		}
	}
	for _, s := range bad {
		u, _ := url.Parse(s)
		if err := Check(u); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s allowed", s)
		}
	}
}

func TestPublic(t *testing.T) {
	public := []string{"1.1.1.1", "93.184.216.34", "2606:4700:4700::1111"}
	private := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "192.168.1.1", "169.254.169.254", "0.0.0.0", "100.64.0.1",
		"::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1", "224.0.0.1", "255.255.255.255", "64:ff9b::a00:1",
	}
	for _, s := range public {
		if !Public(netip.MustParseAddr(s)) {
			t.Errorf("%s not public", s)
		}
	}
	for _, s := range private {
		if Public(netip.MustParseAddr(s)) {
			t.Errorf("%s public", s)
		}
	}
}

// clientTo routes every host to srv, so names that pass Check still reach a
// loopback test server and the dial-time check is exercised for real.
func clientTo(t *testing.T, srv *httptest.Server, allow func(netip.Addr) bool) *Client {
	t.Helper()
	c := New(2 * time.Second)
	c.allow = allow
	d := &net.Dialer{Timeout: time.Second, Control: c.control}
	addr := srv.Listener.Addr().String()
	c.hc.Transport.(*http.Transport).DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return d.DialContext(ctx, network, addr)
	}
	return c
}

func TestDialRefusesPrivateAddresses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("hi")) }))
	defer srv.Close()
	// A public-looking name that resolves to loopback.
	if _, err := clientTo(t, srv, Public).Get(context.Background(), "http://rebind.example.com/", 1<<10); !errors.Is(err, ErrBlocked) {
		t.Fatalf("loopback dial allowed: %v", err)
	}
	res, err := clientTo(t, srv, func(netip.Addr) bool { return true }).Get(context.Background(), "http://ok.example.com/", 1<<10)
	if err != nil || string(res.Body) != "hi" {
		t.Fatalf("allowed fetch: %v", err)
	}
}

func TestRedirectsAndLimits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/to-internal":
			http.Redirect(w, r, "http://bridge.internal/kv/x", http.StatusFound)
		case "/to-file":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		case "/hop":
			http.Redirect(w, r, "http://other.example.com/big", http.StatusFound)
		case "/big":
			_, _ = w.Write([]byte(strings.Repeat("x", 2048)))
		}
	}))
	defer srv.Close()
	c := clientTo(t, srv, func(netip.Addr) bool { return true })
	ctx := context.Background()
	for _, p := range []string{"/to-internal", "/to-file", "/loop"} {
		if _, err := c.Get(ctx, "http://a.example.com"+p, 1<<10); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: %v", p, err)
		}
	}
	if _, err := c.Get(ctx, "http://a.example.com/hop", 1<<10); !errors.Is(err, ErrTooLarge) {
		t.Errorf("size limit: %v", err)
	}
	res, err := c.Get(ctx, "http://a.example.com/hop", 4<<10)
	if err != nil || res.URL.Host != "other.example.com" || len(res.Body) != 2048 {
		t.Errorf("redirect followed: %v %+v", err, res)
	}
}

func TestPostForm(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		w.Header().Set("Link", `<https://a.example.com/wm>; rel="webmention"`)
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(r.Method + " " + r.Header.Get("Content-Type") + " " + r.PostForm.Get("source")))
	}))
	defer srv.Close()
	c := clientTo(t, srv, func(netip.Addr) bool { return true })
	res, err := c.PostForm(context.Background(), "http://a.example.com/wm", url.Values{"source": {"https://halid.org/en/posts/x"}}, 1<<10)
	if err != nil || res.Status != http.StatusAccepted || string(res.Body) != "POST application/x-www-form-urlencoded https://halid.org/en/posts/x" {
		t.Fatalf("post: %v %+v", err, res)
	}
	if res.Header.Get("Link") == "" {
		t.Error("headers not returned")
	}
	if _, err := c.PostForm(context.Background(), "http://localhost/wm", url.Values{}, 1<<10); !errors.Is(err, ErrBlocked) {
		t.Errorf("localhost post: %v", err)
	}
}

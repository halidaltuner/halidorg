package fetch

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestParseMeta(t *testing.T) {
	m := ParseMeta([]byte(`<html><head>
<title>  Fallback   title </title>
<meta property="og:title" content="Hypermedia: A Reintroduction &amp; more">
<meta name="description" content='Plain description'>
<meta content="OG first" property=og:description>
</head><body><meta property="og:title" content="late"></body></html>`))
	if m.Title != "Hypermedia: A Reintroduction & more" || m.Description != "OG first" {
		t.Errorf("%+v", m)
	}
	m = ParseMeta([]byte("<title>Ghost’tan ayrılmak\n</title><meta name=description content=\"\">"))
	if m.Title != "Ghost’tan ayrılmak" || m.Description != "" {
		t.Errorf("fallback %+v", m)
	}
	long := strings.Repeat("word ", 100)
	if m := ParseMeta([]byte("<title>" + long + "</title>")); len([]rune(m.Title)) > 201 || !strings.HasSuffix(m.Title, "…") {
		t.Errorf("not cut: %q", m.Title)
	}
	if m := ParseMeta([]byte("<title>bad \xff utf8</title>")); m.Title != "bad utf8" {
		t.Errorf("invalid utf8: %q", m.Title)
	}
}

func TestMetaFetch(t *testing.T) {
	big := "<html><head><title>Big page</title></head><body>" + strings.Repeat("x", maxMetaPage) + "</body></html>"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(big))
		case "/pdf":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF"))
		case "/moved":
			http.Redirect(w, r, "http://b.example.com/big", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientTo(t, srv, func(netip.Addr) bool { return true })
	ctx := context.Background()
	m, err := c.Meta(ctx, "http://a.example.com/moved")
	if err != nil || m.Title != "Big page" || m.URL != "http://b.example.com/big" {
		t.Fatalf("big page through a redirect: %+v %v", m, err)
	}
	if _, err := c.Meta(ctx, "http://a.example.com/pdf"); !errors.Is(err, ErrNotHTML) {
		t.Errorf("pdf: %v", err)
	}
	if _, err := c.Meta(ctx, "http://a.example.com/missing"); err == nil {
		t.Error("404 accepted")
	}
	for _, u := range []string{"http://169.254.169.254/latest/meta-data/", "http://bridge.internal/d1/query", "file:///etc/passwd", "javascript:alert(1)"} {
		if _, err := c.Meta(ctx, u); !errors.Is(err, ErrBlocked) {
			t.Errorf("%s: %v", u, err)
		}
	}
	if _, err := clientTo(t, srv, Public).Meta(ctx, "http://rebind.example.com/big"); !errors.Is(err, ErrBlocked) {
		t.Errorf("private address after DNS: %v", err)
	}
}

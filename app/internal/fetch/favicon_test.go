package fetch

import (
	"context"
	"crypto/tls"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"reflect"
	"testing"
)

var png1x1 = []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15\xc4\x89")

func TestIconLinks(t *testing.T) {
	page, _ := url.Parse("https://example.com/blog/")
	body := []byte(`<html><head>
<link rel="apple-touch-icon" href="/apple.png">
<link rel=icon type="image/svg+xml" href="/icon.svg">
<link href='icons/fav.png?v=2' rel="ICON">
<link rel="shortcut  icon" href="//cdn.example.com/f.ico">
<link rel="stylesheet" href="/x.css">
<link rel="icon" href="javascript:alert(1)">
</head><body><link rel="icon" href="/late.png"></body></html>`)
	want := []string{"https://example.com/blog/icons/fav.png?v=2", "https://cdn.example.com/f.ico", "https://example.com/apple.png"}
	if got := iconLinks(page, body); !reflect.DeepEqual(got, want) {
		t.Errorf("got %q", got)
	}
}

func TestFavicon(t *testing.T) {
	var hits []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, r.URL.Path)
		switch r.URL.Path {
		case "/":
			_, _ = w.Write([]byte(`<head><link rel="icon" href="/svg-as-png.png"><link rel="icon" href="/good.png"></head>`))
		case "/svg-as-png.png":
			_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
		case "/good.png":
			_, _ = w.Write(png1x1)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientTo(t, srv, func(netip.Addr) bool { return true })
	c.hc.Transport.(*http.Transport).TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	b, ext, err := c.Favicon(context.Background(), "example.com")
	if err != nil || ext != "png" || len(b) != len(png1x1) {
		t.Fatalf("favicon: %v %q", err, ext)
	}
	if !reflect.DeepEqual(hits, []string{"/", "/svg-as-png.png", "/good.png"}) {
		t.Errorf("hits %q", hits)
	}
	if _, _, err := clientTo(t, srv, Public).Favicon(context.Background(), "example.com"); !errors.Is(err, ErrBlocked) {
		t.Errorf("private site: %v", err)
	}
	if _, _, err := c.Favicon(context.Background(), "bridge.internal"); !errors.Is(err, ErrBlocked) {
		t.Errorf("internal host: %v", err)
	}
}

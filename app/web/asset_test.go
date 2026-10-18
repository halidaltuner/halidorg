package web

import (
	"strings"
	"testing"
)

func TestAsset(t *testing.T) {
	u := Asset("/static/css/app.css")
	if !strings.HasPrefix(u, "/static/css/app.css?v=") || len(u) != len("/static/css/app.css?v=")+10 {
		t.Errorf("got %q", u)
	}
	if Asset("/static/css/app.css") != u || Asset("/static/js/code.js") == strings.Replace(u, "css/app.css", "js/code.js", 1) {
		t.Error("hash is not per file or not stable")
	}
	if got := Asset("/static/css/missing.css"); got != "/static/css/missing.css" {
		t.Errorf("missing file: %q", got)
	}
}

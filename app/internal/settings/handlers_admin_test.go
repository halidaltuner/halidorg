package settings_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/settings"
	"halid.org/engine/internal/store/storetest"
)

func TestHomelabStatsAdmin(t *testing.T) {
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	h, err := settings.NewAdmin(s.DB, admin.NewUI(s.DB), a, jobs.New(s.DB, s.Queue))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	c, _ := a.TestSession(context.Background(), "me@halid.org", time.Hour) // no recent sign-in needed
	do := func(method, target string, form url.Values) *httptest.ResponseRecorder {
		var req *http.Request
		if form != nil {
			req = httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		} else {
			req = httptest.NewRequest(method, target, nil)
		}
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Accept-Language", "en")
		req.AddCookie(c)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if body := do("GET", "/admin/settings", nil).Body.String(); !strings.Contains(body, `name="nodes" value=""`) {
		t.Fatalf("empty form: %s", body)
	}
	rec := do("POST", "/admin/settings/homelab", url.Values{"nodes": {" 3 "}, "idle": {"38 W"}, "uptime": {"99.9%"}})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/settings?saved=homelab" {
		t.Fatalf("save %d %q", rec.Code, rec.Header().Get("Location"))
	}
	var got settings.Homelab
	if err := settings.Get(context.Background(), s.DB, settings.HomelabKey, &got); err != nil || got != (settings.Homelab{Nodes: "3", Idle: "38 W", Uptime: "99.9%"}) {
		t.Fatalf("stored %+v %v", got, err)
	}
	// The homelab pages are re-rendered in both languages.
	sent := s.Queue.(*storetest.Queue).Sent
	if len(sent) != 1 || !strings.Contains(string(mustJSON(t, sent[0])), `"paths":["/tr/homelab","/en/homelab"]`) {
		t.Fatalf("jobs %s", mustJSON(t, sent))
	}
	must := func(body string, want string) {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q", want)
		}
	}
	must(do("GET", "/admin/settings?saved=homelab", nil).Body.String(), `value="99.9%"`)
	if rec := do("POST", "/admin/settings/homelab", url.Values{"nodes": {strings.Repeat("9", 17)}}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("too long %d", rec.Code)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

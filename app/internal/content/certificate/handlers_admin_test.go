package certificate_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content/certificate"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store/storetest"
)

func TestAdmin(t *testing.T) {
	ctx := context.Background()
	s := storetest.Stores(t)
	a := auth.New(auth.Config{SessionSecret: strings.Repeat("k", 32)}, s.DB)
	h, err := certificate.NewAdmin(s.DB, admin.NewUI(s.DB), a, jobs.New(s.DB, s.Queue))
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	h.Register(mux)
	fresh, _ := a.TestSession(ctx, "me@halid.org", time.Minute)
	stale, _ := a.TestSession(ctx, "me@halid.org", time.Hour)
	do := func(c *http.Cookie, method, target string, form url.Values) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, strings.NewReader(form.Encode()))
		if form != nil {
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		}
		req.Header.Set("Sec-Fetch-Site", "same-origin")
		req.Header.Set("Accept-Language", "en")
		req.AddCookie(c)
		rec := httptest.NewRecorder()
		mux.ServeHTTP(rec, req)
		return rec
	}
	if rec := do(fresh, "POST", "/admin/certificates/issuers", url.Values{"name": {"Linux Foundation"}, "tone": {"indigo"}}); rec.Code != http.StatusSeeOther {
		t.Fatalf("issuer %d %s", rec.Code, rec.Body.String())
	}
	iss, _ := certificate.NewRepo(s.DB).Issuers(ctx)
	if len(iss) != 1 || iss[0].Slug != "linux-foundation" {
		t.Fatalf("issuers %+v", iss)
	}
	body := do(fresh, "GET", "/admin/certificates/new", nil).Body.String()
	if !strings.Contains(body, `<option value="`+iss[0].ID+`">Linux Foundation</option>`) {
		t.Error("issuer not offered")
	}
	f := url.Values{"name": {"LFCS"}, "issuer": {iss[0].ID}, "issued": {"2023-06"}, "expires": {"2023-01"}, "skills": {"Linux, ops"}, "published": {"1"}}
	rec := do(fresh, "POST", "/admin/certificates", f)
	if rec.Code != http.StatusUnprocessableEntity || !strings.Contains(rec.Body.String(), "The expiry must come after the issue month.") {
		t.Fatalf("bad expiry %d", rec.Code)
	}
	queued := len(s.Queue.(*storetest.Queue).Sent)
	f.Set("expires", "2025-06")
	rec = do(fresh, "POST", "/admin/certificates", f)
	loc := rec.Header().Get("Location")
	if rec.Code != http.StatusSeeOther || !strings.HasPrefix(loc, "/admin/certificates/") {
		t.Fatalf("create %d %s", rec.Code, rec.Body.String())
	}
	if len(s.Queue.(*storetest.Queue).Sent) == queued {
		t.Error("no page refresh queued")
	}
	id := strings.TrimSuffix(strings.TrimPrefix(loc, "/admin/certificates/"), "?saved=1")
	body = do(fresh, "GET", "/admin/certificates", nil).Body.String()
	if !strings.Contains(body, `<span class="ent-title">LFCS</span>`) || !strings.Contains(body, `2023-06 → 2025-06`) || !strings.Contains(body, `s-expired">expired`) {
		t.Errorf("list %s", body)
	}
	// The issuer is in use; deleting the certificate needs a recent sign-in.
	if rec := do(fresh, "POST", "/admin/certificates/issuers/"+iss[0].ID+"/delete", url.Values{}); rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("delete issuer in use %d", rec.Code)
	}
	if rec := do(stale, "POST", "/admin/certificates/"+id+"/delete", url.Values{}); rec.Code == http.StatusSeeOther && rec.Header().Get("Location") == "/admin/certificates?done=deleted" {
		t.Error("stale session deleted")
	}
	if rec := do(fresh, "POST", "/admin/certificates/"+id+"/delete", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Fatalf("delete %d", rec.Code)
	}
	if rec := do(fresh, "POST", "/admin/certificates/issuers/"+iss[0].ID+"/delete", url.Values{}); rec.Code != http.StatusSeeOther {
		t.Errorf("delete issuer %d", rec.Code)
	}
}

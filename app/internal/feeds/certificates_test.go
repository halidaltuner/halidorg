package feeds

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"halid.org/engine/internal/content/certificate"
	"halid.org/engine/internal/store/storetest"
)

func TestCertificateFeed(t *testing.T) {
	t.Setenv("PUBLIC_ORIGIN", "https://halid.org")
	ctx := context.Background()
	db := storetest.NewDB(t)
	r := certificate.NewRepo(db)
	iss, _ := r.SaveIssuer(ctx, certificate.Issuer{Name: "CNCF", Tone: "cyan"})
	issued, _ := certificate.ParseMonth("2026-03")
	url := "https://www.credly.com/badges/cka"
	if _, err := r.Save(ctx, certificate.Certificate{IssuerID: iss.ID, Name: "CKA", CredlyURL: &url, IssuedAt: issued, Published: true}); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(db).Register(mux)
	rec := get(t, mux, "/tr/certificates/feed.xml")
	f := rec.Body.String()
	if rec.Code != 200 || !strings.Contains(f, "<title>CKA</title>") || !strings.Contains(f, "<link>https://www.credly.com/badges/cka</link>") ||
		!strings.Contains(f, "halid.org · sertifikalar") {
		t.Fatalf("feed %d %s", rec.Code, f)
	}
	for _, p := range []string{"/en/certificates/atom.xml", "/en/certificates/feed.json"} {
		if rec := get(t, mux, p); rec.Code != 200 || !strings.Contains(rec.Body.String(), "CKA") {
			t.Errorf("%s %d", p, rec.Code)
		}
	}
	if sm := get(t, mux, "/sitemap.xml").Body.String(); !strings.Contains(sm, "<loc>https://halid.org/en/certificates</loc>") {
		t.Error("sitemap misses certificates")
	}
}

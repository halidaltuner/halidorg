package public

import (
	"context"
	"strings"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/content/certificate"
	pages "halid.org/engine/internal/content/page"
)

func month(s string) *int64 { v, _ := certificate.ParseMonth(s); return &v }

func TestCertificatesPage(t *testing.T) {
	mux, db := newSite(t)
	ctx := context.Background()
	clock := content.Clock
	t.Cleanup(func() { content.Clock = clock })
	content.Clock = func() time.Time { return time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC) }
	_, body := get(t, mux, "/en/certificates")
	must(t, body, `No certificates published yet.`)

	r := certificate.NewRepo(db)
	cncf, _ := r.SaveIssuer(ctx, certificate.Issuer{Name: "CNCF", Tone: "cyan"})
	aws, _ := r.SaveIssuer(ctx, certificate.Issuer{Name: "AWS", Tone: "amber", Sort: 1})
	url := "https://www.credly.com/badges/cka"
	for _, c := range []certificate.Certificate{
		{IssuerID: cncf.ID, Name: "CKA", Skills: []string{"kubernetes", "ops"}, CredlyURL: &url, IssuedAt: *month("2026-03"), ExpiresAt: month("2029-03"), Published: true},
		{IssuerID: aws.ID, Name: "AWS SA Associate", IssuedAt: *month("2025-11"), ExpiresAt: month("2026-12"), Published: true},
		{IssuerID: aws.ID, Name: "Cloud Practitioner", IssuedAt: *month("2022-09"), ExpiresAt: month("2025-09"), Published: true},
		{IssuerID: cncf.ID, Name: "Hidden draft", IssuedAt: *month("2024-01"), Published: false},
	} {
		if _, err := r.Save(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	code, body := get(t, mux, "/en/certificates")
	if code != 200 {
		t.Fatalf("certificates %d", code)
	}
	must(t, body,
		`<h1>Badges and certifications.</h1>`,
		`<div><dd class="tone-green">2</dd><dt class="mono">active</dt></div><div><dd class="tone-cyan">2</dd><dt class="mono">issuers</dt></div><div><dd>3</dd><dt class="mono">total</dt></div>`,
		`<a class="chip tone-cyan" href="/en/certificates?issuer=cncf" data-filter-key="cncf">CNCF</a>`,
		`<article class="ce-card tone-cyan" data-filter-value="cncf">`,
		`<span class="ce-issuer mono">CNCF</span>`, `<h2>CKA</h2>`, `<div class="ce-skills"><span class="mono">kubernetes</span><span class="mono">ops</span></div>`,
		`<span>2026-03</span><span class="ce-st st-active">active</span><a href="https://www.credly.com/badges/cka" rel="noopener">verify ↗</a>`,
		`<span class="ce-st st-renew_soon">renew soon</span>`,
		`<article class="ce-card tone-amber is-expired" data-filter-value="aws">`, `<span class="ce-st st-expired">expired</span>`,
		`href="/en/certificates/feed.xml"`, `<link rel="canonical" href="https://halid.org/en/certificates">`,
	)
	if strings.Contains(body, "Hidden draft") {
		t.Error("unpublished certificate shown")
	}
	_, body = get(t, mux, "/tr/certificates?issuer=aws")
	must(t, body, `data-filtered="aws"`, `<meta name="robots" content="noindex">`, `süresi doldu`)
	if strings.Contains(body, "<h2>CKA</h2>") {
		t.Error("filter kept another issuer")
	}

	// About's card: the active ones, newest names first.
	about, err := pages.NewRepo(db).Create(ctx, pages.Draft{Page: pages.Page{Entry: content.Entry{Lang: "en", Title: "About"}, Template: pages.About}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pages.NewRepo(db).Publish(ctx, about.ID); err != nil {
		t.Fatal(err)
	}
	_, body = get(t, mux, "/en/about")
	must(t, body, `<a class="ab-card tone-cyan ab-certs" href="/en/certificates">`, `<span class="hex tone-cyan"></span><span class="hex tone-amber"></span></span>`,
		`<span class="t">CKA and AWS SA Associate</span>`)
}

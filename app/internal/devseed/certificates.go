package devseed

import (
	"context"

	"halid.org/engine/internal/content/certificate"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Certificate samples (Phase 5.2): two issuers and three certificates, one
// active, one to renew soon, one expired. Placeholder names only. They have
// no entries: one pages.changed job re-renders the pages that show them.
func seedCertificates(ctx context.Context, db store.DB, j *jobs.Jobs) (int, error) {
	r := certificate.NewRepo(db)
	have, err := r.All(ctx)
	if err != nil {
		return 0, err
	}
	if j != nil {
		if err := jobs.EnqueuePages(ctx, j, certificate.Paths()...); err != nil {
			return 0, err
		}
	}
	if len(have) > 0 {
		return 0, nil
	}
	issuers := map[string]certificate.Issuer{}
	for _, i := range []certificate.Issuer{{Name: "Sample cloud", Slug: "sample-cloud", Tone: "cyan"}, {Name: "Sample infra", Slug: "sample-infra", Tone: "amber", Sort: 1}} {
		saved, err := r.SaveIssuer(ctx, i)
		if err != nil {
			return 0, err
		}
		issuers[i.Slug] = saved
	}
	month := func(s string) *int64 { v, _ := certificate.ParseMonth(s); return &v }
	n := 0
	for _, c := range []certificate.Certificate{
		{IssuerID: issuers["sample-cloud"].ID, Name: "Sample: cluster administrator", Skills: []string{"kubernetes"}, IssuedAt: *month("2026-03"), ExpiresAt: month("2029-03")},
		{IssuerID: issuers["sample-infra"].ID, Name: "Sample: infrastructure associate", Skills: []string{"iac"}, IssuedAt: *month("2024-12"), ExpiresAt: month("2026-12")},
		{IssuerID: issuers["sample-infra"].ID, Name: "Sample: cloud foundations", IssuedAt: *month("2022-09"), ExpiresAt: month("2025-09")},
	} {
		c.Published = true
		if _, err := r.Save(ctx, c); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

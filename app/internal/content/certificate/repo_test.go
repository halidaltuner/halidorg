package certificate

import (
	"context"
	"errors"
	"testing"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store/storetest"
)

func month(s string) *int64 { v, _ := ParseMonth(s); return &v }

func TestStatus(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)
	for exp, want := range map[string]string{"": Active, "2029-03": Active, "2027-02": Active, "2027-01": RenewSoon,
		"2026-11": RenewSoon, "2026-10": Expired, "2025-06": Expired} {
		c := Certificate{}
		if exp != "" {
			c.ExpiresAt = month(exp)
		}
		if got := c.Status(now); got != want {
			t.Errorf("%q: %s, want %s", exp, got, want)
		}
	}
}

func TestRepo(t *testing.T) {
	ctx := context.Background()
	r := NewRepo(storetest.NewDB(t))
	cncf, err := r.SaveIssuer(ctx, Issuer{Name: "CNCF", Tone: "cyan"})
	if err != nil || cncf.Slug != "cncf" {
		t.Fatalf("issuer %+v %v", cncf, err)
	}
	if _, err := r.SaveIssuer(ctx, Issuer{Name: "CNCF", Tone: "cyan"}); err == nil {
		t.Error("duplicate issuer slug")
	}
	if _, err := r.SaveIssuer(ctx, Issuer{Name: "X", Tone: "pink"}); err == nil {
		t.Error("unknown tone")
	}
	url := "https://www.credly.com/badges/x"
	c, err := r.Save(ctx, Certificate{IssuerID: cncf.ID, Name: " CKA ", Skills: []string{"Kubernetes", " ops ", "kubernetes", ""},
		CredlyURL: &url, IssuedAt: *month("2026-03"), ExpiresAt: month("2029-03"), Published: true})
	if err != nil || c.Name != "CKA" || len(c.Skills) != 2 || c.Skills[0] != "kubernetes" {
		t.Fatalf("save %+v %v", c, err)
	}
	var v content.ValidationError
	bad := c
	bad.ID, bad.ExpiresAt = "", month("2026-01")
	if _, err := r.Save(ctx, bad); !errors.As(err, &v) || v["expires_at"] == "" {
		t.Errorf("expiry before issue: %v", err)
	}
	missing := "nope"
	bad = c
	bad.ID, bad.BadgeID = "", &missing
	if _, err := r.Save(ctx, bad); !errors.As(err, &v) || v["badge"] == "" {
		t.Errorf("missing badge: %v", err)
	}
	draft, _ := r.Save(ctx, Certificate{IssuerID: cncf.ID, Name: "CKAD", IssuedAt: *month("2024-10")})
	if pub, _ := r.Published(ctx); len(pub) != 1 || pub[0].ID != c.ID {
		t.Errorf("published %+v", pub)
	}
	if all, _ := r.All(ctx); len(all) != 2 {
		t.Errorf("all %d", len(all))
	}
	if err := r.DeleteIssuer(ctx, cncf.ID); err == nil {
		t.Error("deleted an issuer in use")
	}
	for _, id := range []string{c.ID, draft.ID} {
		if err := r.Delete(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.DeleteIssuer(ctx, cncf.ID); err != nil {
		t.Errorf("delete unused issuer: %v", err)
	}
}

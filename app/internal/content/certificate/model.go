// Package certificate holds certificates and their issuers (Phase 5.2,
// CONTENT_MODEL.md §3): gallery data without entries or detail pages, one
// list for both languages. The status is derived from the expiry month.
package certificate

import (
	"time"
)

// Statuses, derived by Status.
const (
	Active    = "active"
	RenewSoon = "renew_soon"
	Expired   = "expired"
)

// Issuer is a certificate authority with its tone.
type Issuer struct {
	ID   string `json:"id"`
	Slug string `json:"slug"`
	Name string `json:"name"`
	Tone string `json:"tone"`
	Sort int    `json:"sort"`
}

// Certificate is one badge.
type Certificate struct {
	ID        string   `json:"id"`
	IssuerID  string   `json:"issuer_id"`
	Name      string   `json:"name"`
	Skills    []string `json:"-"`
	BadgeID   *string  `json:"badge_media_id"`
	CredlyURL *string  `json:"credly_url"`
	IssuedAt  int64    `json:"issued_at"`
	ExpiresAt *int64   `json:"expires_at"`
	Sort      int      `json:"sort"`
	Published bool     `json:"-"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
}

// Status is active, renew_soon (it lapses within three months) or expired
// (its expiry month has begun) at now.
func (c Certificate) Status(now time.Time) string {
	if c.ExpiresAt == nil {
		return Active
	}
	exp := time.Unix(*c.ExpiresAt, 0).UTC()
	switch {
	case !exp.After(now):
		return Expired
	case !exp.After(now.AddDate(0, 3, 0)):
		return RenewSoon
	}
	return Active
}

// ParseMonth reads "2006-01" as the first day of the month, unix UTC.
func ParseMonth(s string) (int64, bool) {
	t, err := time.Parse("2006-01", s)
	if err != nil {
		return 0, false
	}
	return t.Unix(), true
}

// Month formats unix seconds as "2006-01"; "" for nil.
func Month(sec *int64) string {
	if sec == nil {
		return ""
	}
	return time.Unix(*sec, 0).UTC().Format("2006-01")
}

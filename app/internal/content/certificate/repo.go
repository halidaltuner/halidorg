package certificate

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store"
)

// Repo reads and writes certificates and issuers.
type Repo struct{ db store.DB }

// NewRepo returns a repository over db.
func NewRepo(db store.DB) *Repo { return &Repo{db: db} }

// --- Issuers ----------------------------------------------------------------

// Issuers lists every issuer in order.
func (r *Repo) Issuers(ctx context.Context) ([]Issuer, error) {
	res, err := r.db.Query(ctx, `SELECT id, slug, name, tone, sort FROM cert_issuers ORDER BY sort, name`)
	if err != nil {
		return nil, err
	}
	return store.Scan[Issuer](res)
}

// ValidateIssuer checks an issuer's fields.
func ValidateIssuer(i Issuer) error {
	v := content.ValidationError{}
	if i.Name == "" || utf8.RuneCountInString(i.Name) > 60 {
		v["issuer_name"] = "1 to 60 characters"
	}
	if !content.ValidSlug(i.Slug) || len(i.Slug) > 40 {
		v["issuer_slug"] = "lowercase letters, digits and hyphens, at most 40"
	}
	if !slices.Contains(content.Tones, i.Tone) {
		v["issuer_tone"] = "unknown tone"
	}
	if len(v) > 0 {
		return v
	}
	return nil
}

// SaveIssuer inserts (no id) or updates an issuer.
func (r *Repo) SaveIssuer(ctx context.Context, i Issuer) (Issuer, error) {
	i.Name, i.Slug = strings.TrimSpace(i.Name), strings.TrimSpace(i.Slug)
	if i.Slug == "" {
		i.Slug = content.Slugify(i.Name)
	}
	if err := ValidateIssuer(i); err != nil {
		return i, err
	}
	if i.ID == "" {
		i.ID = content.NewID()
	}
	_, err := r.db.Query(ctx, `INSERT INTO cert_issuers (id, slug, name, tone, sort) VALUES (?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET slug = excluded.slug, name = excluded.name, tone = excluded.tone, sort = excluded.sort`,
		i.ID, i.Slug, i.Name, i.Tone, i.Sort)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "unique") {
		return i, content.ValidationError{"issuer_slug": "already used"}
	}
	return i, err
}

// ErrIssuerInUse is returned when deleting an issuer that has certificates.
var ErrIssuerInUse = content.ValidationError{"issuer_in_use": "the issuer still has certificates"}

// DeleteIssuer removes an issuer without certificates.
func (r *Repo) DeleteIssuer(ctx context.Context, id string) error {
	res, err := r.db.Query(ctx, `SELECT COUNT(*) AS n FROM certificates WHERE issuer_id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := store.Scan[struct {
		N int `json:"n"`
	}](res)
	if err != nil {
		return err
	}
	if len(rows) == 1 && rows[0].N > 0 {
		return ErrIssuerInUse
	}
	_, err = r.db.Query(ctx, `DELETE FROM cert_issuers WHERE id = ?`, id)
	return err
}

// --- Certificates -----------------------------------------------------------

type row struct {
	Certificate
	SkillsJSON string `json:"skills_json"`
	Pub        int    `json:"published"`
}

const columns = `id, issuer_id, name, skills_json, badge_media_id, credly_url, issued_at, expires_at, sort, published, created_at, updated_at`

func (x row) cert() Certificate {
	c := x.Certificate
	_ = json.Unmarshal([]byte(x.SkillsJSON), &c.Skills)
	c.Published = x.Pub == 1
	return c
}

func (r *Repo) list(ctx context.Context, where string, args ...any) ([]Certificate, error) {
	res, err := r.db.Query(ctx, `SELECT `+columns+` FROM certificates WHERE `+where+
		` ORDER BY sort, issued_at DESC, id DESC LIMIT 500`, args...)
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[row](res)
	if err != nil {
		return nil, err
	}
	out := make([]Certificate, len(rows))
	for i, x := range rows {
		out[i] = x.cert()
	}
	return out, nil
}

// All lists every certificate (admin).
func (r *Repo) All(ctx context.Context) ([]Certificate, error) { return r.list(ctx, "1 = 1") }

// Published lists the published certificates (public pages, feeds).
func (r *Repo) Published(ctx context.Context) ([]Certificate, error) {
	return r.list(ctx, "published = 1")
}

// Get loads one certificate.
func (r *Repo) Get(ctx context.Context, id string) (Certificate, error) {
	cs, err := r.list(ctx, "id = ?", id)
	if err != nil {
		return Certificate{}, err
	}
	if len(cs) == 0 {
		return Certificate{}, content.NotFound("certificate", id)
	}
	return cs[0], nil
}

// Validate checks a certificate's fields (the issuer and badge are checked
// against their tables in Save).
func Validate(c Certificate) error {
	v := content.ValidationError{}
	if c.Name == "" || utf8.RuneCountInString(c.Name) > 160 {
		v["name"] = "1 to 160 characters"
	}
	if c.IssuerID == "" {
		v["issuer"] = "pick an issuer"
	}
	if c.IssuedAt <= 0 {
		v["issued_at"] = "a month, e.g. 2026-03"
	}
	if c.ExpiresAt != nil && *c.ExpiresAt <= c.IssuedAt {
		v["expires_at"] = "after the issue month"
	}
	if c.CredlyURL != nil && !content.HTTPSURL(*c.CredlyURL) {
		v["credly_url"] = "an https URL"
	}
	if len(c.Skills) > 8 {
		v["skills"] = "at most 8"
	}
	for _, s := range c.Skills {
		if utf8.RuneCountInString(s) > 30 {
			v["skills"] = "each at most 30 characters"
		}
	}
	if len(v) > 0 {
		return v
	}
	return nil
}

// Save inserts (no id) or updates a certificate after checking the issuer
// and that the badge is an image with both alt texts.
func (r *Repo) Save(ctx context.Context, c Certificate) (Certificate, error) {
	c.Name = strings.TrimSpace(c.Name)
	skills := c.Skills[:0:0]
	for _, s := range c.Skills {
		if s = strings.ToLower(strings.TrimSpace(s)); s != "" && !slices.Contains(skills, s) {
			skills = append(skills, s)
		}
	}
	c.Skills = skills
	if err := Validate(c); err != nil {
		return c, err
	}
	res, err := r.db.Query(ctx, `SELECT (SELECT COUNT(*) FROM cert_issuers WHERE id = ?) AS issuer,
  (SELECT COUNT(*) FROM media WHERE id = ? AND mime LIKE 'image/%' AND trim(alt_tr) <> '' AND trim(alt_en) <> '') AS badge`,
		c.IssuerID, c.BadgeID)
	if err != nil {
		return c, err
	}
	ok, err := store.Scan[struct {
		Issuer int `json:"issuer"`
		Badge  int `json:"badge"`
	}](res)
	if err != nil {
		return c, err
	}
	if len(ok) == 1 && ok[0].Issuer == 0 {
		return c, content.ValidationError{"issuer": "pick an issuer"}
	}
	if c.BadgeID != nil && (len(ok) != 1 || ok[0].Badge == 0) {
		return c, content.ValidationError{"badge": "an image with both alt texts"}
	}
	raw, err := json.Marshal(c.Skills)
	if err != nil {
		return c, err
	}
	now := content.Clock().Unix()
	if c.ID == "" {
		c.ID, c.CreatedAt = content.NewID(), now
	}
	c.UpdatedAt = now
	pub := 0
	if c.Published {
		pub = 1
	}
	_, err = r.db.Query(ctx, `INSERT INTO certificates (`+columns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT (id) DO UPDATE SET issuer_id = excluded.issuer_id, name = excluded.name, skills_json = excluded.skills_json,
  badge_media_id = excluded.badge_media_id, credly_url = excluded.credly_url, issued_at = excluded.issued_at,
  expires_at = excluded.expires_at, sort = excluded.sort, published = excluded.published, updated_at = excluded.updated_at`,
		c.ID, c.IssuerID, c.Name, string(raw), c.BadgeID, c.CredlyURL, c.IssuedAt, c.ExpiresAt, c.Sort, pub, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return c, fmt.Errorf("certificate: save: %w", err)
	}
	return c, nil
}

// Delete removes a certificate.
func (r *Repo) Delete(ctx context.Context, id string) error {
	res, err := r.db.Query(ctx, `DELETE FROM certificates WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if res.Changes == 0 {
		return content.NotFound("certificate", id)
	}
	return nil
}

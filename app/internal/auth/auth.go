// Package auth is admin sign-in (ADR-005): Google OIDC with PKCE, restricted
// to @halid.org, and opaque sessions stored in admin_sessions.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"halid.org/engine/internal/store"
)

const (
	// Domain is the only Google Workspace domain allowed into admin.
	Domain = "halid.org"

	googleIssuer = "https://accounts.google.com"
	googleJWKS   = "https://www.googleapis.com/oauth2/v3/certs"

	SessionTTL   = 12 * time.Hour
	ReauthMaxAge = 10 * time.Minute
)

// Google endpoints are fixed rather than discovered, so a cold container does
// not need a network round trip before it can render the login page.
var googleEndpoint = oauth2.Endpoint{
	AuthURL:   "https://accounts.google.com/o/oauth2/v2/auth",
	TokenURL:  "https://oauth2.googleapis.com/token",
	AuthStyle: oauth2.AuthStyleInParams,
}

// Config comes from the container environment (worker/src/index.ts).
type Config struct {
	ClientID      string
	ClientSecret  string
	SessionSecret string
	PublicOrigin  string // scheme://host the browser sees; builds the redirect URI
}

// ConfigFromEnv reads Config from the process environment.
func ConfigFromEnv() Config {
	return Config{
		ClientID:      os.Getenv("GOOGLE_CLIENT_ID"),
		ClientSecret:  os.Getenv("GOOGLE_CLIENT_SECRET"),
		SessionSecret: os.Getenv("SESSION_SECRET"),
		PublicOrigin:  strings.TrimRight(os.Getenv("PUBLIC_ORIGIN"), "/"),
	}
}

// Auth holds the OIDC client and session store.
type Auth struct {
	cfg      Config
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	db       store.DB
	now      func() time.Time

	sessionKey, flowKey, flashKey []byte
}

// ErrNotConfigured means a secret is missing; sign-in fails closed.
var ErrNotConfigured = errors.New("auth: GOOGLE_CLIENT_ID, GOOGLE_CLIENT_SECRET, SESSION_SECRET and PUBLIC_ORIGIN are required")

// New builds Auth. With an incomplete Config it still returns an Auth whose
// sign-in fails with ErrNotConfigured, so public pages keep serving.
func New(cfg Config, db store.DB) *Auth {
	return newAuth(cfg, db, oidc.NewRemoteKeySet(context.Background(), googleJWKS), time.Now)
}

func newAuth(cfg Config, db store.DB, keys oidc.KeySet, now func() time.Time) *Auth {
	a := &Auth{
		cfg: cfg,
		oauth: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			Endpoint:     googleEndpoint,
			RedirectURL:  cfg.PublicOrigin + "/admin/auth/callback",
			Scopes:       []string{oidc.ScopeOpenID, "email"},
		},
		verifier: oidc.NewVerifier(googleIssuer, keys, &oidc.Config{
			ClientID:             cfg.ClientID,
			SupportedSigningAlgs: []string{oidc.RS256},
			Now:                  now,
		}),
		db:  db,
		now: now,
	}
	// One secret, separate keys per use, so a value signed for one purpose
	// never verifies as another.
	a.sessionKey = derive(cfg.SessionSecret, "admin_sessions.id")
	a.flowKey = derive(cfg.SessionSecret, "oidc-flow")
	a.flashKey = derive(cfg.SessionSecret, "login-flash")
	return a
}

func (a *Auth) configured() bool {
	c := a.cfg
	return c.ClientID != "" && c.ClientSecret != "" && len(c.SessionSecret) >= 32 && c.PublicOrigin != ""
}

func derive(secret, purpose string) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write([]byte(purpose))
	return m.Sum(nil)
}

// Identity is a verified admin.
type Identity struct {
	Email    string
	AuthTime time.Time // zero unless Google returned auth_time
}

// DomainError is a valid Google account outside @halid.org. The login page
// shows its Email in the rose-rule banner.
type DomainError struct{ Email string }

func (e *DomainError) Error() string {
	return fmt.Sprintf("auth: %q is not an @%s account", e.Email, Domain)
}

var (
	ErrUnverifiedEmail = errors.New("auth: email_verified is not true")
	ErrNonce           = errors.New("auth: nonce mismatch")
	ErrStaleAuth       = errors.New("auth: auth_time missing or older than the re-auth window")
)

// googleClaims are the claims checked beyond what go-oidc verifies.
type googleClaims struct {
	Email         string   `json:"email"`
	EmailVerified flexBool `json:"email_verified"`
	HD            string   `json:"hd"`
	AuthTime      int64    `json:"auth_time"`
}

// flexBool accepts true and "true": older Google tokens sent the string.
type flexBool bool

func (b *flexBool) UnmarshalJSON(d []byte) error {
	var v any
	if err := json.Unmarshal(d, &v); err != nil {
		return err
	}
	switch t := v.(type) {
	case bool:
		*b = flexBool(t)
	case string:
		*b = flexBool(t == "true")
	}
	return nil
}

// verifyIDToken checks signature, iss, aud and exp (go-oidc), then nonce and
// the domain claims. hd is never trusted alone: the email domain must match too.
func (a *Auth) verifyIDToken(ctx context.Context, raw, nonce string) (Identity, error) {
	tok, err := a.verifier.Verify(ctx, raw)
	if err != nil {
		return Identity{}, err
	}
	if !hmac.Equal([]byte(tok.Nonce), []byte(nonce)) || nonce == "" {
		return Identity{}, ErrNonce
	}
	var c googleClaims
	if err := tok.Claims(&c); err != nil {
		return Identity{}, err
	}
	if err := checkClaims(c); err != nil {
		return Identity{}, err
	}
	id := Identity{Email: strings.ToLower(c.Email)}
	if c.AuthTime > 0 {
		id.AuthTime = time.Unix(c.AuthTime, 0)
	}
	return id, nil
}

func checkClaims(c googleClaims) error {
	email := strings.ToLower(strings.TrimSpace(c.Email))
	local, domain, ok := strings.Cut(email, "@")
	if c.HD != Domain || !ok || local == "" || domain != Domain {
		return &DomainError{Email: c.Email}
	}
	if !c.EmailVerified {
		return ErrUnverifiedEmail
	}
	return nil
}

// checkFresh is the re-auth rule: Google must report an auth_time inside the
// window. Missing auth_time fails closed (see docs/environments.md).
func (a *Auth) checkFresh(id Identity) error {
	if id.AuthTime.IsZero() || a.now().Sub(id.AuthTime) > ReauthMaxAge {
		return ErrStaleAuth
	}
	return nil
}

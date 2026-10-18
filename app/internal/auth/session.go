package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"time"

	"halid.org/engine/internal/store"
)

// SessionCookie is scoped to /admin, which the edge Worker never caches.
const SessionCookie = "admin_session"

// Session is one admin_sessions row.
type Session struct {
	ID           string `json:"id"`
	Email        string `json:"email"`
	CreatedAt    int64  `json:"created_at"`
	ExpiresAt    int64  `json:"expires_at"`
	LastReauthAt int64  `json:"last_reauth_at"`
}

// sessionID maps a cookie token to its row id. Only the hash is stored.
func (a *Auth) sessionID(token string) string {
	m := hmac.New(sha256.New, a.sessionKey)
	m.Write([]byte(token))
	return hex.EncodeToString(m.Sum(nil))
}

func randomToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b) // crypto/rand.Read never returns an error
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *Auth) createSession(ctx context.Context, email, ip, ua string) (string, error) {
	token := randomToken()
	now := a.now().Unix()
	_, err := a.db.Query(ctx,
		`INSERT INTO admin_sessions (id, email, created_at, expires_at, last_reauth_at, ip, ua) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		a.sessionID(token), email, now, now+int64(SessionTTL/time.Second), now, ip, ua)
	return token, err
}

// lookup returns the live session for token, or nil.
func (a *Auth) lookup(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, nil
	}
	res, err := a.db.Query(ctx,
		`SELECT id, email, created_at, expires_at, last_reauth_at FROM admin_sessions WHERE id = ? AND expires_at > ?`,
		a.sessionID(token), a.now().Unix())
	if err != nil {
		return nil, err
	}
	rows, err := store.Scan[Session](res)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}

func (a *Auth) markReauth(ctx context.Context, id string) error {
	_, err := a.db.Query(ctx, `UPDATE admin_sessions SET last_reauth_at = ? WHERE id = ?`, a.now().Unix(), id)
	return err
}

func (a *Auth) deleteSession(ctx context.Context, id string) error {
	_, err := a.db.Query(ctx, `DELETE FROM admin_sessions WHERE id = ?`, id)
	return err
}

func setSessionCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/admin",
		MaxAge:   maxAge,
		Secure:   true,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

// TestSession creates a session for email whose last sign-in or re-auth was
// reauthAgo ago and returns its cookie. It exists for handler tests in other
// packages (admin, post); the sign-in flow never calls it, and it refuses to
// run when APP_ENV is prod.
func (a *Auth) TestSession(ctx context.Context, email string, reauthAgo time.Duration) (*http.Cookie, error) {
	if os.Getenv("APP_ENV") == "prod" {
		return nil, errors.New("auth: TestSession is not available in prod")
	}
	token, err := a.createSession(ctx, email, "", "")
	if err != nil {
		return nil, err
	}
	at := a.now().Add(-reauthAgo).Unix()
	if _, err := a.db.Query(ctx, `UPDATE admin_sessions SET last_reauth_at = ? WHERE id = ?`, at, a.sessionID(token)); err != nil {
		return nil, err
	}
	return &http.Cookie{Name: SessionCookie, Value: token}, nil
}

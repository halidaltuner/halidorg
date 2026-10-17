package auth

import (
	"context"
	"crypto/hmac"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	flowCookie  = "admin_oidc"
	flowPath    = "/admin/auth"
	flowTTL     = 10 * time.Minute
	flashCookie = "admin_flash"
	flashPath   = "/admin/login"
	flashTTL    = time.Minute

	LoginPath = "/admin/login"
	HomePath  = "/admin"
)

// flow is what the callback needs to finish what Start began.
type flow struct {
	State     string `json:"s"`
	Nonce     string `json:"n"`
	Verifier  string `json:"v"`
	Next      string `json:"next"`
	SessionID string `json:"sid,omitempty"` // set for re-auth: the session to refresh
}

// Flash is the one-shot message the login page shows after a failed callback.
type Flash struct {
	Kind  string `json:"k"` // "domain" or "failed"
	Email string `json:"e,omitempty"`
}

// Start redirects to Google. A non-nil sess makes it a re-auth for that session.
func (a *Auth) Start(w http.ResponseWriter, r *http.Request, next string, sess *Session) error {
	if !a.configured() {
		return ErrNotConfigured
	}
	f := flow{State: randomToken(), Nonce: randomToken(), Verifier: oauth2.GenerateVerifier(), Next: SafeNext(next)}
	opts := []oauth2.AuthCodeOption{
		oauth2.S256ChallengeOption(f.Verifier),
		oidc.Nonce(f.Nonce),
		oauth2.SetAuthURLParam("hd", Domain), // a hint for the account chooser only
		oauth2.SetAuthURLParam("prompt", "select_account"),
	}
	if sess != nil {
		f.SessionID = sess.ID
		// Google has no prompt=login. auth_time is returned only when asked for
		// here and enabled in the OAuth client's settings; checkFresh needs it.
		opts = append(opts,
			oauth2.SetAuthURLParam("login_hint", sess.Email),
			oauth2.SetAuthURLParam("max_age", "0"),
			oauth2.SetAuthURLParam("claims", `{"id_token":{"auth_time":{"essential":true}}}`))
	}
	if err := setSigned(w, a.flowKey, flowCookie, flowPath, flowTTL, a.now(), f); err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, a.oauth.AuthCodeURL(f.State, opts...), http.StatusSeeOther)
	return nil
}

// Callback finishes the flow on /admin/auth/callback. Every failure lands on
// the login page with a flash; only a domain mismatch names the account.
func (a *Auth) Callback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	next, err := a.finish(w, r)
	if err == nil {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	fl := Flash{Kind: "failed"}
	if de, ok := errors.AsType[*DomainError](err); ok {
		fl = Flash{Kind: "domain", Email: de.Email}
	}
	slog.Warn("admin sign-in rejected", "err", err)
	_ = setSigned(w, a.flashKey, flashCookie, flashPath, flashTTL, a.now(), fl)
	http.Redirect(w, r, LoginPath, http.StatusSeeOther)
}

func (a *Auth) finish(w http.ResponseWriter, r *http.Request) (string, error) {
	if !a.configured() {
		return "", ErrNotConfigured
	}
	var f flow
	if err := readSigned(w, r, a.flowKey, flowCookie, flowPath, a.now(), &f); err != nil {
		return "", err
	}
	q := r.URL.Query()
	if e := q.Get("error"); e != "" {
		return "", errors.New("auth: google returned " + e)
	}
	if !hmac.Equal([]byte(q.Get("state")), []byte(f.State)) {
		return "", errors.New("auth: state mismatch")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tok, err := a.oauth.Exchange(ctx, q.Get("code"), oauth2.VerifierOption(f.Verifier))
	if err != nil {
		return "", err
	}
	raw, _ := tok.Extra("id_token").(string)
	if raw == "" {
		return "", errors.New("auth: token response has no id_token")
	}
	id, err := a.verifyIDToken(ctx, raw, f.Nonce)
	if err != nil {
		return "", err
	}

	if f.SessionID != "" {
		return f.Next, a.finishReauth(ctx, r, f, id)
	}
	token, err := a.createSession(ctx, id.Email, r.Header.Get("CF-Connecting-IP"), r.UserAgent())
	if err != nil {
		return "", err
	}
	setSessionCookie(w, token, int(SessionTTL/time.Second))
	return f.Next, nil
}

// finishReauth refreshes the session that started the flow, and only for the
// same person: re-auth as another @halid.org account must not extend it.
func (a *Auth) finishReauth(ctx context.Context, r *http.Request, f flow, id Identity) error {
	if err := a.checkFresh(id); err != nil {
		return err
	}
	sess, err := a.lookup(ctx, cookieValue(r, SessionCookie))
	if err != nil {
		return err
	}
	if sess == nil || sess.ID != f.SessionID || sess.Email != id.Email {
		return errors.New("auth: re-auth does not match the current session")
	}
	return a.markReauth(ctx, sess.ID)
}

// Logout deletes the session row and the cookie.
func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if s := SessionFrom(r.Context()); s != nil {
		if err := a.deleteSession(r.Context(), s.ID); err != nil {
			slog.Error("delete session", "err", err)
		}
	}
	setSessionCookie(w, "", -1)
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, LoginPath, http.StatusSeeOther)
}

// TakeFlash returns and clears the login flash, if any.
func (a *Auth) TakeFlash(w http.ResponseWriter, r *http.Request) *Flash {
	if _, err := r.Cookie(flashCookie); err != nil {
		return nil
	}
	var f Flash
	if readSigned(w, r, a.flashKey, flashCookie, flashPath, a.now(), &f) != nil {
		return nil
	}
	return &f
}

// SafeNext keeps post-login redirects inside /admin, so the flow cannot be
// used as an open redirect.
func SafeNext(next string) string {
	u, err := url.Parse(next)
	if err != nil || u.Scheme != "" || u.Host != "" || strings.Contains(next, "\\") || strings.HasPrefix(next, "//") {
		return HomePath
	}
	// Clean first: the browser resolves dot segments, so /admin/../en/ leaves admin.
	if p := path.Clean("/" + u.Path); p != u.Path || (p != HomePath && !strings.HasPrefix(p, HomePath+"/")) {
		return HomePath
	}
	return u.RequestURI()
}

func cookieValue(r *http.Request, name string) string {
	c, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return c.Value
}

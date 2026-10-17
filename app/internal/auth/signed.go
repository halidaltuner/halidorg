package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"
)

var errBadCookie = errors.New("auth: missing, tampered or expired cookie")

// setSigned stores v as base64url(json).base64url(hmac) with its own expiry,
// so the server does not rely on the browser honouring Max-Age.
func setSigned(w http.ResponseWriter, key []byte, name, path string, ttl time.Duration, now time.Time, v any) error {
	body, err := json.Marshal(struct {
		Exp int64 `json:"exp"`
		V   any   `json:"v"`
	}{now.Add(ttl).Unix(), v})
	if err != nil {
		return err
	}
	payload := base64.RawURLEncoding.EncodeToString(body)
	http.SetCookie(w, &http.Cookie{
		Name: name, Value: payload + "." + mac(key, payload), Path: path,
		MaxAge: int(ttl / time.Second), Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode,
	})
	return nil
}

// readSigned verifies and decodes a cookie written by setSigned, then clears it:
// every signed cookie here is single use.
func readSigned(w http.ResponseWriter, r *http.Request, key []byte, name, path string, now time.Time, v any) error {
	c, err := r.Cookie(name)
	if err != nil {
		return errBadCookie
	}
	http.SetCookie(w, &http.Cookie{Name: name, Path: path, MaxAge: -1, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	payload, sig, ok := strings.Cut(c.Value, ".")
	if !ok || !hmac.Equal([]byte(sig), []byte(mac(key, payload))) {
		return errBadCookie
	}
	body, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return errBadCookie
	}
	var env struct {
		Exp int64           `json:"exp"`
		V   json.RawMessage `json:"v"`
	}
	if json.Unmarshal(body, &env) != nil || now.Unix() > env.Exp {
		return errBadCookie
	}
	return json.Unmarshal(env.V, v)
}

func mac(key []byte, s string) string {
	m := hmac.New(sha256.New, key)
	m.Write([]byte(s))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

package auth

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
)

type ctxKey struct{}

// SessionFrom returns the session RequireSession put on the context.
func SessionFrom(ctx context.Context) *Session {
	s, _ := ctx.Value(ctxKey{}).(*Session)
	return s
}

// RequireSession lets through requests with a live session; others go to login.
func (a *Auth) RequireSession(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		sess, err := a.lookup(r.Context(), cookieValue(r, SessionCookie))
		if err != nil {
			slog.Error("session lookup", "err", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		if sess == nil {
			redirect(w, r, LoginPath)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, sess)))
	})
}

// RequireRecentAuth guards publish and delete: the session must have signed
// in or re-authenticated within ReauthMaxAge. Wrap it inside RequireSession.
func (a *Auth) RequireRecentAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sess := SessionFrom(r.Context())
		if sess == nil {
			redirect(w, r, LoginPath)
			return
		}
		if !a.Recent(r) {
			redirect(w, r, ReauthPath(returnTo(r)))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Recent reports whether the session on r signed in or re-authenticated
// within ReauthMaxAge. Handlers that must save before asking for re-auth
// (publish keeps the editor's unsaved text) check it themselves.
func (a *Auth) Recent(r *http.Request) bool {
	sess := SessionFrom(r.Context())
	return sess != nil && a.now().Unix()-sess.LastReauthAt <= int64(ReauthMaxAge.Seconds())
}

// ReauthPath is the re-auth start that returns to next afterwards.
func ReauthPath(next string) string {
	return "/admin/reauth?next=" + url.QueryEscape(SafeNext(next))
}

// Redirect sends a browser to "to", with HX-Redirect for htmx requests.
func Redirect(w http.ResponseWriter, r *http.Request, to string) { redirect(w, r, to) }

// returnTo is where to land after re-auth. A POST cannot be replayed, so it
// returns to the page the form was on.
func returnTo(r *http.Request) string {
	if r.Method == http.MethodGet {
		return SafeNext(r.URL.RequestURI())
	}
	if ref, err := url.Parse(r.Referer()); err == nil {
		return SafeNext(ref.RequestURI())
	}
	return HomePath
}

// redirect uses HX-Redirect for htmx requests, which would otherwise swap the
// login page into a fragment.
func redirect(w http.ResponseWriter, r *http.Request, to string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", to)
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"

	"halid.org/engine/internal/store"
)

const clientID = "test-client.apps.googleusercontent.com"

var (
	googleKey = mustKey()
	otherKey  = mustKey()
	t0        = time.Unix(1_800_000_000, 0)
)

func mustKey() *rsa.PrivateKey {
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}
	return k
}

// claims returns a valid Google ID token payload for me@halid.org.
func claims(nonce string) map[string]any {
	return map[string]any{
		"iss":            "https://accounts.google.com",
		"aud":            clientID,
		"sub":            "1234567890",
		"iat":            t0.Unix(),
		"exp":            t0.Add(time.Hour).Unix(),
		"nonce":          nonce,
		"email":          "me@halid.org",
		"email_verified": true,
		"hd":             "halid.org",
	}
}

func sign(t *testing.T, key *rsa.PrivateKey, c map[string]any) string {
	t.Helper()
	s, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := jwt.Signed(s).Claims(c).Serialize()
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// fakeDB answers the four admin_sessions statements session.go sends.
type fakeDB struct {
	mu   sync.Mutex
	rows map[string]Session
}

func (d *fakeDB) Query(_ context.Context, sql string, args ...any) (store.Result, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch {
	case strings.HasPrefix(sql, "INSERT INTO admin_sessions"):
		d.rows[args[0].(string)] = Session{ID: args[0].(string), Email: args[1].(string), CreatedAt: args[2].(int64), ExpiresAt: args[3].(int64), LastReauthAt: args[4].(int64)}
	case strings.HasPrefix(sql, "SELECT"):
		s, ok := d.rows[args[0].(string)]
		if ok && s.ExpiresAt > args[1].(int64) {
			b, _ := json.Marshal(s)
			return store.Result{Rows: []json.RawMessage{b}}, nil
		}
	case strings.HasPrefix(sql, "UPDATE admin_sessions SET last_reauth_at"):
		s := d.rows[args[1].(string)]
		s.LastReauthAt = args[0].(int64)
		d.rows[s.ID] = s
	case strings.HasPrefix(sql, "DELETE"):
		delete(d.rows, args[0].(string))
	default:
		return store.Result{}, errors.New("unexpected sql: " + sql)
	}
	return store.Result{}, nil
}

func (d *fakeDB) Batch(context.Context, ...store.Stmt) ([]store.Result, error) {
	return nil, errors.New("unused")
}

type harness struct {
	a     *Auth
	db    *fakeDB
	now   time.Time
	token func(code string) map[string]any // id token claims for a code; nil uses claims()
	key   *rsa.PrivateKey
	nonce string // from the last Start
	chal  string
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{db: &fakeDB{rows: map[string]Session{}}, now: t0, key: googleKey}
	cfg := Config{ClientID: clientID, ClientSecret: "s", SessionSecret: strings.Repeat("k", 32), PublicOrigin: "https://staging.halid.org"}
	keys := &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&googleKey.PublicKey}}
	h.a = newAuth(cfg, h.db, keys, func() time.Time { return h.now })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		if base64.RawURLEncoding.EncodeToString(sum[:]) != h.chal {
			http.Error(w, `{"error":"invalid_grant"}`, http.StatusBadRequest)
			return
		}
		c := claims(h.nonce)
		if h.token != nil {
			c = h.token(r.Form.Get("code"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "expires_in": 3600, "id_token": sign(t, h.key, c)})
	}))
	t.Cleanup(srv.Close)
	h.a.oauth.Endpoint.TokenURL = srv.URL
	return h
}

// start runs Start and returns the Google URL and the flow cookie.
func (h *harness) start(t *testing.T, next string, sess *Session) (*url.URL, []*http.Cookie) {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := h.a.Start(rec, httptest.NewRequest("POST", "/admin/login", nil), next, sess); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(rec.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	h.nonce, h.chal = u.Query().Get("nonce"), u.Query().Get("code_challenge")
	return u, rec.Result().Cookies()
}

func (h *harness) callback(t *testing.T, query string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/admin/auth/callback?"+query, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.a.Callback(rec, req)
	return rec
}

func cookie(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// TestClaimChecks covers every check verifyIDToken makes.
func TestClaimChecks(t *testing.T) {
	h := newHarness(t)
	const nonce = "n-1"
	with := func(edit func(map[string]any)) map[string]any {
		c := claims(nonce)
		edit(c)
		return c
	}
	cases := []struct {
		name   string
		key    *rsa.PrivateKey
		claims map[string]any
		nonce  string
		domain string // expected DomainError email; "" when another error or none
		ok     bool
	}{
		{name: "valid halid account", claims: claims(nonce), ok: true},
		{name: "upper-case email is normalised", claims: with(func(c map[string]any) { c["email"] = "Me@HALID.ORG" }), ok: true},
		{name: "issuer without scheme", claims: with(func(c map[string]any) { c["iss"] = "accounts.google.com" }), ok: true},
		{name: "email_verified as string", claims: with(func(c map[string]any) { c["email_verified"] = "true" }), ok: true},
		{name: "gmail account, no hd", claims: with(func(c map[string]any) { c["email"] = "x@gmail.com"; delete(c, "hd") }), domain: "x@gmail.com"},
		{name: "halid email but hd missing", claims: with(func(c map[string]any) { delete(c, "hd") }), domain: "me@halid.org"},
		{name: "hd halid.org but gmail email", claims: with(func(c map[string]any) { c["email"] = "x@gmail.com" }), domain: "x@gmail.com"},
		{name: "other workspace", claims: with(func(c map[string]any) { c["hd"] = "evil.org"; c["email"] = "me@evil.org" }), domain: "me@evil.org"},
		{name: "hd lookalike", claims: with(func(c map[string]any) { c["hd"] = "halid.org.evil.com" }), domain: "me@halid.org"},
		{name: "email domain suffix trick", claims: with(func(c map[string]any) { c["email"] = "me@halid.org.evil.com" }), domain: "me@halid.org.evil.com"},
		{name: "email subdomain", claims: with(func(c map[string]any) { c["email"] = "me@x.halid.org" }), domain: "me@x.halid.org"},
		{name: "empty local part", claims: with(func(c map[string]any) { c["email"] = "@halid.org" }), domain: "@halid.org"},
		{name: "email_verified false", claims: with(func(c map[string]any) { c["email_verified"] = false })},
		{name: "email_verified missing", claims: with(func(c map[string]any) { delete(c, "email_verified") })},
		{name: "wrong audience", claims: with(func(c map[string]any) { c["aud"] = "someone-else" })},
		{name: "wrong issuer", claims: with(func(c map[string]any) { c["iss"] = "https://evil.example" })},
		{name: "expired", claims: with(func(c map[string]any) { c["exp"] = t0.Add(-time.Minute).Unix() })},
		{name: "signed by another key", key: otherKey, claims: claims(nonce)},
		{name: "nonce mismatch", claims: claims(nonce), nonce: "n-2"},
		{name: "empty nonce", claims: claims(""), nonce: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			key := tc.key
			if key == nil {
				key = googleKey
			}
			want := nonce
			if tc.name == "nonce mismatch" || tc.name == "empty nonce" {
				want = tc.nonce
			}
			id, err := h.a.verifyIDToken(context.Background(), sign(t, key, tc.claims), want)
			if tc.ok {
				if err != nil || id.Email != "me@halid.org" {
					t.Fatalf("want ok, got %+v %v", id, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want rejection, got %+v", id)
			}
			de, isDomain := errors.AsType[*DomainError](err)
			if tc.domain != "" && (!isDomain || de.Email != tc.domain) {
				t.Fatalf("want DomainError(%s), got %v", tc.domain, err)
			}
			if tc.domain == "" && isDomain {
				t.Fatalf("want a non-domain error, got %v", err)
			}
		})
	}
}

func TestStartBuildsPKCERequest(t *testing.T) {
	h := newHarness(t)
	u, cookies := h.start(t, "/admin/posts", nil)
	q := u.Query()
	for k, want := range map[string]string{"hd": "halid.org", "code_challenge_method": "S256", "client_id": clientID, "prompt": "select_account", "redirect_uri": "https://staging.halid.org/admin/auth/callback"} {
		if q.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, q.Get(k), want)
		}
	}
	if q.Get("code_challenge") == "" || q.Get("nonce") == "" || q.Get("state") == "" || q.Get("claims") != "" {
		t.Errorf("query %v", q)
	}
	if len(cookies) != 1 || cookies[0].Path != "/admin/auth" || !cookies[0].Secure || !cookies[0].HttpOnly {
		t.Errorf("flow cookie %+v", cookies)
	}

	ru, _ := h.start(t, "/admin", &Session{ID: "sid", Email: "me@halid.org"})
	if ru.Query().Get("max_age") != "0" || !strings.Contains(ru.Query().Get("claims"), "auth_time") || ru.Query().Get("login_hint") != "me@halid.org" {
		t.Errorf("re-auth query %v", ru.Query())
	}
}

func TestStartFailsClosedWithoutSecrets(t *testing.T) {
	a := newAuth(Config{ClientID: clientID, PublicOrigin: "https://x"}, &fakeDB{}, &oidc.StaticKeySet{}, time.Now)
	if err := a.Start(httptest.NewRecorder(), httptest.NewRequest("POST", "/", nil), "/admin", nil); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("got %v", err)
	}
}

func TestCallbackCreatesSession(t *testing.T) {
	h := newHarness(t)
	u, cookies := h.start(t, "/admin/posts?draft=1", nil)
	rec := h.callback(t, "code=c&state="+u.Query().Get("state"), cookies...)
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/posts?draft=1" {
		t.Fatalf("got %d %q", rec.Code, rec.Header().Get("Location"))
	}
	c := cookie(rec, SessionCookie)
	if c == nil || c.Path != "/admin" || !c.Secure || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 12*3600 {
		t.Fatalf("session cookie %+v", c)
	}
	if len(h.db.rows) != 1 {
		t.Fatalf("rows %v", h.db.rows)
	}
	for id, s := range h.db.rows {
		if id == c.Value || id != h.a.sessionID(c.Value) {
			t.Fatal("row id must be the HMAC of the cookie token, not the token")
		}
		if s.Email != "me@halid.org" || s.ExpiresAt-s.CreatedAt != 12*3600 || s.LastReauthAt != t0.Unix() {
			t.Fatalf("row %+v", s)
		}
	}
}

func TestCallbackRejections(t *testing.T) {
	cases := map[string]struct {
		claims  func(nonce string) map[string]any
		query   func(state string) string
		noFlow  bool
		tamper  bool
		wantKey string
		email   string
	}{
		"gmail account":  {claims: func(n string) map[string]any { c := claims(n); c["email"] = "x@gmail.com"; delete(c, "hd"); return c }, wantKey: "domain", email: "x@gmail.com"},
		"state mismatch": {query: func(string) string { return "code=c&state=forged" }, wantKey: "failed"},
		"google error":   {query: func(s string) string { return "error=access_denied&state=" + s }, wantKey: "failed"},
		"no flow cookie": {noFlow: true, wantKey: "failed"},
		"tampered flow":  {tamper: true, wantKey: "failed"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			if tc.claims != nil {
				h.token = func(string) map[string]any { return tc.claims(h.nonce) }
			}
			u, cookies := h.start(t, "/admin", nil)
			state := u.Query().Get("state")
			q := "code=c&state=" + state
			if tc.query != nil {
				q = tc.query(state)
			}
			if tc.noFlow {
				cookies = nil
			}
			if tc.tamper {
				cookies[0].Value = "x" + cookies[0].Value
			}
			rec := h.callback(t, q, cookies...)
			if rec.Header().Get("Location") != LoginPath || cookie(rec, SessionCookie) != nil || len(h.db.rows) != 0 {
				t.Fatalf("want rejection, got %d %q rows=%d", rec.Code, rec.Header().Get("Location"), len(h.db.rows))
			}
			req := httptest.NewRequest("GET", LoginPath, nil)
			req.AddCookie(cookie(rec, flashCookie))
			fl := h.a.TakeFlash(httptest.NewRecorder(), req)
			if fl == nil || fl.Kind != tc.wantKey || fl.Email != tc.email {
				t.Fatalf("flash %+v", fl)
			}
		})
	}
}

// login signs in through the real flow and returns the session cookie.
func (h *harness) login(t *testing.T) *http.Cookie {
	t.Helper()
	u, cookies := h.start(t, "/admin", nil)
	return cookie(h.callback(t, "code=c&state="+u.Query().Get("state"), cookies...), SessionCookie)
}

func TestReauth(t *testing.T) {
	cases := map[string]struct {
		email    string
		authTime any // nil omits the claim
		ok       bool
	}{
		"fresh auth_time":   {email: "me@halid.org", authTime: 0, ok: true},
		"stale auth_time":   {email: "me@halid.org", authTime: -11 * 60},
		"auth_time missing": {email: "me@halid.org"},
		"different account": {email: "other@halid.org", authTime: 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			sc := h.login(t)
			h.now = t0.Add(time.Hour)
			sess, _ := h.a.lookup(context.Background(), sc.Value)
			h.token = func(string) map[string]any {
				c := claims(h.nonce)
				c["iat"], c["exp"], c["email"] = h.now.Unix(), h.now.Add(time.Hour).Unix(), tc.email
				if d, ok := tc.authTime.(int); ok {
					c["auth_time"] = h.now.Unix() + int64(d)
				}
				return c
			}
			u, cookies := h.start(t, "/admin/posts", sess)
			rec := h.callback(t, "code=c&state="+u.Query().Get("state"), append(cookies, sc)...)
			after, _ := h.a.lookup(context.Background(), sc.Value)
			refreshed := after.LastReauthAt == h.now.Unix()
			if tc.ok != refreshed || (tc.ok && rec.Header().Get("Location") != "/admin/posts") {
				t.Fatalf("ok=%v refreshed=%v location=%q", tc.ok, refreshed, rec.Header().Get("Location"))
			}
		})
	}
}

func TestMiddleware(t *testing.T) {
	h := newHarness(t)
	sc := h.login(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(SessionFrom(r.Context()).Email))
	})
	protected := h.a.RequireSession(ok)
	recent := h.a.RequireSession(h.a.RequireRecentAuth(ok))

	do := func(hd http.Handler, method, target string, c *http.Cookie, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, target, nil)
		if c != nil {
			req.AddCookie(c)
		}
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		hd.ServeHTTP(rec, req)
		return rec
	}

	if rec := do(protected, "GET", "/admin", nil, nil); rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != LoginPath {
		t.Errorf("no cookie: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if rec := do(protected, "GET", "/admin", nil, map[string]string{"HX-Request": "true"}); rec.Code != http.StatusUnauthorized || rec.Header().Get("HX-Redirect") != LoginPath {
		t.Errorf("htmx: %d %v", rec.Code, rec.Header())
	}
	forged := &http.Cookie{Name: SessionCookie, Value: h.a.sessionID(sc.Value)}
	if rec := do(protected, "GET", "/admin", forged, nil); rec.Code != http.StatusSeeOther {
		t.Errorf("the stored hash must not work as a cookie: %d", rec.Code)
	}
	if rec := do(protected, "GET", "/admin", sc, nil); rec.Code != 200 || rec.Body.String() != "me@halid.org" || rec.Header().Get("Cache-Control") != "no-store" {
		t.Errorf("valid: %d %q", rec.Code, rec.Body.String())
	}

	h.now = t0.Add(9 * time.Minute)
	if rec := do(recent, "POST", "/admin/posts/1/publish", sc, nil); rec.Code != 200 {
		t.Errorf("9 min after login publish should pass: %d", rec.Code)
	}
	h.now = t0.Add(11 * time.Minute)
	rec := do(recent, "POST", "/admin/posts/1/publish", sc, map[string]string{"Referer": "https://staging.halid.org/admin/posts/1"})
	if rec.Code != http.StatusSeeOther || rec.Header().Get("Location") != "/admin/reauth?next=%2Fadmin%2Fposts%2F1" {
		t.Errorf("11 min: %d %q", rec.Code, rec.Header().Get("Location"))
	}

	h.now = t0.Add(12*time.Hour + time.Second)
	if rec := do(protected, "GET", "/admin", sc, nil); rec.Code != http.StatusSeeOther {
		t.Errorf("expired session: %d", rec.Code)
	}
}

func TestLogoutDeletesRow(t *testing.T) {
	h := newHarness(t)
	sc := h.login(t)
	req := httptest.NewRequest("POST", "/admin/logout", nil)
	req.AddCookie(sc)
	rec := httptest.NewRecorder()
	h.a.RequireSession(http.HandlerFunc(h.a.Logout)).ServeHTTP(rec, req)
	if len(h.db.rows) != 0 || cookie(rec, SessionCookie).MaxAge >= 0 || rec.Header().Get("Location") != LoginPath {
		t.Fatalf("rows=%d cookie=%+v", len(h.db.rows), cookie(rec, SessionCookie))
	}
}

func TestSafeNext(t *testing.T) {
	cases := map[string]string{
		"/admin":                     "/admin",
		"/admin/posts?x=1":           "/admin/posts?x=1",
		"":                           "/admin",
		"/":                          "/admin",
		"/administrator":             "/admin",
		"https://evil.example/admin": "/admin",
		"//evil.example/admin":       "/admin",
		"/\\evil.example":            "/admin",
		"/admin/../en/":              "/admin",
	}
	for in, want := range cases {
		if got := SafeNext(in); got != want {
			t.Errorf("SafeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

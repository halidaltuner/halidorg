// Package spike measures how the container reaches D1, KV and R2 (Phase 0,
// ADR-001). It is temporary: delete it once the access path is chosen and the
// real store client exists.
package spike

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strconv"
	"time"
)

// Config comes from the container environment, set by the Worker's Container class.
type Config struct {
	Enabled      bool
	InternalURL  string // method B: host intercepted by the Worker, never leaves Cloudflare
	PublicURL    string // method C: the Worker's public /__bridge route
	BridgeSecret string
	AccountID    string // method D: Cloudflare REST API
	APIToken     string
	D1ID         string
	KVNamespace  string
}

// FromEnv reads Config from the process environment.
func FromEnv() Config {
	return Config{
		Enabled:      os.Getenv("SPIKE_ENABLED") == "1",
		InternalURL:  envOr("BRIDGE_INTERNAL_URL", "http://bridge.internal"),
		PublicURL:    os.Getenv("BRIDGE_PUBLIC_URL"),
		BridgeSecret: os.Getenv("BRIDGE_SECRET"),
		AccountID:    os.Getenv("CF_ACCOUNT_ID"),
		APIToken:     os.Getenv("CF_API_TOKEN"),
		D1ID:         os.Getenv("CF_D1_ID"),
		KVNamespace:  os.Getenv("CF_KV_NAMESPACE_ID"),
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// Register mounts the spike endpoint when enabled.
func Register(mux *http.ServeMux, cfg Config) {
	if !cfg.Enabled {
		return
	}
	mux.HandleFunc("GET /x/spike/bridge", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if n < 1 || n > 100 {
			n = 20
		}
		method := r.URL.Query().Get("method")
		b, err := cfg.backend(method)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		res := Run(r.Context(), method, b, n)
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(res)
	})
}

// backend is one way of reaching the stores.
type backend interface {
	D1Select(ctx context.Context) error
	KVPut(ctx context.Context, key string, val []byte) error
	KVGet(ctx context.Context, key string) ([]byte, error)
	R2Put(ctx context.Context, key string, val []byte) error
	R2Get(ctx context.Context, key string) ([]byte, error)
}

func (c Config) backend(method string) (backend, error) {
	client := &http.Client{Timeout: 10 * time.Second}
	switch method {
	case "B":
		return &bridge{base: c.InternalURL, client: client}, nil
	case "C":
		if c.PublicURL == "" || c.BridgeSecret == "" {
			return nil, errors.New("method C needs BRIDGE_PUBLIC_URL and BRIDGE_SECRET")
		}
		return &bridge{base: c.PublicURL, secret: []byte(c.BridgeSecret), client: client}, nil
	case "D":
		if c.AccountID == "" || c.APIToken == "" || c.D1ID == "" || c.KVNamespace == "" {
			return nil, errors.New("method D needs CF_ACCOUNT_ID, CF_API_TOKEN, CF_D1_ID, CF_KV_NAMESPACE_ID")
		}
		return &restAPI{cfg: c, client: client}, nil
	}
	return nil, errors.New("method must be B, C or D")
}

// OpResult is latency for one operation over n runs.
type OpResult struct {
	OK    int     `json:"ok"`
	Fail  int     `json:"fail"`
	P50MS float64 `json:"p50_ms"`
	P95MS float64 `json:"p95_ms"`
	Error string  `json:"error,omitempty"`
}

// Result is the JSON body of /x/spike/bridge.
type Result struct {
	Method string              `json:"method"`
	N      int                 `json:"n"`
	Ops    map[string]OpResult `json:"ops"`
}

// Run executes each operation n times against b.
func Run(ctx context.Context, method string, b backend, n int) Result {
	key := "spike/" + method + "-probe"
	val := []byte("probe " + time.Now().UTC().Format(time.RFC3339))
	ops := []struct {
		name string
		fn   func(context.Context) error
	}{
		{"d1_select", b.D1Select},
		{"kv_put", func(ctx context.Context) error { return b.KVPut(ctx, key, val) }},
		{"kv_get", func(ctx context.Context) error { _, err := b.KVGet(ctx, key); return err }},
		{"r2_put", func(ctx context.Context) error { return b.R2Put(ctx, key, val) }},
		{"r2_get", func(ctx context.Context) error { _, err := b.R2Get(ctx, key); return err }},
	}
	res := Result{Method: method, N: n, Ops: map[string]OpResult{}}
	for _, op := range ops {
		var durs []float64
		var r OpResult
		for range n {
			start := time.Now()
			err := op.fn(ctx)
			if err != nil {
				r.Fail++
				r.Error = err.Error()
				continue
			}
			r.OK++
			durs = append(durs, float64(time.Since(start).Microseconds())/1000)
		}
		r.P50MS, r.P95MS = percentile(durs, 0.50), percentile(durs, 0.95)
		res.Ops[op.name] = r
	}
	return res
}

func percentile(xs []float64, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	s := slices.Clone(xs)
	slices.Sort(s)
	return s[int(p*float64(len(s)-1)+0.5)]
}

// bridge talks to the Worker's bridge handler (methods B and C share it).
type bridge struct {
	base   string
	secret []byte // empty for B: interception is not reachable from outside
	client *http.Client
}

func (b *bridge) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if len(b.secret) > 0 {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		req.Header.Set("X-Bridge-Ts", ts)
		req.Header.Set("X-Bridge-Sig", Sign(b.secret, ts, method, path, body))
	}
	return send(b.client, req)
}

func (b *bridge) D1Select(ctx context.Context) error {
	_, err := b.do(ctx, "POST", "/d1/query", []byte(`{"sql":"SELECT 1"}`))
	return err
}
func (b *bridge) KVPut(ctx context.Context, k string, v []byte) error {
	_, err := b.do(ctx, "PUT", "/kv/"+k, v)
	return err
}
func (b *bridge) KVGet(ctx context.Context, k string) ([]byte, error) {
	return b.do(ctx, "GET", "/kv/"+k, nil)
}
func (b *bridge) R2Put(ctx context.Context, k string, v []byte) error {
	_, err := b.do(ctx, "PUT", "/r2/"+k, v)
	return err
}
func (b *bridge) R2Get(ctx context.Context, k string) ([]byte, error) {
	return b.do(ctx, "GET", "/r2/"+k, nil)
}

// Sign is the bridge HMAC. worker/src/bridge.ts must compute the same string:
// ts, method, path and the body's sha256, newline-separated.
func Sign(secret []byte, ts, method, path string, body []byte) string {
	bodySum := sha256.Sum256(body)
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%s\n%s\n%s\n%s", ts, method, path, hex.EncodeToString(bodySum[:]))
	return hex.EncodeToString(mac.Sum(nil))
}

// restAPI uses Cloudflare's public REST API. R2 needs the S3 API with SigV4,
// which is not worth a dependency for a comparison; it reports unsupported.
type restAPI struct {
	cfg    Config
	client *http.Client
}

var errUnsupported = errors.New("not measured: R2 over REST needs the S3 API (SigV4)")

func (a *restAPI) url(path string) string {
	return "https://api.cloudflare.com/client/v4/accounts/" + a.cfg.AccountID + path
}

func (a *restAPI) do(ctx context.Context, method, url, ctype string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+a.cfg.APIToken)
	if ctype != "" {
		req.Header.Set("Content-Type", ctype)
	}
	return send(a.client, req)
}

func (a *restAPI) D1Select(ctx context.Context) error {
	_, err := a.do(ctx, "POST", a.url("/d1/database/"+a.cfg.D1ID+"/query"), "application/json", []byte(`{"sql":"SELECT 1"}`))
	return err
}
func (a *restAPI) KVPut(ctx context.Context, k string, v []byte) error {
	_, err := a.do(ctx, "PUT", a.url("/storage/kv/namespaces/"+a.cfg.KVNamespace+"/values/"+k), "text/plain", v)
	return err
}
func (a *restAPI) KVGet(ctx context.Context, k string) ([]byte, error) {
	return a.do(ctx, "GET", a.url("/storage/kv/namespaces/"+a.cfg.KVNamespace+"/values/"+k), "", nil)
}
func (a *restAPI) R2Put(context.Context, string, []byte) error   { return errUnsupported }
func (a *restAPI) R2Get(context.Context, string) ([]byte, error) { return nil, errUnsupported }

func send(c *http.Client, req *http.Request) ([]byte, error) {
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s: %d %s", req.Method, req.URL.Path, resp.StatusCode, bytes.TrimSpace(b))
	}
	return b, nil
}

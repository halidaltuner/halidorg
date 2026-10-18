package jobs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"

	"halid.org/engine/internal/i18n"
	"halid.org/engine/internal/store"
)

// PageMeta is the KV metadata the edge Worker reads (worker/src/cache.ts).
type PageMeta struct {
	ETag        string `json:"etag"`
	ContentType string `json:"contentType"`
	StoredAt    int64  `json:"storedAt"`
}

// CacheKey mirrors cacheKey in worker/src/cache.ts: page:{lang}:{path}, with
// "-" when the first segment is not a site language.
func CacheKey(path string) string {
	first := strings.SplitN(strings.TrimPrefix(path, "/"), "/", 2)[0]
	lang := "-"
	if i18n.IsSupported(first) {
		lang = first
	}
	return "page:" + lang + ":" + path
}

// cacheableTypes mirrors isCacheableResponse in worker/src/cache.ts.
var cacheableTypes = []string{
	"text/html", "application/rss+xml", "application/atom+xml",
	"application/feed+json", "application/xml", "text/plain",
}

// Cacheable reports whether the Worker would store this response.
func Cacheable(code int, h http.Header) bool {
	if code != http.StatusOK || h.Get("Set-Cookie") != "" {
		return false
	}
	ct := h.Get("Content-Type")
	okType := false
	for _, t := range cacheableTypes {
		if strings.HasPrefix(ct, t) {
			okType = true
		}
	}
	if !okType {
		return false
	}
	pub := false
	for _, d := range strings.Split(strings.ToLower(h.Get("Cache-Control")), ",") {
		switch strings.TrimSpace(d) {
		case "public":
			pub = true
		case "no-store", "private":
			return false
		}
	}
	return pub
}

// Warmer renders public paths in process and writes them to KV, so a purge
// leaves fresh pages behind instead of empty keys that would make the next
// visitor wait on the container (ADR-001).
type Warmer struct {
	app http.Handler
	kv  store.KV
	now func() time.Time
}

// NewWarmer renders through app, the same mux that serves the public site.
func NewWarmer(app http.Handler, kv store.KV) *Warmer {
	return &Warmer{app: app, kv: kv, now: time.Now}
}

// Result of warming one path.
type Result struct {
	Path   string
	Stored bool // false: the path no longer renders and its key was deleted
}

// Warm renders each path. A cacheable 200 is stored; anything else deletes
// the key. Errors stop the run so the job retries; warming is idempotent.
func (w *Warmer) Warm(ctx context.Context, paths []string) ([]Result, error) {
	out := make([]Result, 0, len(paths))
	for _, p := range paths {
		req := httptest.NewRequestWithContext(ctx, http.MethodGet, p, nil)
		rec := httptest.NewRecorder()
		w.app.ServeHTTP(rec, req)
		key := CacheKey(p)
		if !Cacheable(rec.Code, rec.Header()) {
			if err := w.kv.Delete(ctx, key); err != nil {
				return out, fmt.Errorf("warm %s: delete: %w", p, err)
			}
			out = append(out, Result{Path: p})
			continue
		}
		body := rec.Body.Bytes()
		etag := rec.Header().Get("ETag")
		if etag == "" {
			sum := sha256.Sum256(body)
			etag = `"` + hex.EncodeToString(sum[:8]) + `"`
		}
		meta := PageMeta{ETag: etag, ContentType: rec.Header().Get("Content-Type"), StoredAt: w.now().Unix()}
		if err := w.kv.Put(ctx, key, body, store.KVPut{Metadata: meta}); err != nil {
			return out, fmt.Errorf("warm %s: put: %w", p, err)
		}
		out = append(out, Result{Path: p, Stored: true})
	}
	return out, nil
}

package store

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeWorker mimics worker/src/bridge.ts closely enough to pin the wire format.
type fakeWorker struct {
	mu      sync.Mutex
	kv      map[string]string
	kvMeta  map[string]string
	kvTTL   map[string]string
	r2      map[string]string
	r2Type  map[string]string
	queries []Stmt
	d1Err   string
}

func newFake(t *testing.T) (*fakeWorker, Stores) {
	t.Helper()
	f := &fakeWorker{kv: map[string]string{}, kvMeta: map[string]string{}, kvTTL: map[string]string{}, r2: map[string]string{}, r2Type: map[string]string{}}
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	return f, NewBridge(srv.URL, srv.Client())
}

func (f *fakeWorker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	parts := strings.SplitN(r.URL.EscapedPath(), "/", 3)
	store, rawKey := parts[1], parts[2]
	if strings.Contains(rawKey, "/") {
		http.Error(w, "key must arrive as one escaped segment", http.StatusBadRequest)
		return
	}
	key, _ := url.PathUnescape(rawKey)
	body, _ := io.ReadAll(r.Body)

	switch store {
	case "d1":
		if f.d1Err != "" {
			http.Error(w, f.d1Err, http.StatusInternalServerError)
			return
		}
		row := json.RawMessage(`{"id":"01J","n":2}`)
		res := Result{Rows: []json.RawMessage{row}, Changes: 1, LastRowID: 7}
		if key == "query" {
			var st Stmt
			_ = json.Unmarshal(body, &st)
			f.queries = append(f.queries, st)
			_ = json.NewEncoder(w).Encode(res)
			return
		}
		var b struct{ Statements []Stmt }
		_ = json.Unmarshal(body, &b)
		f.queries = append(f.queries, b.Statements...)
		out := make([]Result, len(b.Statements))
		for i := range out {
			out[i] = res
		}
		_ = json.NewEncoder(w).Encode(out)
	case "kv", "r2":
		m := f.kv
		if store == "r2" {
			m = f.r2
		}
		switch r.Method {
		case http.MethodGet:
			v, ok := m[key]
			if !ok {
				http.Error(w, "not found", http.StatusNotFound)
				return
			}
			if store == "r2" {
				w.Header().Set("Content-Type", f.r2Type[key])
				w.Header().Set("ETag", `"e1"`)
			}
			_, _ = io.WriteString(w, v)
		case http.MethodPut:
			m[key] = string(body)
			f.kvMeta[key] = r.Header.Get("X-KV-Metadata")
			f.kvTTL[key] = r.Header.Get("X-KV-TTL")
			f.r2Type[key] = r.Header.Get("Content-Type")
			w.WriteHeader(http.StatusNoContent)
		case http.MethodDelete:
			delete(m, key)
			w.WriteHeader(http.StatusNoContent)
		}
	}
}

func TestQuerySendsParamsAndScans(t *testing.T) {
	f, s := newFake(t)
	res, err := s.DB.Query(context.Background(), "SELECT id, n FROM t WHERE id = ? AND n > ?", "01J", 1)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.queries[0]; got.SQL != "SELECT id, n FROM t WHERE id = ? AND n > ?" || len(got.Args) != 2 || got.Args[0] != "01J" {
		t.Fatalf("sent %+v", got)
	}
	if res.Changes != 1 || res.LastRowID != 7 {
		t.Fatalf("meta %+v", res)
	}
	rows, err := Scan[struct {
		ID string `json:"id"`
		N  int64  `json:"n"`
	}](res)
	if err != nil || len(rows) != 1 || rows[0].ID != "01J" || rows[0].N != 2 {
		t.Fatalf("scan %+v %v", rows, err)
	}
}

func TestBatch(t *testing.T) {
	f, s := newFake(t)
	res, err := s.DB.Batch(context.Background(),
		Stmt{SQL: "INSERT INTO t VALUES (?)", Args: []any{"a"}},
		Stmt{SQL: "DELETE FROM t WHERE id = ?", Args: []any{"b"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 || len(f.queries) != 2 || f.queries[1].SQL != "DELETE FROM t WHERE id = ?" {
		t.Fatalf("res %+v queries %+v", res, f.queries)
	}
}

func TestD1ErrorCarriesMessage(t *testing.T) {
	f, s := newFake(t)
	f.d1Err = "UNIQUE constraint failed: t.id"
	_, err := s.DB.Query(context.Background(), "INSERT INTO t VALUES (1)")
	var se *StatusError
	if !errors.As(err, &se) || se.Code != 500 || !strings.Contains(se.Body, "UNIQUE constraint") {
		t.Fatalf("got %v", err)
	}
}

func TestKVRoundTripWithAwkwardKey(t *testing.T) {
	f, s := newFake(t)
	ctx := context.Background()
	key := "page:en:/en/posts/çay?x"
	meta := map[string]string{"etag": `"abc"`}
	if err := s.KV.Put(ctx, key, []byte("<html>"), KVPut{TTL: 2 * time.Minute, Metadata: meta}); err != nil {
		t.Fatal(err)
	}
	if f.kvTTL[key] != "120" || f.kvMeta[key] != `{"etag":"\"abc\""}` {
		t.Fatalf("ttl %q meta %q", f.kvTTL[key], f.kvMeta[key])
	}
	got, err := s.KV.Get(ctx, key)
	if err != nil || string(got) != "<html>" {
		t.Fatalf("get %q %v", got, err)
	}
	if err := s.KV.Delete(ctx, key); err != nil {
		t.Fatal(err)
	}
	if _, err := s.KV.Get(ctx, key); !errors.Is(err, ErrNotFound) {
		t.Fatalf("after delete: %v", err)
	}
}

func TestBlobRoundTrip(t *testing.T) {
	_, s := newFake(t)
	ctx := context.Background()
	if err := s.Blob.Put(ctx, "media/01J/a.png", []byte("png"), "image/png"); err != nil {
		t.Fatal(err)
	}
	obj, err := s.Blob.Get(ctx, "media/01J/a.png")
	if err != nil || string(obj.Body) != "png" || obj.ContentType != "image/png" || obj.ETag != `"e1"` {
		t.Fatalf("get %+v %v", obj, err)
	}
	if _, err := s.Blob.Get(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}

func TestContextCancel(t *testing.T) {
	_, s := newFake(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.KV.Get(ctx, "k"); !errors.Is(err, context.Canceled) {
		t.Fatalf("got %v", err)
	}
}

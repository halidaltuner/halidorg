package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// maxBody bounds what one bridge response may hold in memory.
const maxBody = 32 << 20

// NewBridge returns stores that talk to the Worker bridge at base. A nil
// client gets a 15s timeout.
func NewBridge(base string, client *http.Client) Stores {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	b := &bridge{base: base, client: client}
	return Stores{DB: bridgeDB{b}, KV: bridgeKV{b}, Blob: bridgeBlob{b}}
}

type bridge struct {
	base   string
	client *http.Client
}

// StatusError is a non-2xx bridge response. D1 errors arrive as 500 with
// SQLite's message as the body.
type StatusError struct {
	Method, Path string
	Code         int
	Body         string
}

func (e *StatusError) Error() string {
	return fmt.Sprintf("store: %s %s: %d %s", e.Method, e.Path, e.Code, e.Body)
}

// do sends one request. Keys are path-escaped as a single segment, so a key
// may contain "/" and ":" (page:en:/en/posts/x).
func (b *bridge) do(ctx context.Context, method, path string, body []byte, header http.Header) (*http.Response, []byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, b.base+path, bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return nil, nil, err
	}
	if resp.StatusCode == http.StatusNotFound && method == http.MethodGet {
		return resp, nil, ErrNotFound
	}
	if resp.StatusCode >= 300 {
		return resp, nil, &StatusError{Method: method, Path: path, Code: resp.StatusCode, Body: string(bytes.TrimSpace(data))}
	}
	return resp, data, nil
}

func keyPath(store, key string) string { return "/" + store + "/" + url.PathEscape(key) }

type bridgeDB struct{ b *bridge }

func (d bridgeDB) Query(ctx context.Context, sql string, args ...any) (Result, error) {
	body, err := json.Marshal(Stmt{SQL: sql, Args: args})
	if err != nil {
		return Result{}, err
	}
	_, data, err := d.b.do(ctx, http.MethodPost, "/d1/query", body, jsonHeader())
	if err != nil {
		return Result{}, err
	}
	var res Result
	return res, json.Unmarshal(data, &res)
}

func (d bridgeDB) Batch(ctx context.Context, stmts ...Stmt) ([]Result, error) {
	body, err := json.Marshal(struct {
		Statements []Stmt `json:"statements"`
	}{stmts})
	if err != nil {
		return nil, err
	}
	_, data, err := d.b.do(ctx, http.MethodPost, "/d1/batch", body, jsonHeader())
	if err != nil {
		return nil, err
	}
	var res []Result
	return res, json.Unmarshal(data, &res)
}

func jsonHeader() http.Header { return http.Header{"Content-Type": {"application/json"}} }

type bridgeKV struct{ b *bridge }

func (k bridgeKV) Get(ctx context.Context, key string) ([]byte, error) {
	_, data, err := k.b.do(ctx, http.MethodGet, keyPath("kv", key), nil, nil)
	return data, err
}

func (k bridgeKV) Put(ctx context.Context, key string, val []byte, opts KVPut) error {
	h := http.Header{}
	if opts.TTL > 0 {
		h.Set("X-KV-TTL", strconv.FormatInt(int64(opts.TTL/time.Second), 10))
	}
	if opts.Metadata != nil {
		meta, err := json.Marshal(opts.Metadata)
		if err != nil {
			return err
		}
		h.Set("X-KV-Metadata", string(meta))
	}
	_, _, err := k.b.do(ctx, http.MethodPut, keyPath("kv", key), val, h)
	return err
}

func (k bridgeKV) Delete(ctx context.Context, key string) error {
	_, _, err := k.b.do(ctx, http.MethodDelete, keyPath("kv", key), nil, nil)
	return err
}

type bridgeBlob struct{ b *bridge }

func (o bridgeBlob) Get(ctx context.Context, key string) (Object, error) {
	resp, data, err := o.b.do(ctx, http.MethodGet, keyPath("r2", key), nil, nil)
	if err != nil {
		return Object{}, err
	}
	return Object{Body: data, ContentType: resp.Header.Get("Content-Type"), ETag: resp.Header.Get("ETag")}, nil
}

func (o bridgeBlob) Put(ctx context.Context, key string, body []byte, contentType string) error {
	h := http.Header{}
	if contentType != "" {
		h.Set("Content-Type", contentType)
	}
	_, _, err := o.b.do(ctx, http.MethodPut, keyPath("r2", key), body, h)
	return err
}

func (o bridgeBlob) Delete(ctx context.Context, key string) error {
	_, _, err := o.b.do(ctx, http.MethodDelete, keyPath("r2", key), nil, nil)
	return err
}

// Package store is the container's access to D1, KV and R2. The container has
// no bindings of its own (ADR-001): every call goes to the edge Worker's
// bridge at http://bridge.internal, which answers with the Worker's bindings.
package store

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"time"
)

// ErrNotFound is returned by KV and Blob reads of a missing key.
var ErrNotFound = errors.New("store: not found")

// Stmt is one SQL statement with positional (?) parameters.
type Stmt struct {
	SQL  string `json:"sql"`
	Args []any  `json:"params,omitempty"`
}

// Result is the outcome of one statement. Rows are raw JSON objects keyed by
// column name; decode them with Scan.
type Result struct {
	Rows      []json.RawMessage `json:"results"`
	Changes   int64             `json:"changes"`
	LastRowID int64             `json:"last_row_id"`
}

// DB is D1.
type DB interface {
	Query(ctx context.Context, sql string, args ...any) (Result, error)
	// Batch runs the statements in one D1 transaction: all apply or none do.
	Batch(ctx context.Context, stmts ...Stmt) ([]Result, error)
}

// KVPut tunes a KV write. Metadata must marshal to JSON (KV caps it at 1 KiB).
type KVPut struct {
	TTL      time.Duration // zero means no expiry; KV's minimum is 60s
	Metadata any
}

// KV is the page cache namespace.
type KV interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, val []byte, opts KVPut) error
	Delete(ctx context.Context, key string) error
}

// Object is an R2 object read back in full.
type Object struct {
	Body        []byte
	ContentType string
	ETag        string
}

// Blob is the R2 media bucket.
type Blob interface {
	Get(ctx context.Context, key string) (Object, error)
	Put(ctx context.Context, key string, body []byte, contentType string) error
	Delete(ctx context.Context, key string) error
}

// Queue is the jobs queue (Cloudflare Queues). The container cannot consume
// it; the Worker's queue handler delivers each message back to the container
// at POST /internal/jobs (prompts/_constraints.md).
type Queue interface {
	Send(ctx context.Context, msg any) error
}

// Stores bundles the stores the app uses. Search is the separate FTS5
// database (ADR-010).
type Stores struct {
	DB     DB
	Search DB
	KV     KV
	Blob   Blob
	Queue  Queue
}

// FromEnv returns the bridge-backed stores. BRIDGE_INTERNAL_URL overrides the
// bridge host, which tests and a bare `go run` outside the container need.
func FromEnv() Stores {
	base := os.Getenv("BRIDGE_INTERNAL_URL")
	if base == "" {
		base = "http://bridge.internal"
	}
	return NewBridge(base, nil)
}

// Scan decodes each row of res into a T using the T's json tags.
func Scan[T any](res Result) ([]T, error) {
	out := make([]T, 0, len(res.Rows))
	for _, raw := range res.Rows {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, nil
}

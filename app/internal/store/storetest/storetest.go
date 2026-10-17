// Package storetest provides in-memory stores for tests: a SQLite-backed DB
// with the repo's migrations applied, and map-backed KV and Blob. The DB
// returns rows in the same JSON shape the Worker bridge sends from D1, so
// repositories run their real SQL against the real schema.
package storetest

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite" // registers the "sqlite" driver

	"halid.org/engine/internal/store"
)

// repoRoot is the directory holding migrations/ and migrations-search/.
func repoRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..")
}

// DB is an in-memory SQLite database implementing store.DB.
type DB struct {
	db *sql.DB
	mu sync.Mutex
}

// NewDB returns a database with migrations/ applied, the main D1 schema.
func NewDB(t testing.TB) *DB { return open(t, "migrations") }

// NewSearchDB returns a database with migrations-search/ applied.
func NewSearchDB(t testing.TB) *DB { return open(t, "migrations-search") }

func open(t testing.TB, dir string) *DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatal(err)
	}
	// One connection: each :memory: connection is its own database.
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { db.Close() })

	files, err := filepath.Glob(filepath.Join(repoRoot(), dir, "*.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(src)); err != nil {
			t.Fatalf("%s: %v", filepath.Base(f), err)
		}
	}
	return &DB{db: db}
}

// Query runs one statement.
func (d *DB) Query(ctx context.Context, q string, args ...any) (store.Result, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return run(ctx, d.db, q, args)
}

// Batch runs the statements in one transaction, like D1's batch.
func (d *DB) Batch(ctx context.Context, stmts ...store.Stmt) ([]store.Result, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	out := make([]store.Result, 0, len(stmts))
	for _, st := range stmts {
		res, err := run(ctx, tx, st.SQL, st.Args)
		if err != nil {
			_ = tx.Rollback()
			return nil, err
		}
		out = append(out, res)
	}
	return out, tx.Commit()
}

type queryer interface {
	QueryContext(ctx context.Context, q string, args ...any) (*sql.Rows, error)
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

// run returns rows for statements that produce them, and changes otherwise,
// as D1 does. Arguments go through JSON first, as on the bridge, and bools
// bind as 1/0 like D1 binds them.
func run(ctx context.Context, q queryer, query string, args []any) (store.Result, error) {
	args, err := viaJSON(args)
	if err != nil {
		return store.Result{}, err
	}
	if returnsRows(query) {
		rows, err := q.QueryContext(ctx, query, args...)
		if err != nil {
			return store.Result{}, err
		}
		defer rows.Close()
		return scanRows(rows)
	}
	res, err := q.ExecContext(ctx, query, args...)
	if err != nil {
		return store.Result{}, err
	}
	n, _ := res.RowsAffected()
	id, _ := res.LastInsertId()
	return store.Result{Rows: []json.RawMessage{}, Changes: n, LastRowID: id}, nil
}

func returnsRows(q string) bool {
	s := strings.ToUpper(strings.TrimSpace(q))
	return strings.HasPrefix(s, "SELECT") || strings.HasPrefix(s, "WITH") ||
		strings.HasPrefix(s, "PRAGMA") || strings.Contains(s, "RETURNING")
}

func viaJSON(args []any) ([]any, error) {
	if len(args) == 0 {
		return nil, nil
	}
	b, err := json.Marshal(args)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	var out []any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	for i, a := range out {
		switch v := a.(type) {
		case json.Number:
			if n, err := v.Int64(); err == nil {
				out[i] = n
			} else {
				f, _ := v.Float64()
				out[i] = f
			}
		case bool:
			if v {
				out[i] = int64(1)
			} else {
				out[i] = int64(0)
			}
		}
	}
	return out, nil
}

func scanRows(rows *sql.Rows) (store.Result, error) {
	cols, err := rows.Columns()
	if err != nil {
		return store.Result{}, err
	}
	res := store.Result{Rows: []json.RawMessage{}}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return store.Result{}, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		raw, err := json.Marshal(m)
		if err != nil {
			return store.Result{}, err
		}
		res.Rows = append(res.Rows, raw)
	}
	return res, rows.Err()
}

// KV is a map-backed store.KV that records metadata and TTLs.
type KV struct {
	mu   sync.Mutex
	Vals map[string][]byte
	Meta map[string]any
	TTL  map[string]time.Duration
}

// NewKV returns an empty KV.
func NewKV() *KV {
	return &KV{Vals: map[string][]byte{}, Meta: map[string]any{}, TTL: map[string]time.Duration{}}
}

func (k *KV) Get(_ context.Context, key string) ([]byte, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	v, ok := k.Vals[key]
	if !ok {
		return nil, store.ErrNotFound
	}
	return v, nil
}

func (k *KV) Put(_ context.Context, key string, val []byte, opts store.KVPut) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	k.Vals[key] = append([]byte(nil), val...)
	k.Meta[key] = opts.Metadata
	k.TTL[key] = opts.TTL
	return nil
}

func (k *KV) Delete(_ context.Context, key string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	delete(k.Vals, key)
	delete(k.Meta, key)
	delete(k.TTL, key)
	return nil
}

// Keys returns the stored keys, sorted.
func (k *KV) Keys() []string {
	k.mu.Lock()
	defer k.mu.Unlock()
	out := make([]string, 0, len(k.Vals))
	for key := range k.Vals {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// Blob is a map-backed store.Blob.
type Blob struct {
	mu      sync.Mutex
	Objects map[string]store.Object
}

// NewBlob returns an empty Blob.
func NewBlob() *Blob { return &Blob{Objects: map[string]store.Object{}} }

func (b *Blob) Get(_ context.Context, key string) (store.Object, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	o, ok := b.Objects[key]
	if !ok {
		return store.Object{}, store.ErrNotFound
	}
	return o, nil
}

func (b *Blob) Put(_ context.Context, key string, body []byte, contentType string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.Objects[key] = store.Object{Body: append([]byte(nil), body...), ContentType: contentType, ETag: fmt.Sprintf("%x", len(body))}
	return nil
}

func (b *Blob) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.Objects, key)
	return nil
}

// Stores returns a full set of in-memory stores with the main schema applied.
func Stores(t testing.TB) store.Stores {
	return store.Stores{DB: NewDB(t), KV: NewKV(), Blob: NewBlob()}
}

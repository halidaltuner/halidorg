// Package jobs runs background work: the container enqueues through the
// store bridge, the Worker's queue consumer delivers each message back to
// POST /internal/jobs, and every attempt is recorded in job_runs.
//
// Delivery is at least once, so every handler must be idempotent.
package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"halid.org/engine/internal/content"
	"halid.org/engine/internal/store"
)

// InternalHeader marks requests the Worker sends itself. The Worker also
// refuses /internal/* from the internet, so this is a second check.
const InternalHeader = "X-Halid-Internal"

// MaxAttempts matches max_retries in wrangler.jsonc (3) plus the first try.
const MaxAttempts = 4

// Message is what travels on the queue.
type Message struct {
	ID      string          `json:"id"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// Handler does one job. A returned error makes the Worker retry the message.
type Handler func(ctx context.Context, payload json.RawMessage) error

// Permanent wraps an error that retrying cannot fix (bad payload): the run
// is marked dead at once and the message is acknowledged.
type Permanent struct{ Err error }

func (p Permanent) Error() string { return p.Err.Error() }
func (p Permanent) Unwrap() error { return p.Err }

// Jobs enqueues and runs jobs.
type Jobs struct {
	db       store.DB
	queue    store.Queue
	handlers map[string]Handler
	timeout  time.Duration
}

// New returns a runner with no handlers.
func New(db store.DB, q store.Queue) *Jobs {
	return &Jobs{db: db, queue: q, handlers: map[string]Handler{}, timeout: 2 * time.Minute}
}

// Handle registers the handler for a job type.
func (j *Jobs) Handle(typ string, h Handler) { j.handlers[typ] = h }

// Enqueue records a run and sends it. The row is written first so a failed
// send still leaves a visible "queued" run the dashboard can retry.
func (j *Jobs) Enqueue(ctx context.Context, typ string, payload any) (string, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	id := content.NewID()
	if _, err := j.db.Query(ctx, `INSERT INTO job_runs (id, type, payload, max_attempts, created_at) VALUES (?, ?, ?, ?, ?)`,
		id, typ, string(raw), MaxAttempts, content.Clock().Unix()); err != nil {
		return "", err
	}
	if err := j.queue.Send(ctx, Message{ID: id, Type: typ, Payload: raw}); err != nil {
		_, _ = j.db.Query(ctx, `UPDATE job_runs SET status = 'failed', error = ? WHERE id = ?`, "enqueue: "+err.Error(), id)
		return id, err
	}
	return id, nil
}

// Retry re-sends a failed or dead run with its stored payload.
func (j *Jobs) Retry(ctx context.Context, id string) error {
	res, err := j.db.Query(ctx, `SELECT type, payload FROM job_runs WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := store.Scan[struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}](res)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return content.NotFound("job", id)
	}
	if _, err := j.db.Query(ctx, `UPDATE job_runs SET status = 'queued', attempt = 0, error = '', finished_at = NULL WHERE id = ?`, id); err != nil {
		return err
	}
	return j.queue.Send(ctx, Message{ID: id, Type: rows[0].Type, Payload: json.RawMessage(rows[0].Payload)})
}

// Register adds POST /internal/jobs.
func (j *Jobs) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /internal/jobs", j.serve)
}

// serve runs one delivery. 200 tells the Worker to ack, 500 to retry.
func (j *Jobs) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Header.Get(InternalHeader) != "jobs" {
		http.NotFound(w, r)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	var msg Message
	if err := json.Unmarshal(body, &msg); err != nil || msg.ID == "" {
		// Nothing to record against; ack so it does not loop.
		slog.Error("job: bad message", "err", err)
		w.WriteHeader(http.StatusOK)
		return
	}
	attempt, _ := strconv.Atoi(r.Header.Get("X-Job-Attempt"))
	if attempt < 1 {
		attempt = 1
	}
	err = j.Run(r.Context(), msg, attempt)
	var perm Permanent
	if err != nil && !errors.As(err, &perm) {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// Run executes msg and records the outcome. It returns the handler's error.
func (j *Jobs) Run(ctx context.Context, msg Message, attempt int) error {
	now := func() int64 { return content.Clock().Unix() }
	if _, err := j.db.Query(ctx, `UPDATE job_runs SET status = 'running', attempt = ?, started_at = ? WHERE id = ?`,
		attempt, now(), msg.ID); err != nil {
		return err
	}
	h, ok := j.handlers[msg.Type]
	var runErr error
	if !ok {
		runErr = Permanent{fmt.Errorf("unknown job type %q", msg.Type)}
	} else {
		ctx, cancel := context.WithTimeout(ctx, j.timeout)
		runErr = h(ctx, msg.Payload)
		cancel()
	}
	status, errText := "done", ""
	var perm Permanent
	switch {
	case runErr == nil:
	case errors.As(runErr, &perm) || attempt >= MaxAttempts:
		status, errText = "dead", runErr.Error()
	default:
		status, errText = "failed", runErr.Error()
	}
	if runErr != nil {
		slog.Error("job failed", "id", msg.ID, "type", msg.Type, "attempt", attempt, "status", status, "err", runErr)
	}
	if _, err := j.db.Query(ctx, `UPDATE job_runs SET status = ?, error = ?, finished_at = ? WHERE id = ?`,
		status, errText, now(), msg.ID); err != nil && runErr == nil {
		return err
	}
	return runErr
}

// Run is a row of job_runs, for the dashboard.
type RunRow struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Status      string `json:"status"`
	Attempt     int    `json:"attempt"`
	MaxAttempts int    `json:"max_attempts"`
	Error       string `json:"error"`
	CreatedAt   int64  `json:"created_at"`
	FinishedAt  *int64 `json:"finished_at"`
}

// Failed lists failed and dead runs, newest first.
func (j *Jobs) Failed(ctx context.Context, limit int) ([]RunRow, error) {
	if limit <= 0 {
		limit = 20
	}
	res, err := j.db.Query(ctx, `SELECT id, type, status, attempt, max_attempts, error, created_at, finished_at
FROM job_runs WHERE status IN ('failed', 'dead') ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	return store.Scan[RunRow](res)
}

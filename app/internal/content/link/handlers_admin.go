package link

import (
	"errors"
	"log/slog"
	"net/http"

	"halid.org/engine/internal/admin"
	"halid.org/engine/internal/auth"
	"halid.org/engine/internal/content"
	"halid.org/engine/internal/jobs"
	"halid.org/engine/internal/store"
)

// Admin serves the link admin routes. For now: the read toggle on the
// public links page (web/static/js/links.js). The link editor comes with
// Quick Capture (Phase 2.4).
type Admin struct {
	repo *Repo
	db   store.DB
	auth *auth.Auth
	jobs *jobs.Jobs
}

// NewAdmin wires the link admin.
func NewAdmin(db store.DB, a *auth.Auth, j *jobs.Jobs) *Admin {
	return &Admin{repo: NewRepo(db), db: db, auth: a, jobs: j}
}

// Register adds the routes. The toggle is a write: admin.Guard (CSRF)
// inside a session, no recent sign-in needed (it is a reading log).
func (h *Admin) Register(mux *http.ServeMux) {
	mux.Handle("GET /admin/links/session", h.auth.RequireSession(http.HandlerFunc(h.session)))
	mux.Handle("POST /admin/links/{id}/read", admin.Guard(h.auth.RequireSession(http.HandlerFunc(h.setRead))))
}

// session answers 204 to a signed-in browser; RequireSession redirects
// anyone else to the login page.
func (h *Admin) session(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// setRead sets status=read|unread and re-renders the links page through
// the entry.changed job. It answers with the new status.
func (h *Admin) setRead(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	status := r.FormValue("status")
	if status != Read && status != Unread {
		http.Error(w, "status must be read or unread", http.StatusBadRequest)
		return
	}
	prev, err := jobs.RefOf(r.Context(), h.db, id)
	if err == nil && (prev == nil || prev.Kind != Kind) {
		err = content.NotFound(Kind, id)
	}
	if err == nil {
		err = h.repo.SetRead(r.Context(), id, status == Read)
	}
	if errors.Is(err, content.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		slog.Error("link read toggle", "id", id, "err", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if err := jobs.Enqueue(r.Context(), h.jobs, id, prev); err != nil {
		slog.Error("enqueue entry.changed", "entry", id, "err", err)
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte(status))
}

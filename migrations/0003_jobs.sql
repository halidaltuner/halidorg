-- Job runs (Phase 1.3). One row per enqueued job; the Worker's queue
-- consumer delivers the message to the container, which updates the row on
-- every attempt. The dashboard reads failed rows; retry re-sends payload.

CREATE TABLE job_runs (
  id           TEXT PRIMARY KEY,
  type         TEXT NOT NULL,
  payload      TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(payload)),
  status       TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'done', 'failed', 'dead')),
  attempt      INTEGER NOT NULL DEFAULT 0,
  max_attempts INTEGER NOT NULL DEFAULT 4,
  error        TEXT NOT NULL DEFAULT '',
  created_at   INTEGER NOT NULL,
  started_at   INTEGER,
  finished_at  INTEGER
);

CREATE INDEX job_runs_status ON job_runs (status, created_at DESC);

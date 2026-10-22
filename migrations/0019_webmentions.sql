-- Phase 8.1: received webmentions (CONTENT_MODEL.md §1 webmentions).
-- POST /webmention (the Worker) queues a webmention.receive job; the job
-- fetches the source, checks it links to the target, reads its h-entry and
-- keeps one row per source and target entry. Replies and mentions wait for
-- moderation (pending); likes and reposts show once verified (approved).
-- Rejected rows stay, so a source that sends again stays hidden.
CREATE TABLE webmentions (
  id              TEXT PRIMARY KEY,
  target_entry_id TEXT NOT NULL REFERENCES entries (id) ON DELETE CASCADE,
  target_url      TEXT NOT NULL,             -- as the sender wrote it
  source_url      TEXT NOT NULL,
  author          TEXT NOT NULL DEFAULT '',  -- plain text, from the h-card
  author_url      TEXT NOT NULL DEFAULT '',
  author_avatar   TEXT NOT NULL DEFAULT '',  -- /avatars/{hash}.{ext} in R2, or ''
  content         TEXT NOT NULL DEFAULT '',  -- plain text, never HTML
  type            TEXT NOT NULL CHECK (type IN ('reply', 'like', 'repost', 'mention')),
  status          TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
  published_at    INTEGER,                   -- the source's dt-published, when it has one
  received_at     INTEGER NOT NULL,
  updated_at      INTEGER NOT NULL,
  UNIQUE (source_url, target_entry_id)
);

CREATE INDEX webmentions_target ON webmentions (target_entry_id, status);
CREATE INDEX webmentions_status ON webmentions (status, received_at DESC);

-- The Worker counts recent webmention.receive runs to rate-limit the endpoint.
CREATE INDEX job_runs_type ON job_runs (type, created_at);

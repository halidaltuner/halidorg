-- Phase 8.2: webmentions sent on publish (CONTENT_MODEL.md §1).
-- The webmention.send job notifies every outside page an entry links to.
-- No foreign keys: an entry that is deleted or unpublished still needs its
-- old targets told (they then find the source gone), and these rows say
-- which ones.

-- What was last sent for an entry: from which URL, for which published
-- revision. The entry.changed job queues a send only when this differs.
CREATE TABLE webmention_sources (
  entry_id    TEXT PRIMARY KEY,
  source_url  TEXT NOT NULL,
  revision_id TEXT NOT NULL,
  sent_at     INTEGER NOT NULL
);

-- One row per outside link of the entry's last sent revision, with what
-- its webmention endpoint answered.
CREATE TABLE webmention_sends (
  entry_id   TEXT NOT NULL,
  target_url TEXT NOT NULL,
  endpoint   TEXT NOT NULL DEFAULT '',  -- '' when the target names none
  status     INTEGER NOT NULL DEFAULT 0, -- the endpoint's HTTP status, 0 when not sent
  error      TEXT NOT NULL DEFAULT '',
  sent_at    INTEGER NOT NULL,
  PRIMARY KEY (entry_id, target_url)
);

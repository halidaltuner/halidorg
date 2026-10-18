-- Phase 5.1: singleton pages (CONTENT_MODEL.md §2, page). About and Now are
-- entries of kind page whose slug is their template, so UNIQUE (kind, lang,
-- slug) on entries keeps one of each per language. Their structured
-- sections live in revisions.data_json.

CREATE TABLE pages (
  entry_id TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  template TEXT NOT NULL CHECK (template IN ('about', 'now'))
);

-- Every publish of a page, for /now/archive: the revision that went live
-- and when. Revisions are kept, so an old version reads its own data.
CREATE TABLE page_versions (
  entry_id     TEXT NOT NULL REFERENCES entries (id) ON DELETE CASCADE,
  revision_id  TEXT NOT NULL REFERENCES revisions (id) ON DELETE CASCADE,
  published_at INTEGER NOT NULL,
  PRIMARY KEY (entry_id, revision_id)
);

CREATE INDEX page_versions_time ON page_versions (entry_id, published_at DESC);

-- Phase 5.5: garden pages and runbooks (CONTENT_MODEL.md §2). Both are
-- long-form kinds written in the post editor: each also has a posts row
-- (reading time, mermaid), plus its own row below.

CREATE TABLE garden_pages (
  entry_id       TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  maturity       TEXT NOT NULL DEFAULT 'seedling' CHECK (maturity IN ('seedling', 'budding', 'evergreen')),
  last_tended_at INTEGER -- set on every publish
);

CREATE TABLE runbooks (
  entry_id      TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  platform      TEXT NOT NULL DEFAULT '',
  prerequisites TEXT NOT NULL DEFAULT '', -- one per line, shown as a checklist above the steps
  verified_on   INTEGER,                  -- the day the steps last worked, midnight unix UTC
  risk          TEXT NOT NULL DEFAULT 'low' CHECK (risk IN ('low', 'med', 'high'))
);

-- Phase 5.5: the glossary and the reading list (CONTENT_MODEL.md §2).
-- A term's entry title is the term; the body is the long explanation.
-- A book's entry title is the book's title, the summary a short take,
-- the body notes; its cover is the entry's cover_media_id.

CREATE TABLE terms (
  entry_id     TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  term         TEXT NOT NULL,              -- the title, kept for sorting the index
  aliases_json TEXT NOT NULL DEFAULT '[]', -- other names it goes by: ["key-value store"]
  short_def    TEXT NOT NULL DEFAULT ''    -- one or two sentences, on the index and the page lead
);

CREATE TABLE books (
  entry_id     TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  author       TEXT NOT NULL DEFAULT '',
  isbn         TEXT NOT NULL DEFAULT '',
  status       TEXT NOT NULL DEFAULT 'want' CHECK (status IN ('want', 'reading', 'done', 'dropped')),
  progress_pct INTEGER NOT NULL DEFAULT 0 CHECK (progress_pct BETWEEN 0 AND 100),
  started_at   INTEGER,
  finished_at  INTEGER,
  rating       INTEGER CHECK (rating IS NULL OR rating BETWEEN 1 AND 5)
);

CREATE INDEX books_status ON books (status);

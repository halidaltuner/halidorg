-- Phase 5.3: photos (CONTENT_MODEL.md §3). Gallery data: no entries, one
-- list for both languages with tr/en titles. The grid and the lightbox use
-- an image from media (ADR-009: no third-party load on the page); a photo
-- links out to its 500px page.

CREATE TABLE photo_albums (
  id      TEXT PRIMARY KEY,
  slug    TEXT NOT NULL UNIQUE CHECK (slug NOT GLOB '*[^a-z0-9-]*' AND length(slug) BETWEEN 1 AND 40),
  name_tr TEXT NOT NULL CHECK (length(name_tr) BETWEEN 1 AND 60),
  name_en TEXT NOT NULL CHECK (length(name_en) BETWEEN 1 AND 60),
  sort    INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE photos (
  id         TEXT PRIMARY KEY,
  album_id   TEXT NOT NULL REFERENCES photo_albums (id),
  media_id   TEXT NOT NULL REFERENCES media (id),
  title_tr   TEXT NOT NULL DEFAULT '',
  title_en   TEXT NOT NULL DEFAULT '',
  url_500px  TEXT CHECK (url_500px IS NULL OR url_500px GLOB 'https://500px.com/*'),
  camera     TEXT NOT NULL DEFAULT '',
  lens       TEXT NOT NULL DEFAULT '',
  exposure   TEXT NOT NULL DEFAULT '',            -- "1/250 · f/8 · ISO 160"
  place      TEXT NOT NULL DEFAULT '',
  taken_at   INTEGER,                             -- unix UTC, from EXIF or typed
  row_span   INTEGER NOT NULL DEFAULT 2 CHECK (row_span BETWEEN 1 AND 3),
  col_span   INTEGER NOT NULL DEFAULT 1 CHECK (col_span BETWEEN 1 AND 2),
  sort       INTEGER NOT NULL DEFAULT 0,
  published  INTEGER NOT NULL DEFAULT 0 CHECK (published IN (0, 1)),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  CHECK (title_tr <> '' OR title_en <> '')
);

CREATE INDEX photos_album ON photos (album_id);

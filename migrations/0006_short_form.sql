-- Phase 2.1: short-form and curation kinds (CONTENT_MODEL.md §2) and the
-- categories they are grouped by. Each kind table hangs off entries, so
-- slugs, languages, tags, revisions, search and publishing are shared.

CREATE TABLE categories (
  id        TEXT PRIMARY KEY,
  kind      TEXT NOT NULL CHECK (kind IN ('link', 'snippet', 'fav', 'homelab', 'awesome')),
  slug      TEXT NOT NULL CHECK (slug NOT GLOB '*[^a-z0-9-]*' AND length(slug) BETWEEN 1 AND 80),
  name_tr   TEXT NOT NULL DEFAULT '',
  name_en   TEXT NOT NULL DEFAULT '',
  tone      TEXT CHECK (tone IS NULL OR tone IN ('cyan', 'indigo', 'amber', 'green', 'orange', 'rose', 'violet', 'navy')),
  parent_id TEXT REFERENCES categories (id) ON DELETE SET NULL,
  sort      INTEGER NOT NULL DEFAULT 0,
  UNIQUE (kind, slug)
);

-- note (TIL): title optional, body is the note.
CREATE TABLE notes (
  entry_id   TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  source_url TEXT,
  til        INTEGER NOT NULL DEFAULT 0 CHECK (til IN (0, 1))
);

-- snippet: body_md is the description; the files are the snippet. Each
-- revision keeps a copy of the files in data_json, so diffs and restores
-- cover them too.
CREATE TABLE snippets (
  entry_id    TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  category_id TEXT REFERENCES categories (id) ON DELETE SET NULL,
  gist_url    TEXT
);

CREATE TABLE snippet_files (
  snippet_id TEXT NOT NULL REFERENCES snippets (entry_id) ON DELETE CASCADE,
  filename   TEXT NOT NULL CHECK (length(filename) BETWEEN 1 AND 120 AND filename NOT GLOB '*[/\]*'),
  language   TEXT NOT NULL DEFAULT '',
  code       TEXT NOT NULL DEFAULT '',
  sort       INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (snippet_id, filename)
);

-- link: entries.title is the link title, entries.summary is my comment.
CREATE TABLE links (
  entry_id     TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  url          TEXT NOT NULL CHECK (url GLOB 'https://*'),
  domain       TEXT NOT NULL DEFAULT '',
  favicon_url  TEXT,
  category_id  TEXT REFERENCES categories (id) ON DELETE SET NULL,
  via          TEXT,
  archived_url TEXT,
  read_status  TEXT NOT NULL DEFAULT 'unread' CHECK (read_status IN ('unread', 'read'))
);

CREATE INDEX links_category ON links (category_id);

CREATE TABLE favs (
  entry_id    TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  fav_type    TEXT NOT NULL CHECK (fav_type IN ('tool', 'repo', 'video', 'product', 'article', 'book', 'place', 'other')),
  author      TEXT,
  url         TEXT CHECK (url IS NULL OR url GLOB 'https://*'),
  rating      INTEGER CHECK (rating IS NULL OR rating BETWEEN 1 AND 5),
  category_id TEXT REFERENCES categories (id) ON DELETE SET NULL
);

CREATE INDEX favs_type ON favs (fav_type);

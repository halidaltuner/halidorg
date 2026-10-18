-- Phase 1 content core (CONTENT_MODEL.md §1, §2 post). IDs are ULIDs, times
-- are unix seconds UTC. Search lives in its own database (ADR-010): no
-- virtual tables here, or the nightly D1 export breaks.

CREATE TABLE media (
  id         TEXT PRIMARY KEY,
  r2_key     TEXT NOT NULL UNIQUE,
  mime       TEXT NOT NULL,
  width      INTEGER,
  height     INTEGER,
  alt_tr     TEXT NOT NULL DEFAULT '',
  alt_en     TEXT NOT NULL DEFAULT '',
  blurhash   TEXT,
  created_at INTEGER NOT NULL
);

CREATE TABLE series (
  id       TEXT PRIMARY KEY,
  slug     TEXT NOT NULL UNIQUE CHECK (slug NOT GLOB '*[^a-z0-9-]*' AND length(slug) BETWEEN 1 AND 80),
  title_tr TEXT NOT NULL DEFAULT '',
  title_en TEXT NOT NULL DEFAULT '',
  summary  TEXT NOT NULL DEFAULT '',
  status   TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'done', 'archived'))
);

CREATE TABLE entries (
  id                   TEXT PRIMARY KEY,
  kind                 TEXT NOT NULL CHECK (kind IN ('post', 'note', 'garden', 'runbook', 'snippet', 'link', 'fav', 'project', 'buildlog', 'presentation', 'book', 'term', 'radar_item', 'page')),
  slug                 TEXT NOT NULL CHECK (slug NOT GLOB '*[^a-z0-9-]*' AND length(slug) BETWEEN 1 AND 80),
  lang                 TEXT NOT NULL CHECK (lang IN ('tr', 'en')),
  translation_group_id TEXT NOT NULL,
  title                TEXT NOT NULL DEFAULT '',
  summary              TEXT NOT NULL DEFAULT '',
  status               TEXT NOT NULL DEFAULT 'draft' CHECK (status IN ('draft', 'scheduled', 'published', 'archived')),
  visibility           TEXT NOT NULL DEFAULT 'public' CHECK (visibility IN ('public', 'unlisted', 'private')),
  published_at         INTEGER,
  created_at           INTEGER NOT NULL,
  updated_at           INTEGER NOT NULL,
  -- current: newest revision (what the editor shows). published: what the
  -- public pages render; moves only on publish (ADR-006). Both are set in the
  -- same batch that writes the revision.
  current_revision_id   TEXT REFERENCES revisions (id),
  published_revision_id TEXT REFERENCES revisions (id),
  cover_media_id       TEXT REFERENCES media (id) ON DELETE SET NULL,
  tone                 TEXT CHECK (tone IN ('cyan', 'indigo', 'amber', 'green', 'orange', 'rose', 'violet', 'navy')),
  series_id            TEXT REFERENCES series (id) ON DELETE SET NULL,
  series_order         INTEGER,
  UNIQUE (kind, lang, slug),
  -- One sibling per language in a translation group.
  UNIQUE (translation_group_id, lang),
  CHECK (status NOT IN ('published', 'scheduled') OR published_at IS NOT NULL),
  CHECK (status <> 'published' OR published_revision_id IS NOT NULL)
);

CREATE INDEX entries_listing ON entries (kind, lang, status, published_at DESC);
CREATE INDEX entries_series ON entries (series_id, series_order);

-- body_html and toc_json are filled by the renderer; drafts keep them too so
-- the editor preview and publish share one code path (ADR-002).
CREATE TABLE revisions (
  id         TEXT PRIMARY KEY,
  entry_id   TEXT NOT NULL REFERENCES entries (id) ON DELETE CASCADE,
  body_md    TEXT NOT NULL DEFAULT '',
  body_html  TEXT NOT NULL DEFAULT '',
  toc_json   TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(toc_json)),
  data_json  TEXT NOT NULL DEFAULT '{}' CHECK (json_valid(data_json)),
  origin     TEXT NOT NULL CHECK (origin IN ('autosave', 'manual', 'publish', 'import')),
  created_at INTEGER NOT NULL,
  note       TEXT NOT NULL DEFAULT ''
);

CREATE INDEX revisions_entry ON revisions (entry_id, created_at DESC);

CREATE TABLE tags (
  id      TEXT PRIMARY KEY,
  slug    TEXT NOT NULL UNIQUE CHECK (slug NOT GLOB '*[^a-z0-9-]*' AND length(slug) BETWEEN 1 AND 80),
  name_tr TEXT NOT NULL DEFAULT '',
  name_en TEXT NOT NULL DEFAULT '',
  tone    TEXT CHECK (tone IN ('cyan', 'indigo', 'amber', 'green', 'orange', 'rose', 'violet', 'navy'))
);

CREATE TABLE entry_tags (
  entry_id TEXT NOT NULL REFERENCES entries (id) ON DELETE CASCADE,
  tag_id   TEXT NOT NULL REFERENCES tags (id) ON DELETE CASCADE,
  PRIMARY KEY (entry_id, tag_id)
);

CREATE INDEX entry_tags_tag ON entry_tags (tag_id);

CREATE TABLE posts (
  entry_id         TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  reading_minutes  INTEGER NOT NULL DEFAULT 0,
  canonical_url    TEXT NOT NULL DEFAULT '',
  mermaid          INTEGER NOT NULL DEFAULT 0 CHECK (mermaid IN (0, 1)),
  comments_enabled INTEGER NOT NULL DEFAULT 0 CHECK (comments_enabled IN (0, 1))
);

-- 410 rows are tombstones: to_path is empty.
CREATE TABLE redirects (
  from_path  TEXT PRIMARY KEY,
  to_path    TEXT NOT NULL DEFAULT '',
  code       INTEGER NOT NULL DEFAULT 301 CHECK (code IN (301, 302, 410)),
  created_at INTEGER NOT NULL,
  CHECK ((code = 410) = (to_path = ''))
);

-- Written by the Worker on every 404 (Phase 1.3), read by the dashboard.
CREATE TABLE not_found_log (
  path         TEXT PRIMARY KEY,
  hits         INTEGER NOT NULL DEFAULT 1,
  last_seen_at INTEGER NOT NULL
);

CREATE INDEX not_found_log_seen ON not_found_log (last_seen_at DESC);

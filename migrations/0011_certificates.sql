-- Phase 5.2: certificates (CONTENT_MODEL.md §3). Gallery data: no entry, no
-- detail page, the same list in both languages. Status (active, renew soon,
-- expired) is derived from expires_at at render time, never stored.

CREATE TABLE cert_issuers (
  id    TEXT PRIMARY KEY,
  slug  TEXT NOT NULL UNIQUE CHECK (slug NOT GLOB '*[^a-z0-9-]*' AND length(slug) BETWEEN 1 AND 40),
  name  TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 60),
  tone  TEXT NOT NULL CHECK (tone IN ('cyan', 'indigo', 'amber', 'green', 'orange', 'rose', 'violet', 'navy')),
  sort  INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE certificates (
  id             TEXT PRIMARY KEY,
  issuer_id      TEXT NOT NULL REFERENCES cert_issuers (id),
  name           TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 160),
  skills_json    TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(skills_json)),
  badge_media_id TEXT REFERENCES media (id),
  credly_url     TEXT CHECK (credly_url IS NULL OR credly_url GLOB 'https://*'),
  issued_at      INTEGER NOT NULL,             -- first day of the month, unix UTC
  expires_at     INTEGER,                      -- first day of the month it lapses; NULL never
  sort           INTEGER NOT NULL DEFAULT 0,
  published      INTEGER NOT NULL DEFAULT 0 CHECK (published IN (0, 1)),
  created_at     INTEGER NOT NULL,
  updated_at     INTEGER NOT NULL
);

CREATE INDEX certificates_issuer ON certificates (issuer_id);

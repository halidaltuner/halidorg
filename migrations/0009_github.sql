-- Phase 4.4: GitHub data synced every 6 hours (CONTENT_MODEL.md §4). The
-- sync owns these tables: it upserts repos and replaces the derived rows.
-- Repos are never deleted; one gone from GitHub is marked gone. pinned is
-- set by the first sync from the GitHub profile, then by the admin only.
-- Projects link to a repo by projects.github_repo (owner/name).

CREATE TABLE gh_repos (
  id          TEXT PRIMARY KEY,              -- GitHub node id
  full_name   TEXT NOT NULL UNIQUE,          -- owner/name
  name        TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  url         TEXT NOT NULL CHECK (url GLOB 'https://*'),
  stars       INTEGER NOT NULL DEFAULT 0,
  forks       INTEGER NOT NULL DEFAULT 0,
  language    TEXT NOT NULL DEFAULT '',      -- primary language
  topics_json TEXT NOT NULL DEFAULT '[]' CHECK (json_valid(topics_json)),
  pushed_at   INTEGER,
  created_at  INTEGER,
  archived    INTEGER NOT NULL DEFAULT 0 CHECK (archived IN (0, 1)),
  fork        INTEGER NOT NULL DEFAULT 0 CHECK (fork IN (0, 1)),
  gone        INTEGER NOT NULL DEFAULT 0 CHECK (gone IN (0, 1)), -- not in the last sync
  pinned      INTEGER NOT NULL DEFAULT 0 CHECK (pinned IN (0, 1)),
  pin_sort    INTEGER NOT NULL DEFAULT 0,
  synced_at   INTEGER NOT NULL
);

CREATE TABLE gh_languages (
  repo_id  TEXT NOT NULL REFERENCES gh_repos (id) ON DELETE CASCADE,
  language TEXT NOT NULL,
  bytes    INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (repo_id, language)
);

-- One row per week of the contribution calendar (Sunday, unix UTC).
CREATE TABLE gh_weekly (
  week_start    INTEGER PRIMARY KEY,
  contributions INTEGER NOT NULL DEFAULT 0
);

-- Notable activity: releases and new repos. id is stable per event, so a
-- re-sync does not duplicate rows.
CREATE TABLE gh_activity (
  id          TEXT PRIMARY KEY,
  type        TEXT NOT NULL CHECK (type IN ('release', 'repo')),
  repo        TEXT NOT NULL,                 -- owner/name
  title       TEXT NOT NULL,
  note        TEXT NOT NULL DEFAULT '',
  url         TEXT NOT NULL CHECK (url GLOB 'https://*'),
  occurred_at INTEGER NOT NULL
);

CREATE INDEX gh_activity_time ON gh_activity (occurred_at DESC);

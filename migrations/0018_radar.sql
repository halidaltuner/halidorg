-- Phase 6: the tech radar (CONTENT_MODEL.md §2 radar_item).
-- radar_items is the working copy the admin edits: one row per item, both
-- languages. Nothing in it is public. Freezing an edition copies every item
-- on the radar into radar_history for that edition and publishes the
-- item's entries (kind radar_item, one per language, one translation group),
-- whose title is the name and summary the note in that language.

CREATE TABLE radar_items (
  group_id   TEXT PRIMARY KEY,             -- translation_group_id of its entries
  slug       TEXT NOT NULL UNIQUE,         -- the entries' slug, kept when renamed
  name       TEXT NOT NULL,
  quadrant   TEXT NOT NULL CHECK (quadrant IN ('languages', 'techniques', 'tools', 'platforms')),
  ring       TEXT NOT NULL CHECK (ring IN ('adopt', 'trial', 'assess', 'hold')),
  note_tr    TEXT NOT NULL DEFAULT '',
  note_en    TEXT NOT NULL DEFAULT '',
  off        INTEGER NOT NULL DEFAULT 0,   -- 1: left out of the next edition
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL
);

CREATE TABLE radar_editions (
  id           TEXT PRIMARY KEY,
  label        TEXT NOT NULL UNIQUE,       -- 2026-10
  published_at INTEGER NOT NULL
);

-- One row per item per edition it is on: what the radar showed then. The
-- name and notes are copied, so old editions keep their words and survive
-- an item's deletion.
CREATE TABLE radar_history (
  group_id   TEXT NOT NULL,
  edition_id TEXT NOT NULL REFERENCES radar_editions (id) ON DELETE CASCADE,
  slug       TEXT NOT NULL,
  name       TEXT NOT NULL,
  quadrant   TEXT NOT NULL,
  ring       TEXT NOT NULL,
  note_tr    TEXT NOT NULL DEFAULT '',
  note_en    TEXT NOT NULL DEFAULT '',
  changed_at INTEGER NOT NULL,             -- the freeze time
  PRIMARY KEY (group_id, edition_id)
);

CREATE INDEX radar_history_edition ON radar_history (edition_id);

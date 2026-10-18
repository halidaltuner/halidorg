-- Phase 5.4: presentations (CONTENT_MODEL.md §2, replaces `talk`). The
-- abstract is entries + revisions; the rows below are the meta, the outline
-- and the related links. Each revision keeps the outline and links in
-- data_json, so restores bring them back. The deck is one self-contained
-- HTML file in R2 (decks/{entry_id}/index.html), served from the decks
-- origin only (ADR-007); a new upload replaces it in place.

CREATE TABLE presentations (
  entry_id          TEXT PRIMARY KEY REFERENCES entries (id) ON DELETE CASCADE,
  pres_type         TEXT NOT NULL DEFAULT 'talk' CHECK (pres_type IN ('talk', 'workshop', 'lightning')),
  event             TEXT NOT NULL DEFAULT '',
  city              TEXT NOT NULL DEFAULT '',
  held_on           INTEGER,                   -- the day it was given, midnight unix UTC
  talk_lang         TEXT NOT NULL DEFAULT 'tr' CHECK (talk_lang IN ('tr', 'en')), -- the language spoken
  slide_count       INTEGER NOT NULL DEFAULT 0 CHECK (slide_count >= 0),
  deck_r2_key       TEXT,                      -- set by the deck upload
  deck_version      TEXT,                      -- hash prefix of the deck: busts caches of the iframe URL
  recording_url     TEXT CHECK (recording_url IS NULL OR recording_url GLOB 'https://*'),
  recording_minutes INTEGER CHECK (recording_minutes IS NULL OR recording_minutes > 0),
  outline_json      TEXT NOT NULL DEFAULT '[]' -- [{"n": 3, "title": "Why render-on-publish"}]
);

CREATE INDEX presentations_type ON presentations (pres_type, held_on);

-- The "related" list under the aside buttons.
CREATE TABLE presentation_links (
  presentation_id TEXT NOT NULL REFERENCES presentations (entry_id) ON DELETE CASCADE,
  label           TEXT NOT NULL CHECK (length(label) BETWEEN 1 AND 120),
  url             TEXT NOT NULL,
  sort            INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX presentation_links_presentation ON presentation_links (presentation_id, sort);

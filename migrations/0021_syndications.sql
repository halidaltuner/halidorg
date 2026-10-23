-- Phase 8.3: POSSE copies of an entry (CONTENT_MODEL.md §1 syndications).
-- Nothing posts automatically: the editor offers share links (X, LinkedIn)
-- and the author pastes back the URL of each copy. The entry's page links
-- them as u-syndication.
CREATE TABLE syndications (
  entry_id  TEXT NOT NULL REFERENCES entries (id) ON DELETE CASCADE,
  network   TEXT NOT NULL,             -- x, linkedin, mastodon, bluesky, or the host
  url       TEXT NOT NULL,
  posted_at INTEGER NOT NULL,          -- when the URL was added
  sort      INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (entry_id, url)
);

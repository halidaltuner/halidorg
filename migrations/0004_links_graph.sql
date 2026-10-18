-- Backlinks (CONTENT_MODEL.md, links_graph). Rebuilt by the entry.changed
-- job from the published HTML of each entry: one row per internal link from
-- one entry to another. Admin uses the count for the typed-slug delete
-- confirmation; public pages list them under "backlinks".
CREATE TABLE links_graph (
  from_entry_id TEXT NOT NULL REFERENCES entries (id) ON DELETE CASCADE,
  to_entry_id   TEXT NOT NULL REFERENCES entries (id) ON DELETE CASCADE,
  context       TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (from_entry_id, to_entry_id),
  CHECK (from_entry_id <> to_entry_id)
);

CREATE INDEX links_graph_to ON links_graph (to_entry_id);

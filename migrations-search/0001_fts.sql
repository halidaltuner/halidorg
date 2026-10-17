-- Search index (ADR-010), in halidorg-search-{env}, never in the main
-- database: D1 cannot export a database that holds a virtual table.
-- Derived data only, rebuilt from the main database by a job; not backed up.
-- Text columns are folded in Go first (ı, İ, I -> i, internal/search), then
-- unicode61 strips the remaining diacritics.

CREATE VIRTUAL TABLE search_fts USING fts5 (
  entry_id UNINDEXED,
  kind UNINDEXED,
  lang UNINDEXED,
  status UNINDEXED,
  visibility UNINDEXED,
  slug UNINDEXED,
  title,
  summary,
  body,
  tags,
  tokenize = 'unicode61 remove_diacritics 2',
  prefix = '2 3'
);

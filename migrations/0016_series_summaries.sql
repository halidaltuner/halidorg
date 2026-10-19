-- Phase 5.5: a series has a summary per language. The old one-language
-- summary is copied into both and left unused.
ALTER TABLE series ADD COLUMN summary_tr TEXT NOT NULL DEFAULT '';
ALTER TABLE series ADD COLUMN summary_en TEXT NOT NULL DEFAULT '';
ALTER TABLE series ADD COLUMN sort INTEGER NOT NULL DEFAULT 0;
UPDATE series SET summary_tr = summary, summary_en = summary;

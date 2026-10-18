-- Phase 5.3: a plain-text description per photo and language, shown in the
-- lightbox under the title (CONTENT_MODEL.md §3).
ALTER TABLE photos ADD COLUMN description_tr TEXT NOT NULL DEFAULT '';
ALTER TABLE photos ADD COLUMN description_en TEXT NOT NULL DEFAULT '';

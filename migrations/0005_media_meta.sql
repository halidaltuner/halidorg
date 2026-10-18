-- Media library (Phase 1.7): the original file name and size, shown in the
-- admin grid. The R2 object lives at media.r2_key; alt_tr and alt_en must
-- both be filled before the admin lets an image be used.
ALTER TABLE media ADD COLUMN name TEXT NOT NULL DEFAULT '';
ALTER TABLE media ADD COLUMN bytes INTEGER NOT NULL DEFAULT 0;

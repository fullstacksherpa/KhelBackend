BEGIN;

DROP INDEX IF EXISTS idx_venues_fts;
ALTER TABLE venues DROP COLUMN IF EXISTS fts;

DROP INDEX IF EXISTS idx_venues_name_trgm;
DROP INDEX IF EXISTS idx_venues_status;

COMMIT;

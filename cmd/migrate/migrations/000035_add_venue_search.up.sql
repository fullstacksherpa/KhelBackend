BEGIN;

CREATE EXTENSION IF NOT EXISTS pg_trgm;

-- Fast partial/fuzzy matching for name (basic search)
CREATE INDEX IF NOT EXISTS idx_venues_name_trgm
  ON venues USING gin (name gin_trgm_ops);

-- Useful for filtering
CREATE INDEX IF NOT EXISTS idx_venues_status
  ON venues (status);

-- Full text: keep it tight (name + sport only)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_name = 'venues' AND column_name = 'fts'
  ) THEN
    ALTER TABLE venues
    ADD COLUMN fts tsvector GENERATED ALWAYS AS (
      setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
      setweight(to_tsvector('english', coalesce(sport, '')), 'A')
    ) STORED;
  END IF;

  IF NOT EXISTS (
    SELECT 1 FROM pg_indexes
    WHERE tablename = 'venues' AND indexname = 'idx_venues_fts'
  ) THEN
    CREATE INDEX idx_venues_fts ON venues USING gin (fts);
  END IF;
END$$;

COMMIT;

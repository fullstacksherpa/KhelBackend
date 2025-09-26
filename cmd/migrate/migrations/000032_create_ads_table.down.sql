
-- Drop trigger
DROP TRIGGER IF EXISTS update_ads_updated_at ON ads;

-- Drop function
DROP FUNCTION IF EXISTS update_updated_at_column;

-- Drop indexes
DROP INDEX IF EXISTS idx_ads_active_order;
DROP INDEX IF EXISTS idx_ads_created_at;

-- Drop table
DROP TABLE IF EXISTS ads;

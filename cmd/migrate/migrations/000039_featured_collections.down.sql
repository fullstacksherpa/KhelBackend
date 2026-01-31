BEGIN;

DROP MATERIALIZED VIEW IF EXISTS featured_collections_cache;

DROP TABLE IF EXISTS featured_items;
DROP TABLE IF EXISTS featured_collections;

COMMIT;

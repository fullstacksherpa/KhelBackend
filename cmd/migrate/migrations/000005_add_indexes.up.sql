-- Create the extension and indexes for full-text search
-- check the article: https://niallburkley.com/blog/index-columns-for-like-in-postgres/

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE INDEX IF NOT EXISTS idx_venues_name ON venues USING gin (name gin_trgm_ops);

CREATE INDEX  IF NOT EXISTS idx_venues_id ON venues (id);
CREATE INDEX IF NOT EXISTS idx_users_id ON users (id);

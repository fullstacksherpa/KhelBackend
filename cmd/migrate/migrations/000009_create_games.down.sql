-- Down Migration: Remove "games" table and trigger
DROP TRIGGER IF EXISTS update_games_modtime ON games;
DROP FUNCTION IF EXISTS update_modified_column();
DROP TABLE IF EXISTS games;

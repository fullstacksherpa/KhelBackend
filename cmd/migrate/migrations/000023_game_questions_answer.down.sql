-- Down Migration: Remove all objects in reverse creation order

-- 1. Drop triggers first (depend on the function)
DROP TRIGGER IF EXISTS update_game_question_replies_modtime ON game_question_replies;
DROP TRIGGER IF EXISTS update_game_questions_modtime ON game_questions;

-- 2. Drop the function (used by triggers)
DROP FUNCTION IF EXISTS update_modified_column();

-- 3. Drop indexes (depend on tables)
DROP INDEX IF EXISTS idx_replies_question_id;
DROP INDEX IF EXISTS idx_game_questions_game_id;

-- 4. Drop tables in reverse FK dependency order
DROP TABLE IF EXISTS game_question_replies;
DROP TABLE IF EXISTS game_questions;
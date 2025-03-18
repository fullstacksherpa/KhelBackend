-- Down Migration: Remove the trigger and function
DROP TRIGGER IF EXISTS enforce_max_players ON game_players;
DROP FUNCTION IF EXISTS check_max_players;

DROP TRIGGER IF EXISTS enforce_max_players ON game_players;

-- Rollback: Drop the corrected function
DROP FUNCTION IF EXISTS check_max_players;

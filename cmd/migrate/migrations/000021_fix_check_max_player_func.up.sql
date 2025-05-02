-- Drop the trigger first since it depends on the function
DROP TRIGGER IF EXISTS enforce_max_players ON game_players;

-- Drop the old function if it exists
DROP FUNCTION IF EXISTS check_max_players;

-- Create or replace the function with updated logic
CREATE OR REPLACE FUNCTION check_max_players()
RETURNS TRIGGER AS $$
DECLARE
    current_players INT;
    max_players INT;
BEGIN
    -- Count players currently in the game
    SELECT COUNT(*) INTO current_players FROM game_players WHERE game_id = NEW.game_id;
    
    -- Get max_players from the games table
    SELECT g.max_players INTO max_players FROM games g WHERE g.id = NEW.game_id;

    -- Raise exception if game is already full
    IF current_players >= max_players THEN
        RAISE EXCEPTION 'Cannot join: game is full';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create the trigger to enforce max players
CREATE TRIGGER enforce_max_players
BEFORE INSERT ON game_players
FOR EACH ROW EXECUTE FUNCTION check_max_players();

-- Up Migration: Create the trigger function and trigger
CREATE OR REPLACE FUNCTION check_max_players()
RETURNS TRIGGER AS $$
DECLARE
    current_players INT;
    max_players INT;
BEGIN
    -- Get current number of players for the game
    SELECT COUNT(*) INTO current_players FROM game_players WHERE game_id = NEW.game_id;
    
    -- Get max players limit for the game
    SELECT max_players INTO max_players FROM games WHERE id = NEW.game_id;

    -- Prevent insert if game is full
    IF current_players >= max_players THEN
        RAISE EXCEPTION 'Cannot join: game is full';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to enforce max players per game
CREATE TRIGGER enforce_max_players
BEFORE INSERT ON game_players
FOR EACH ROW EXECUTE FUNCTION check_max_players();

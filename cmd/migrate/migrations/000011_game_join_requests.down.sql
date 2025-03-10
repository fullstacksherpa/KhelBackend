BEGIN;

-- Drop the trigger first (it depends on the table)
DROP TRIGGER IF EXISTS update_game_join_requests_timestamp ON game_join_requests;

-- Drop the function next (it is used by the trigger)
DROP FUNCTION IF EXISTS update_timestamp;

-- Drop the table
DROP TABLE IF EXISTS game_join_requests;

-- Drop the ENUM type (if it was created)
DO $$ 
BEGIN
    IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'game_request_status') THEN
        DROP TYPE game_request_status;
    END IF;
END $$;

COMMIT;

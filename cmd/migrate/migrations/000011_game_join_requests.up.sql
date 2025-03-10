BEGIN;

-- Create ENUM type if not already created
DO $$ 
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'game_request_status') THEN
        CREATE TYPE game_request_status AS ENUM ('pending', 'accepted', 'rejected');
    END IF;
END $$;

-- Create game_join_requests table
CREATE TABLE IF NOT EXISTS game_join_requests (
    id BIGSERIAL PRIMARY KEY,
    game_id BIGINT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    status game_request_status NOT NULL DEFAULT 'pending',
    request_time TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP(0) WITH TIME ZONE DEFAULT NOW(),
    CONSTRAINT unique_game_user_request UNIQUE (game_id, user_id) -- Prevent duplicate requests
);

-- Create function to auto-update updated_at
CREATE OR REPLACE FUNCTION update_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to update updated_at on row update
CREATE TRIGGER update_game_join_requests_timestamp
BEFORE UPDATE ON game_join_requests
FOR EACH ROW
EXECUTE FUNCTION update_timestamp();

COMMIT;

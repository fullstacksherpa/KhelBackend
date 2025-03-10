-- Up Migration: Create "games" table and trigger
CREATE TABLE IF NOT EXISTS games (
    id BIGSERIAL PRIMARY KEY,
    sport_type VARCHAR(50) CHECK (sport_type IN ('futsal', 'basketball', 'badminton', 'e-sport', 'cricket', 'tennis')),
    price INT,
    format VARCHAR(20),
    venue_id INT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    admin_id INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    max_players INT NOT NULL CHECK (max_players > 0),
    game_level VARCHAR(20) CHECK (game_level IN ('beginner', 'intermediate', 'advanced')),
    start_time TIMESTAMP(0) WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP(0) WITH TIME ZONE NOT NULL,
    visibility VARCHAR(10) CHECK (visibility IN ('public', 'private')) NOT NULL DEFAULT 'public',
    instruction TEXT,
    status VARCHAR(10) CHECK (status IN ('active', 'cancelled', 'completed')) NOT NULL DEFAULT 'active',
    is_booked BOOLEAN DEFAULT FALSE,
    match_full BOOLEAN DEFAULT FALSE,
    created_at TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW()
);

-- Function to auto-update "updated_at" column before updates
CREATE OR REPLACE FUNCTION update_modified_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Trigger to call the function before updating any row
CREATE TRIGGER update_games_modtime 
BEFORE UPDATE ON games 
FOR EACH ROW EXECUTE FUNCTION update_modified_column();

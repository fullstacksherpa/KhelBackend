-- Create favorite_venues table
CREATE TABLE IF NOT EXISTS favorite_venues (
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, venue_id)
);

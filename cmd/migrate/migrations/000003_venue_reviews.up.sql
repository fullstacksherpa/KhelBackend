CREATE TABLE IF NOT EXISTS reviews (
    id BIGSERIAL PRIMARY KEY,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    rating INT NOT NULL CHECK (rating BETWEEN 1 AND 5),
    comment TEXT,
    created_at TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP(0) WITH TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_venue_reviews_venue_id ON reviews(venue_id);
CREATE INDEX idx_venue_reviews_user_id ON reviews(user_id);
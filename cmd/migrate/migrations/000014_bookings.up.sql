-- Create bookings table
CREATE TABLE IF NOT EXISTS bookings (
    id BIGSERIAL PRIMARY KEY,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    start_time TIMESTAMP WITH TIME ZONE NOT NULL,
    end_time TIMESTAMP WITH TIME ZONE NOT NULL,
    total_price INT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'confirmed',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
);



-- ENUM ('confirmed', 'pending', 'rejected', 'done');



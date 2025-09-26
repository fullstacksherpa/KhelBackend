-- 1. Create enum type
CREATE TYPE booking_status_enum AS ENUM ('pending', 'requested', 'booked', 'rejected', 'cancelled');

-- 2. Add new column with enum type (default 'pending')
ALTER TABLE games
    ADD COLUMN booking_status booking_status_enum NOT NULL DEFAULT 'pending';

-- 3. Drop old boolean column
ALTER TABLE games
    DROP COLUMN is_booked;

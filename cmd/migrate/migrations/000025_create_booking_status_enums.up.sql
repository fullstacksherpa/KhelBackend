-- 1. Create enum type
CREATE TYPE booking_status AS ENUM ('confirmed', 'pending', 'rejected', 'done');

-- 2. Drop default before altering the column
ALTER TABLE bookings
ALTER COLUMN status DROP DEFAULT;

-- 3. Alter column type from VARCHAR to ENUM using cast
ALTER TABLE bookings
ALTER COLUMN status TYPE booking_status
USING status::booking_status;

-- 4. Set the default again (now using ENUM type)
ALTER TABLE bookings
ALTER COLUMN status SET DEFAULT 'pending';

-- 5. Create indexes on user_id and venue_id
CREATE INDEX idx_bookings_user_id ON bookings(user_id);
CREATE INDEX idx_bookings_venue_id ON bookings(venue_id);

-- 1. Drop indexes
DROP INDEX IF EXISTS idx_bookings_user_id;
DROP INDEX IF EXISTS idx_bookings_venue_id;

-- 2. Drop default before changing type back
ALTER TABLE bookings
ALTER COLUMN status DROP DEFAULT;

-- 3. Change column type back to VARCHAR
ALTER TABLE bookings
ALTER COLUMN status TYPE VARCHAR(20)
USING status::text;

-- 4. Restore default as string
ALTER TABLE bookings
ALTER COLUMN status SET DEFAULT 'confirmed';

-- 5. Drop enum type
DROP TYPE IF EXISTS booking_status;

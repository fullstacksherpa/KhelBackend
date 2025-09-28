-- 1. Delete rows using 'canceled'
DELETE FROM bookings WHERE status = 'canceled';

-- 2. Drop default
ALTER TABLE bookings ALTER COLUMN status DROP DEFAULT;

-- 3. Create new enum without 'canceled'
CREATE TYPE booking_status_new AS ENUM ('confirmed', 'pending', 'rejected', 'done');

-- 4. Alter column to use new enum
ALTER TABLE bookings
ALTER COLUMN status TYPE booking_status_new
USING status::text::booking_status_new;

-- 5. Drop old enum type
DROP TYPE booking_status;

-- 6. Rename new enum to original name
ALTER TYPE booking_status_new RENAME TO booking_status;

-- 7. Restore default
ALTER TABLE bookings ALTER COLUMN status SET DEFAULT 'pending';

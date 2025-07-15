-- 1. Delete rows that use 'canceled' (or handle them another way)
DELETE FROM bookings WHERE status = 'canceled';

-- 2. Rename current enum
ALTER TYPE booking_status RENAME TO booking_status_old;

-- 3. Recreate enum without 'canceled'
CREATE TYPE booking_status AS ENUM ('confirmed', 'pending', 'rejected', 'done');

-- 4. Alter column to use new enum
ALTER TABLE bookings
ALTER COLUMN status TYPE booking_status
USING status::text::booking_status;

-- 5. Drop old enum type
DROP TYPE booking_status_old;

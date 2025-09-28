-- Drop default temporarily
ALTER TABLE bookings ALTER COLUMN status DROP DEFAULT;

-- Add the new value directly to the existing enum
ALTER TYPE booking_status ADD VALUE IF NOT EXISTS 'canceled';

-- Restore default if needed
ALTER TABLE bookings ALTER COLUMN status SET DEFAULT 'pending';

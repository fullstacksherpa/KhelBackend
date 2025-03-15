-- Down migration: Drop the trigger and function

-- Drop the trigger
DROP TRIGGER IF EXISTS update_bookings_modtime ON bookings;

-- Drop the function
DROP FUNCTION IF EXISTS update_bookings_modified_column();
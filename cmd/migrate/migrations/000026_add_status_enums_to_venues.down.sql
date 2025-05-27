-- 1. Remove the column
ALTER TABLE venues DROP COLUMN status;

-- 2. Drop the enum type
DROP TYPE venue_status;

-- 1. Re-add old column
ALTER TABLE games
    ADD COLUMN is_booked BOOLEAN DEFAULT FALSE;

-- 2. Drop new column
ALTER TABLE games
    DROP COLUMN booking_status;

-- 3. Drop enum type
DROP TYPE booking_status_enum;

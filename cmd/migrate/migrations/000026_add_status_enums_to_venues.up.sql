-- 1. Create the enum type
CREATE TYPE venue_status AS ENUM ('requested', 'active', 'rejected', 'hold');

-- 2. Add the column using the new enum type
ALTER TABLE venues ADD COLUMN status venue_status;

-- 3. Set default to 'requested' for future inserts (optional)
ALTER TABLE venues ALTER COLUMN status SET DEFAULT 'requested';

-- 4. Set status to 'active' for all existing records
UPDATE venues SET status = 'active' WHERE status IS NULL;

-- (Optional) 5. Add NOT NULL constraint if status should always be present
ALTER TABLE venues ALTER COLUMN status SET NOT NULL;

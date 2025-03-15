-- This table defines on which days and between which times the venue is open along with the price per hour.
CREATE TABLE IF NOT EXISTS venue_pricing (
    id BIGSERIAL PRIMARY KEY,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    day_of_week VARCHAR(10) NOT NULL,  -- e.g. 'monday', 'tuesday', …, 'sunday'
    start_time TIME NOT NULL,          -- e.g. 09:00:00
    end_time TIME NOT NULL,            -- e.g. 12:00:00
    price INT NOT NULL,                -- Price per hour for this slot
    CONSTRAINT valid_day CHECK (day_of_week IN ('sunday', 'monday', 'tuesday', 'wednesday', 'thursday', 'friday', 'saturday'))
);
-- Facilities are bookable units under one venue.
-- Example:
-- venue: "ABC Sports Center"
-- facilities: "Ground A", "Ground B", "Futsal Court", "Cricket Net"

CREATE TABLE IF NOT EXISTS facilities (
    id BIGSERIAL PRIMARY KEY,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,

    name VARCHAR(120) NOT NULL,
    description TEXT,

    -- Keep sport here because one venue may have different sports later.
    -- Example: one venue can have futsal + badminton.
    sport VARCHAR(50),

    -- Optional metadata for future filtering.
    surface_type VARCHAR(80),
    capacity INT,

    image_urls TEXT[] NOT NULL DEFAULT '{}',

    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),

    CONSTRAINT facilities_name_not_blank CHECK (length(trim(name)) > 0),
    CONSTRAINT facilities_capacity_positive CHECK (capacity IS NULL OR capacity > 0)
);

-- Only one default facility per venue.
CREATE UNIQUE INDEX IF NOT EXISTS facilities_one_default_per_venue_idx
ON facilities (venue_id)
WHERE is_default = TRUE;

CREATE INDEX IF NOT EXISTS facilities_venue_id_idx
ON facilities (venue_id);

CREATE INDEX IF NOT EXISTS facilities_venue_active_idx
ON facilities (venue_id, is_active);

-- Create one default facility for every existing venue.
-- This keeps old one-venue-one-ground behavior working.
INSERT INTO facilities (
    venue_id,
    name,
    description,
    sport,
    image_urls,
    is_default,
    is_active,
    created_at,
    updated_at
)
SELECT
    v.id,
    'Main Facility',
    v.description,
    v.sport,
    COALESCE(v.image_urls, '{}'),
    TRUE,
    TRUE,
    NOW(),
    NOW()
FROM venues v
WHERE NOT EXISTS (
    SELECT 1
    FROM facilities f
    WHERE f.venue_id = v.id
);

-- Add facility_id to bookings.
-- Keep venue_id for now because your current code, admin screens,
-- customer analytics, games, and earnings likely depend on it.
ALTER TABLE bookings
ADD COLUMN IF NOT EXISTS facility_id BIGINT;

-- Backfill old bookings to the default facility.
UPDATE bookings b
SET facility_id = f.id
FROM facilities f
WHERE f.venue_id = b.venue_id
  AND f.is_default = TRUE
  AND b.facility_id IS NULL;

ALTER TABLE bookings
ALTER COLUMN facility_id SET NOT NULL;

ALTER TABLE bookings
ADD CONSTRAINT bookings_facility_id_fkey
FOREIGN KEY (facility_id) REFERENCES facilities(id) ON DELETE RESTRICT;

CREATE INDEX IF NOT EXISTS bookings_facility_start_end_idx
ON bookings (facility_id, start_time, end_time);

CREATE INDEX IF NOT EXISTS bookings_facility_status_start_idx
ON bookings (facility_id, status, start_time);

-- Add facility_id to pricing slots.
ALTER TABLE venue_pricing
ADD COLUMN IF NOT EXISTS facility_id BIGINT;

-- Backfill old pricing to default facility.
UPDATE venue_pricing vp
SET facility_id = f.id
FROM facilities f
WHERE f.venue_id = vp.venue_id
  AND f.is_default = TRUE
  AND vp.facility_id IS NULL;

ALTER TABLE venue_pricing
ALTER COLUMN facility_id SET NOT NULL;

ALTER TABLE venue_pricing
ADD CONSTRAINT venue_pricing_facility_id_fkey
FOREIGN KEY (facility_id) REFERENCES facilities(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS venue_pricing_facility_day_start_idx
ON venue_pricing (facility_id, day_of_week, start_time);

-- Prevent exact duplicate pricing rows for same facility/day/time.
CREATE UNIQUE INDEX IF NOT EXISTS venue_pricing_facility_unique_slot_idx
ON venue_pricing (facility_id, day_of_week, start_time, end_time);
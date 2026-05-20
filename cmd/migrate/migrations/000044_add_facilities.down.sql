DROP INDEX IF EXISTS venue_pricing_facility_unique_slot_idx;
DROP INDEX IF EXISTS venue_pricing_facility_day_start_idx;

ALTER TABLE venue_pricing
DROP CONSTRAINT IF EXISTS venue_pricing_facility_id_fkey;

ALTER TABLE venue_pricing
DROP COLUMN IF EXISTS facility_id;

DROP INDEX IF EXISTS bookings_facility_status_start_idx;
DROP INDEX IF EXISTS bookings_facility_start_end_idx;

ALTER TABLE bookings
DROP CONSTRAINT IF EXISTS bookings_facility_id_fkey;

ALTER TABLE bookings
DROP COLUMN IF EXISTS facility_id;

DROP INDEX IF EXISTS facilities_venue_active_idx;
DROP INDEX IF EXISTS facilities_venue_id_idx;
DROP INDEX IF EXISTS facilities_one_default_per_venue_idx;

DROP TABLE IF EXISTS facilities;
-- Remove the unique index for confirmed bookings
DROP INDEX IF EXISTS unique_confirmed_bookings_per_venue_time;

-- Add unique index to prevent duplicate confirmed bookings for the same venue and time
CREATE UNIQUE INDEX IF NOT EXISTS unique_confirmed_bookings_per_venue_time
ON bookings (venue_id, start_time, end_time)
WHERE status = 'confirmed';


-- 👉 No two bookings can have the same venue_id, start_time, and end_time if they are both 'confirmed'.
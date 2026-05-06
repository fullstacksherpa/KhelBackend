CREATE INDEX IF NOT EXISTS idx_bookings_venue_user
ON bookings (venue_id, user_id);

CREATE INDEX IF NOT EXISTS idx_bookings_venue_user_created_at
ON bookings (venue_id, user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_booking_inventory_items_venue_booking
ON booking_inventory_items (venue_id, booking_id);
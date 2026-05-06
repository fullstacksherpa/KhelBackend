CREATE TABLE IF NOT EXISTS booking_inventory_items (
    id BIGSERIAL PRIMARY KEY,

    booking_id BIGINT NOT NULL REFERENCES bookings(id) ON DELETE CASCADE,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,
    inventory_item_id BIGINT NOT NULL REFERENCES venue_inventory_items(id) ON DELETE RESTRICT,

    item_name_snapshot VARCHAR(100) NOT NULL,
    unit_price_snapshot INT NOT NULL CHECK (unit_price_snapshot >= 0),

    quantity INT NOT NULL DEFAULT 1 CHECK (quantity > 0),

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_booking_inventory_item UNIQUE (booking_id, inventory_item_id)
);

CREATE INDEX idx_booking_inventory_items_booking_id
ON booking_inventory_items(booking_id);

CREATE INDEX idx_booking_inventory_items_venue_id
ON booking_inventory_items(venue_id);
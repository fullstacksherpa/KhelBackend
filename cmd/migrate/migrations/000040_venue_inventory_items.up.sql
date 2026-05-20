CREATE TABLE IF NOT EXISTS venue_inventory_items (
    id BIGSERIAL PRIMARY KEY,
    venue_id BIGINT NOT NULL REFERENCES venues(id) ON DELETE CASCADE,

    name VARCHAR(100) NOT NULL,
    description TEXT,
    unit_price INT NOT NULL CHECK (unit_price >= 0),

    image_url TEXT,

    stock_quantity INT CHECK (stock_quantity >= 0),
    track_stock BOOLEAN NOT NULL DEFAULT FALSE,

    is_active BOOLEAN NOT NULL DEFAULT TRUE,

    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),

    CONSTRAINT unique_inventory_item_per_venue UNIQUE (venue_id, name)
);

CREATE INDEX idx_venue_inventory_items_venue_id
ON venue_inventory_items(venue_id);
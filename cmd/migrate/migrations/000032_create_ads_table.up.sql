CREATE TABLE ads (
    id SERIAL PRIMARY KEY,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    image_url TEXT NOT NULL,
    image_alt VARCHAR(255), -- For accessibility
    link TEXT,
    active BOOLEAN DEFAULT TRUE,
    display_order INT DEFAULT 0,
    impressions INT DEFAULT 0,
    clicks INT DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    
    -- Constraints
    CONSTRAINT ads_display_order_positive CHECK (display_order >= 0),
    CONSTRAINT ads_impressions_positive CHECK (impressions >= 0),
    CONSTRAINT ads_clicks_positive CHECK (clicks >= 0)
);

-- Indexes for performance
CREATE INDEX idx_ads_active_order ON ads(active, display_order) WHERE active = TRUE;
CREATE INDEX idx_ads_created_at ON ads(created_at DESC);

-- Trigger for updated_at
CREATE OR REPLACE FUNCTION update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ language 'plpgsql';

CREATE TRIGGER update_ads_updated_at 
    BEFORE UPDATE ON ads 
    FOR EACH ROW 
    EXECUTE FUNCTION update_updated_at_column();
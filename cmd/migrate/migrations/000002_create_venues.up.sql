-- Enable PostGIS extension
CREATE EXTENSION IF NOT EXISTS postgis;

-- Create venues table
CREATE TABLE IF NOT EXISTS venues (
    id BIGSERIAL PRIMARY KEY,
    owner_id BIGSERIAL NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    address TEXT NOT NULL,
    location GEOGRAPHY(POINT, 4326) NOT NULL,
    description TEXT,
    amenities TEXT[], -- Array of venue amenities
    open_time TEXT, -- Store operating hours as a string
    image_urls TEXT[], -- Store multiple images as an array
    created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
    updated_at timestamp(0) with time zone NOT NULL DEFAULT NOW()
);

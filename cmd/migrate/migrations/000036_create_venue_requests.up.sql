CREATE EXTENSION IF NOT EXISTS postgis;

--  keep as TEXT for flexibility
-- status: requested | approved | rejected

CREATE TABLE IF NOT EXISTS venue_requests (
  id BIGSERIAL PRIMARY KEY,

  -- request payload
  name TEXT NOT NULL,
  address TEXT NOT NULL,
  location GEOGRAPHY(POINT, 4326) NOT NULL,

  description TEXT,
  amenities TEXT[] DEFAULT '{}'::text[],
  open_time TEXT,
  sport TEXT NOT NULL DEFAULT 'futsal',
  phone_number TEXT NOT NULL,

  -- moderation workflow
  status TEXT NOT NULL DEFAULT 'requested',
  admin_note TEXT,

  -- anti-abuse / auditing
  requester_ip TEXT,
  requester_user_agent TEXT,

  created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
  updated_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),

  approved_at timestamp(0) with time zone,
  approved_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
  rejected_at timestamp(0) with time zone,
  rejected_by BIGINT REFERENCES users(id) ON DELETE SET NULL
);

-- Helpful indexes
CREATE INDEX IF NOT EXISTS idx_venue_requests_status_created
  ON venue_requests (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_venue_requests_location_gist
  ON venue_requests
  USING GIST (location);



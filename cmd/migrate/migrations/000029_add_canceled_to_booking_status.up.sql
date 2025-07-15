-- Add new value to the existing enum
ALTER TYPE booking_status ADD VALUE IF NOT EXISTS 'canceled';

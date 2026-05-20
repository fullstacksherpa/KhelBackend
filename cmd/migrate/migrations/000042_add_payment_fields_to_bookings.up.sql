-- Add payment-related columns to bookings table
-- Safe: all columns are nullable to avoid breaking existing inserts

ALTER TABLE bookings
ADD COLUMN payment_method TEXT,
ADD COLUMN paid_amount INT,
ADD COLUMN final_amount INT,
ADD COLUMN paid_at TIMESTAMP WITH TIME ZONE;
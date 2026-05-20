-- Rollback: remove payment-related columns

ALTER TABLE bookings
DROP COLUMN IF EXISTS paid_at,
DROP COLUMN IF EXISTS final_amount,
DROP COLUMN IF EXISTS paid_amount,
DROP COLUMN IF EXISTS payment_method;
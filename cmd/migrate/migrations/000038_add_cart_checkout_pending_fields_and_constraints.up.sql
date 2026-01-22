-- Adds support for locking a cart during online payment ("checkout_pending")
-- and linking that cart to the created order (checkout_order_id).
--
-- Intended behavior:
-- - Online checkout:
--     carts.status = 'checkout_pending'
--     carts.checkout_order_id = <order.id>
--   Cart is locked so cart_items cannot be mutated during gateway flow.
--
-- - Payment success:
--     cart transitions to 'converted'
-- - Payment failure/cancel:
--     cart transitions back to 'active'
--     checkout_order_id cleared
--
-- This migration must be separate from enum additions due to Postgres "unsafe use of new enum value"
-- within a single transaction.

-- 1) Add checkout_order_id to carts (nullable)
ALTER TABLE carts
  ADD COLUMN IF NOT EXISTS checkout_order_id BIGINT;

-- 2) Foreign key to orders.id (nullable, ON DELETE SET NULL for safety)
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class t ON c.conrelid = t.oid
    WHERE t.relname = 'carts'
      AND c.conname = 'carts_checkout_order_id_fkey'
  ) THEN
    ALTER TABLE carts
      ADD CONSTRAINT carts_checkout_order_id_fkey
      FOREIGN KEY (checkout_order_id)
      REFERENCES orders(id)
      ON DELETE SET NULL;
  END IF;
END$$;

-- 3) Data hygiene constraint:
-- checkout_order_id may only be set when status is 'checkout_pending'.
-- Prevents accidental dangling references after unlocking/convert flows.
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_constraint c
    JOIN pg_class t ON c.conrelid = t.oid
    WHERE t.relname = 'carts'
      AND c.conname = 'carts_checkout_order_id_requires_pending'
  ) THEN
    ALTER TABLE carts
      ADD CONSTRAINT carts_checkout_order_id_requires_pending
      CHECK (
        checkout_order_id IS NULL OR status = 'checkout_pending'
      );
  END IF;
END$$;

-- 4) Enforce "one live cart per user" (active OR checkout_pending).
-- Your application logic assumes a single active cart; this makes it true at the DB level.
-- Partial unique index applies only to logged-in users (user_id is not null).
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_indexes
    WHERE tablename = 'carts'
      AND indexname = 'uniq_active_or_pending_cart_per_user'
  ) THEN
    CREATE UNIQUE INDEX uniq_active_or_pending_cart_per_user
      ON carts(user_id)
      WHERE user_id IS NOT NULL AND status IN ('active','checkout_pending');
  END IF;
END$$;

-- 5) Normalize any unexpected existing rows (defensive):
-- converted/abandoned carts should never carry checkout_order_id.
UPDATE carts
SET checkout_order_id = NULL
WHERE status IN ('converted','abandoned') AND checkout_order_id IS NOT NULL;

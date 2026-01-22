-- Adds new enum labels needed for robust checkout state transitions.
--
-- Why this migration exists as a standalone step:
-- Postgres (and Neon) disallow using newly-added enum values in the same transaction
-- where they are introduced (error: "unsafe use of new value ...").
-- Many migration tools (including golang-migrate) run each migration inside a single
-- transaction by default. Therefore we isolate enum additions into their own migration
-- and do not reference new labels elsewhere in this file.

-- carts.status
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_enum e
    JOIN pg_type t ON t.oid = e.enumtypid
    WHERE t.typname = 'cart_status' AND e.enumlabel = 'checkout_pending'
  ) THEN
    ALTER TYPE cart_status ADD VALUE 'checkout_pending';
  END IF;
END$$;

-- orders.status
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1
    FROM pg_enum e
    JOIN pg_type t ON t.oid = e.enumtypid
    WHERE t.typname = 'order_status' AND e.enumlabel = 'awaiting_payment'
  ) THEN
    ALTER TYPE order_status ADD VALUE 'awaiting_payment';
  END IF;

  IF NOT EXISTS (
    SELECT 1
    FROM pg_enum e
    JOIN pg_type t ON t.oid = e.enumtypid
    WHERE t.typname = 'order_status' AND e.enumlabel = 'payment_failed'
  ) THEN
    ALTER TYPE order_status ADD VALUE 'payment_failed';
  END IF;
END$$;

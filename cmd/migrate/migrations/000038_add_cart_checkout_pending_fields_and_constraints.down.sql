-- Reverts checkout-pending cart linkage changes.
-- Note: enum values remain (see step-1 down no-op rationale).

-- Drop CHECK first (depends on column)
ALTER TABLE carts
  DROP CONSTRAINT IF EXISTS carts_checkout_order_id_requires_pending;

-- Drop FK constraint
ALTER TABLE carts
  DROP CONSTRAINT IF EXISTS carts_checkout_order_id_fkey;

-- Drop unique partial index
DROP INDEX IF EXISTS uniq_active_or_pending_cart_per_user;

-- Drop column
ALTER TABLE carts
  DROP COLUMN IF EXISTS checkout_order_id;

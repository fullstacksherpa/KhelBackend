BEGIN;

-- Drop triggers that reference functions/objects
DROP TRIGGER IF EXISTS categories_set_updated_at ON categories;
DROP TRIGGER IF EXISTS brands_set_updated_at ON brands;
DROP TRIGGER IF EXISTS products_set_updated_at ON products;
DROP TRIGGER IF EXISTS product_variants_set_updated_at ON product_variants;
DROP TRIGGER IF EXISTS product_images_set_updated_at ON product_images;
DROP TRIGGER IF EXISTS carts_set_updated_at ON carts;
DROP TRIGGER IF EXISTS cart_items_set_updated_at ON cart_items;
DROP TRIGGER IF EXISTS orders_set_updated_at ON orders;
DROP TRIGGER IF EXISTS order_items_set_updated_at ON order_items;
DROP TRIGGER IF EXISTS payments_set_updated_at ON payments;
DROP TRIGGER IF EXISTS payment_logs_set_updated_at ON payment_logs;
DROP TRIGGER IF EXISTS order_status_history_set_updated_at ON order_status_history;
DROP TRIGGER IF EXISTS order_status_change_trigger ON orders;

-- Drop FK from orders -> payments (if exists)
ALTER TABLE IF EXISTS orders DROP CONSTRAINT IF EXISTS orders_primary_payment_id_fkey;
ALTER TABLE IF EXISTS orders DROP COLUMN IF EXISTS primary_payment_id;

-- Drop indexes for FTS first (if they exist)
DROP INDEX IF EXISTS idx_categories_fts;
DROP INDEX IF EXISTS idx_products_fts;

-- Drop tables (reverse dependency order)
DROP TABLE IF EXISTS payment_logs CASCADE;
DROP TABLE IF EXISTS payments CASCADE;
DROP TABLE IF EXISTS order_status_history CASCADE;
DROP TABLE IF EXISTS order_items CASCADE;
DROP TABLE IF EXISTS orders CASCADE;
DROP TABLE IF EXISTS cart_items CASCADE;
DROP TABLE IF EXISTS carts CASCADE;
DROP TABLE IF EXISTS product_images CASCADE;
DROP TABLE IF EXISTS product_variants CASCADE;

-- Drop products table (now has FTS column)
DROP TABLE IF EXISTS products CASCADE;

DROP TABLE IF EXISTS brands CASCADE;

-- Drop categories table last (due to self-reference and FTS column)
DROP TABLE IF EXISTS categories CASCADE;

-- Drop trigger functions
DROP FUNCTION IF EXISTS fn_set_updated_at() CASCADE;
DROP FUNCTION IF EXISTS log_order_status_change() CASCADE;

-- Drop types
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'payment_method') THEN
    DROP TYPE payment_method;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'payment_status') THEN
    DROP TYPE payment_status;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'order_status') THEN
    DROP TYPE order_status;
  END IF;
  IF EXISTS (SELECT 1 FROM pg_type WHERE typname = 'cart_status') THEN
    DROP TYPE cart_status;
  END IF;
END$$;

-- Extensions: be cautious — only drop if you created them and no other objects depend on them.
-- DROP EXTENSION IF EXISTS pg_trgm;

COMMIT;
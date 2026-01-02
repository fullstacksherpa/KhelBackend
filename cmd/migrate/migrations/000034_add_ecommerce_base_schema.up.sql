BEGIN;

-- Extensions
CREATE EXTENSION IF NOT EXISTS pg_trgm;


-- ENUM types
DO $$
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'cart_status') THEN
    CREATE TYPE cart_status AS ENUM ('active', 'converted', 'abandoned');
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'order_status') THEN
    CREATE TYPE order_status AS ENUM (
      'pending','processing','shipped','delivered','cancelled','refunded'
    );
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'payment_status') THEN
    CREATE TYPE payment_status AS ENUM (
      'pending','paid','failed','refunded','partially_refunded'
    );
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'payment_method') THEN
    CREATE TYPE payment_method AS ENUM ('esewa','khalti','cash_on_delivery','bank_transfer');
  END IF;
END$$;

-- Helper: update timestamp trigger function
CREATE OR REPLACE FUNCTION fn_set_updated_at()
RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;


-- CATALOG: categories, brands, tags with adjacency list model
CREATE TABLE IF NOT EXISTS categories (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL UNIQUE,
  parent_id BIGINT REFERENCES categories(id) ON DELETE SET NULL,
  image_urls TEXT[],
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- for fuzzy name matching
CREATE INDEX IF NOT EXISTS idx_categories_name_trgm ON categories USING gin (name gin_trgm_ops);
CREATE INDEX IF NOT EXISTS idx_categories_name_slug ON categories (name, slug);
CREATE INDEX IF NOT EXISTS idx_categories_active ON categories (is_active) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_categories_parent_id ON categories(parent_id);

-- Full-Text Search migration for categories
DO $$
BEGIN
  -- Add FTS column if it doesn't exist
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                WHERE table_name = 'categories' AND column_name = 'fts') THEN
    ALTER TABLE categories 
    ADD COLUMN fts tsvector GENERATED ALWAYS AS (
      setweight(to_tsvector('english', coalesce(name, '')), 'A')
    ) STORED;
  END IF;
  
  -- Create FTS index if it doesn't exist
  IF NOT EXISTS (SELECT 1 FROM pg_indexes 
                WHERE tablename = 'categories' AND indexname = 'idx_categories_fts') THEN
    CREATE INDEX idx_categories_fts ON categories USING gin (fts);
  END IF;
END$$;

CREATE TABLE IF NOT EXISTS brands (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL UNIQUE,
  slug TEXT UNIQUE,
  description TEXT,
  logo_url TEXT,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- PRODUCTS & VARIANTS
CREATE TABLE IF NOT EXISTS products (
  id BIGSERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT UNIQUE,
  description TEXT,
  category_id BIGINT REFERENCES categories(id) ON DELETE SET NULL,
  brand_id BIGINT REFERENCES brands(id) ON DELETE SET NULL,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- Indexes for products
CREATE INDEX IF NOT EXISTS idx_products_category_id ON products(category_id);
CREATE INDEX IF NOT EXISTS idx_products_brand_id ON products(brand_id);
CREATE INDEX IF NOT EXISTS idx_products_active ON products(is_active) WHERE is_active = true;
CREATE INDEX IF NOT EXISTS idx_products_name_trgm ON products USING gin (name gin_trgm_ops);

-- Full-Text Search migration for products
DO $$
BEGIN
  -- Add FTS column if it doesn't exist
  IF NOT EXISTS (SELECT 1 FROM information_schema.columns 
                WHERE table_name = 'products' AND column_name = 'fts') THEN
    ALTER TABLE products 
    ADD COLUMN fts tsvector GENERATED ALWAYS AS (
      setweight(to_tsvector('english', coalesce(name, '')), 'A') ||
      setweight(to_tsvector('english', coalesce(description, '')), 'B')
    ) STORED;
  END IF;
  
  -- Create FTS index if it doesn't exist
  IF NOT EXISTS (SELECT 1 FROM pg_indexes 
                WHERE tablename = 'products' AND indexname = 'idx_products_fts') THEN
    CREATE INDEX idx_products_fts ON products USING gin (fts);
  END IF;
END$$;

CREATE TABLE IF NOT EXISTS product_variants (
  id BIGSERIAL PRIMARY KEY,
  product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  price_cents BIGINT NOT NULL CHECK (price_cents >= 0),
  attributes JSONB DEFAULT '{}'::jsonb,
  is_active BOOLEAN NOT NULL DEFAULT TRUE,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_variants_product_id ON product_variants(product_id);

CREATE TABLE IF NOT EXISTS product_images (
  id BIGSERIAL PRIMARY KEY,
  product_id BIGINT NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  product_variant_id BIGINT REFERENCES product_variants(id) ON DELETE CASCADE,
  url TEXT NOT NULL,
  alt TEXT,
  is_primary BOOLEAN NOT NULL DEFAULT FALSE,
  sort_order INT NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_product_images_product_id ON product_images(product_id);
CREATE INDEX IF NOT EXISTS idx_product_images_variant_id ON product_images(product_variant_id);
CREATE INDEX IF NOT EXISTS idx_product_images_is_primary ON product_images(product_id, is_primary);

-- ... rest of your existing tables (carts, orders, payments, etc.) remain the same ...

-- Carts
CREATE TABLE IF NOT EXISTS carts (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT,                    -- NULL for guests
  guest_token TEXT,                  -- NULL for logged-in users
  status cart_status NOT NULL DEFAULT 'active',
  expires_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (
    (user_id IS NULL AND guest_token IS NOT NULL) OR
    (user_id IS NOT NULL AND guest_token IS NULL)
  )
);

CREATE INDEX IF NOT EXISTS idx_carts_user_id ON carts(user_id) WHERE user_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_carts_guest_token ON carts(guest_token) WHERE guest_token IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_carts_expires ON carts(expires_at) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS cart_items (
  id BIGSERIAL PRIMARY KEY,
  cart_id BIGINT NOT NULL REFERENCES carts(id) ON DELETE CASCADE,
  product_variant_id BIGINT NOT NULL REFERENCES product_variants(id) ON DELETE RESTRICT,
  quantity INT NOT NULL DEFAULT 1 CHECK (quantity > 0),
  price_cents BIGINT NOT NULL CHECK (price_cents >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (cart_id, product_variant_id)
);

-- ORDERS (initially create without primary_payment_id to avoid circular FKs)
CREATE TABLE IF NOT EXISTS orders (
  id BIGSERIAL PRIMARY KEY,
  user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
  order_number TEXT UNIQUE NOT NULL,
  cart_id BIGINT REFERENCES carts(id),
  status order_status NOT NULL DEFAULT 'pending',
  payment_status payment_status NOT NULL DEFAULT 'pending',
  payment_method payment_method,
  paid_at timestamptz,
  shipping_name TEXT NOT NULL,
  shipping_phone TEXT NOT NULL,
  shipping_address TEXT NOT NULL,
  shipping_city TEXT NOT NULL,
  shipping_postal_code TEXT,
  shipping_country TEXT DEFAULT 'Nepal',
  tracking_number TEXT,
  estimated_delivery timestamptz,
  subtotal_cents BIGINT NOT NULL CHECK (subtotal_cents >= 0),
  discount_cents BIGINT NOT NULL DEFAULT 0 CHECK (discount_cents >= 0),
  tax_cents BIGINT NOT NULL DEFAULT 0 CHECK (tax_cents >= 0),
  shipping_cents BIGINT NOT NULL DEFAULT 0 CHECK (shipping_cents >= 0),
  total_cents BIGINT NOT NULL CHECK (total_cents >= 0),
  notes TEXT,
  cancelled_reason TEXT,
  cancelled_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK (
    total_cents = subtotal_cents - discount_cents + tax_cents + shipping_cents
  ),
  CHECK (
    (payment_method = 'cash_on_delivery' AND payment_status IN ('pending','paid','failed')) OR
    (payment_method != 'cash_on_delivery' AND payment_status IN ('pending','paid','failed','refunded','partially_refunded'))
  ),
  CHECK (
    (paid_at IS NULL AND payment_status IN ('pending','failed')) OR
    (paid_at IS NOT NULL AND payment_status IN ('paid','refunded','partially_refunded'))
  ),
  CHECK (
    (cancelled_at IS NULL AND status != 'cancelled') OR
    (cancelled_at IS NOT NULL AND status = 'cancelled')
  )
);

-- ORDER ITEMS
CREATE TABLE IF NOT EXISTS order_items (
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  product_id BIGINT REFERENCES products(id),
  product_variant_id BIGINT REFERENCES product_variants(id),
  product_name TEXT NOT NULL,
  variant_attributes JSONB NOT NULL DEFAULT '{}',
  quantity INT NOT NULL CHECK (quantity > 0),
  unit_price_cents BIGINT NOT NULL CHECK (unit_price_cents >= 0),
  total_price_cents BIGINT NOT NULL CHECK (total_price_cents >= 0),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (total_price_cents = unit_price_cents * quantity)
);
CREATE INDEX IF NOT EXISTS idx_order_items_order ON order_items(order_id);
CREATE INDEX IF NOT EXISTS idx_order_items_product ON order_items(product_id);
CREATE INDEX IF NOT EXISTS idx_order_items_variant ON order_items(product_variant_id);

-- ORDER STATUS HISTORY
CREATE TABLE IF NOT EXISTS order_status_history (
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  old_status order_status,
  new_status order_status NOT NULL,
  note TEXT,
  created_by TEXT,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_order_status_history_order ON order_status_history(order_id);
CREATE INDEX IF NOT EXISTS idx_order_status_history_created ON order_status_history(created_at);

-- Trigger function to log order status changes
CREATE OR REPLACE FUNCTION log_order_status_change()
RETURNS TRIGGER AS $$
BEGIN
    IF OLD.status IS DISTINCT FROM NEW.status THEN
        INSERT INTO order_status_history (order_id, old_status, new_status, note, created_by)
        VALUES (NEW.id, OLD.status, NEW.status,
               CASE
                 WHEN NEW.status = 'shipped' THEN 'Tracking: ' || COALESCE(NEW.tracking_number, 'N/A')
                 WHEN NEW.status = 'cancelled' THEN 'Reason: ' || COALESCE(NEW.cancelled_reason, 'Not specified')
                 ELSE NULL
               END,
               'system'
        );
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger for orders status changes
DROP TRIGGER IF EXISTS order_status_change_trigger ON orders;
CREATE TRIGGER order_status_change_trigger
    AFTER UPDATE ON orders
    FOR EACH ROW
    EXECUTE FUNCTION log_order_status_change();

-- PAYMENTS (single source of truth for gateway ids and raw payloads)
CREATE TABLE IF NOT EXISTS payments (
  id BIGSERIAL PRIMARY KEY,
  order_id BIGINT NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  provider TEXT NOT NULL,               -- e.g. 'esewa', 'khalti', 'stripe'
  provider_ref TEXT,                    -- gateway transaction reference (unique when provided)
  amount_cents BIGINT NOT NULL CHECK (amount_cents >= 0),
  currency TEXT DEFAULT 'NPR',
  status payment_status NOT NULL DEFAULT 'pending',
  gateway_response JSONB,               -- raw gateway response webhook / API response
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (provider, provider_ref)       -- avoid duplicates for same provider_ref
);
CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id);

-- Payment logs (store raw request/response/webhook/error payloads)
CREATE TABLE IF NOT EXISTS payment_logs (
    id BIGSERIAL PRIMARY KEY,
    payment_id BIGINT REFERENCES payments(id) ON DELETE CASCADE,
    log_type TEXT CHECK (log_type IN ('request','response','webhook','error')),
    payload JSONB,
    created_at timestamptz DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_payment_logs_payment_id ON payment_logs(payment_id);

-- Now link orders -> payments via primary_payment_id (nullable)
ALTER TABLE orders
  ADD COLUMN IF NOT EXISTS primary_payment_id BIGINT;

-- Add foreign key constraint referencing payments.id after payments created
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint c
    JOIN pg_class t ON c.conrelid = t.oid
    WHERE t.relname = 'orders' AND c.conname = 'orders_primary_payment_id_fkey'
  ) THEN
    ALTER TABLE orders
      ADD CONSTRAINT orders_primary_payment_id_fkey FOREIGN KEY (primary_payment_id) REFERENCES payments(id) ON DELETE SET NULL;
  END IF;
END$$;

-- Add triggers to set updated_at on updates for many tables

CREATE TRIGGER categories_set_updated_at BEFORE UPDATE ON categories FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
CREATE TRIGGER brands_set_updated_at BEFORE UPDATE ON brands FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
CREATE TRIGGER products_set_updated_at BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
CREATE TRIGGER product_variants_set_updated_at BEFORE UPDATE ON product_variants FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
CREATE TRIGGER product_images_set_updated_at BEFORE UPDATE ON product_images FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
CREATE TRIGGER carts_set_updated_at BEFORE UPDATE ON carts FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
CREATE TRIGGER cart_items_set_updated_at BEFORE UPDATE ON cart_items FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
CREATE TRIGGER orders_set_updated_at BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
CREATE TRIGGER order_items_set_updated_at BEFORE UPDATE ON order_items FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();
CREATE TRIGGER payments_set_updated_at BEFORE UPDATE ON payments FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();

COMMIT;
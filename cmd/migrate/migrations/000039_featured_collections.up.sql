BEGIN;

-- ============================================================
-- FEATURED COLLECTIONS (admin-managed)
-- ============================================================
CREATE TABLE IF NOT EXISTS featured_collections (
  id           BIGSERIAL PRIMARY KEY,
  key          TEXT NOT NULL UNIQUE,      -- stable identifier used by API/apps
  title        TEXT NOT NULL,
  type         TEXT NOT NULL,             -- TEXT instead of enum (easier migrations)
  description  TEXT,
  is_active    BOOLEAN NOT NULL DEFAULT TRUE,
  starts_at    TIMESTAMPTZ,
  ends_at      TIMESTAMPTZ,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
  CHECK (ends_at IS NULL OR starts_at IS NULL OR ends_at > starts_at)
);

CREATE INDEX IF NOT EXISTS idx_featured_collections_active
  ON featured_collections (is_active)
  WHERE is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_featured_collections_time_window
  ON featured_collections (starts_at, ends_at);

-- NOTE: requires fn_set_updated_at() to exist in your DB.
-- If you don't have it, remove trigger lines.
DROP TRIGGER IF EXISTS trg_featured_collections_updated_at ON featured_collections;
CREATE TRIGGER trg_featured_collections_updated_at
BEFORE UPDATE ON featured_collections
FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();


-- ============================================================
-- FEATURED ITEMS (ordered items inside a collection)
-- ============================================================
CREATE TABLE IF NOT EXISTS featured_items (
  id                 BIGSERIAL PRIMARY KEY,
  collection_id      BIGINT NOT NULL REFERENCES featured_collections(id) ON DELETE CASCADE,

  position           INT NOT NULL DEFAULT 0,
  badge_text         TEXT,
  subtitle           TEXT,
  deal_price_cents   BIGINT CHECK (deal_price_cents >= 0),
  deal_percent       INT CHECK (deal_percent >= 0 AND deal_percent <= 100),

  -- Item can reference:
  -- 1) a specific variant (best for controlled pricing)
  -- 2) a product (we pick a default active variant in the MV)
  product_id         BIGINT REFERENCES products(id) ON DELETE RESTRICT,
  product_variant_id BIGINT REFERENCES product_variants(id) ON DELETE RESTRICT,

  is_active          BOOLEAN NOT NULL DEFAULT TRUE,
  starts_at          TIMESTAMPTZ,
  ends_at            TIMESTAMPTZ,

  created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ NOT NULL DEFAULT now(),

  CHECK ((product_id IS NOT NULL) OR (product_variant_id IS NOT NULL)),
  CHECK (ends_at IS NULL OR starts_at IS NULL OR ends_at > starts_at)
);

-- Stable ordering inside a collection (prevents duplicates at same position)
CREATE UNIQUE INDEX IF NOT EXISTS uq_featured_items_collection_position
  ON featured_items (collection_id, position);

CREATE INDEX IF NOT EXISTS idx_featured_items_collection
  ON featured_items (collection_id);

CREATE INDEX IF NOT EXISTS idx_featured_items_active
  ON featured_items (is_active)
  WHERE is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_featured_items_time_window
  ON featured_items (starts_at, ends_at);

CREATE INDEX IF NOT EXISTS idx_featured_items_product
  ON featured_items (product_id)
  WHERE product_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_featured_items_variant
  ON featured_items (product_variant_id)
  WHERE product_variant_id IS NOT NULL;

DROP TRIGGER IF EXISTS trg_featured_items_updated_at ON featured_items;
CREATE TRIGGER trg_featured_items_updated_at
BEFORE UPDATE ON featured_items
FOR EACH ROW EXECUTE FUNCTION fn_set_updated_at();


-- ============================================================
-- Helpful indexes for image lookup
-- These make the LATERAL image selection very fast.
-- ============================================================
CREATE INDEX IF NOT EXISTS idx_product_images_variant_primary_sort
  ON product_images (product_variant_id, is_primary DESC, sort_order ASC, id ASC)
  WHERE product_variant_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_product_images_product_primary_sort
  ON product_images (product_id, is_primary DESC, sort_order ASC, id ASC);


-- ============================================================
-- MATERIALIZED VIEW (read model / cache)
-- Goals:
-- - Simple rows (NO JSON aggregation)
-- - Extremely fast app reads
-- - Deterministic "default variant" if item references product only
-- - Prefer variant image, fallback to product image
-- - Includes pv.is_active = TRUE filter (so you never show inactive variants)
-- ============================================================
DROP MATERIALIZED VIEW IF EXISTS featured_collections_cache;

CREATE MATERIALIZED VIEW featured_collections_cache AS
SELECT
  -- Collections
  fc.key         AS collection_key,
  fc.title       AS collection_title,
  fc.type        AS collection_type,
  fc.description AS collection_description,

  -- Items
  fi.id          AS item_id,
  fi.position,
  fi.badge_text,
  fi.subtitle,
  fi.deal_price_cents,
  fi.deal_percent,

  -- Products (NOTE: product_description intentionally NOT included)
  p.id           AS product_id,
  p.name         AS product_name,
  p.slug         AS product_slug,

  -- Chosen Variant (explicit variant OR default active variant for product)
  v.id           AS variant_id,
  v.price_cents  AS variant_price_cents,

  -- Images (prefer variant image, else product image)
  COALESCE(vimg.url, pimg.url) AS image_url,

  -- Cache metadata
  now() AS cached_at

FROM featured_collections fc
JOIN featured_items fi ON fi.collection_id = fc.id

-- Resolve product:
-- If item references product: p = that product
-- If item references variant: p = variant.product_id (only if variant is active)
LEFT JOIN products p ON p.id = COALESCE(
  fi.product_id,
  (
    SELECT pv.product_id
    FROM product_variants pv
    WHERE pv.id = fi.product_variant_id
      AND pv.is_active = TRUE
    LIMIT 1
  )
)

-- Choose a variant:
-- - If fi.product_variant_id is set: use that variant (must be active)
-- - Else: pick cheapest active variant for the product
LEFT JOIN LATERAL (
  SELECT pv.id, pv.price_cents
  FROM product_variants pv
  WHERE
    (
      (fi.product_variant_id IS NOT NULL AND pv.id = fi.product_variant_id)
      OR
      (fi.product_variant_id IS NULL AND fi.product_id IS NOT NULL AND pv.product_id = fi.product_id)
    )
    AND pv.is_active = TRUE
  ORDER BY
    -- Prefer the explicitly chosen variant first (if provided)
    CASE WHEN pv.id = fi.product_variant_id THEN 0 ELSE 1 END,
    -- Otherwise choose cheapest
    pv.price_cents ASC,
    -- Tie-breaker for deterministic results
    pv.id ASC
  LIMIT 1
) v ON TRUE

-- Variant image (best for chosen variant)
LEFT JOIN LATERAL (
  SELECT url
  FROM product_images
  WHERE product_variant_id = v.id
  ORDER BY is_primary DESC, sort_order ASC, id ASC
  LIMIT 1
) vimg ON TRUE

-- Product image fallback
LEFT JOIN LATERAL (
  SELECT url
  FROM product_images
  WHERE product_id = p.id
  ORDER BY (product_variant_id IS NULL) DESC, is_primary DESC, sort_order ASC, id ASC
  LIMIT 1
) pimg ON TRUE

WHERE
  -- Active checks
  fc.is_active = TRUE
  AND fi.is_active = TRUE
  AND p.is_active = TRUE

  -- Time windows
  AND (fc.starts_at IS NULL OR fc.starts_at <= now())
  AND (fc.ends_at   IS NULL OR fc.ends_at   >  now())
  AND (fi.starts_at IS NULL OR fi.starts_at <= now())
  AND (fi.ends_at   IS NULL OR fi.ends_at   >  now())

  -- Guarantee we always have a chosen active variant (for price)
  AND v.id IS NOT NULL
WITH DATA;

-- ============================================================
-- MV Indexes
-- IMPORTANT:
-- REFRESH MATERIALIZED VIEW CONCURRENTLY requires *a* UNIQUE index.
-- You're choosing (collection_key, item_id) as the unique identity.
-- ============================================================
CREATE UNIQUE INDEX IF NOT EXISTS idx_featured_collections_cache_composite
  ON featured_collections_cache (collection_key, item_id);

CREATE INDEX IF NOT EXISTS idx_featured_collections_cache_collection
  ON featured_collections_cache (collection_key, position);

CREATE INDEX IF NOT EXISTS idx_featured_collections_cache_type
  ON featured_collections_cache (collection_type);

CREATE INDEX IF NOT EXISTS idx_featured_collections_cache_product
  ON featured_collections_cache (product_id);

COMMIT;

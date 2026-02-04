package carts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"khel/internal/infra/dbx"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Repository struct {
	db  dbx.Querier
	ttl time.Duration
}

func NewRepository(q dbx.Querier) *Repository {
	return &Repository{db: q, ttl: 7 * 24 * time.Hour}
}

func NewRepositoryWithTTL(q dbx.Querier, ttl time.Duration) *Repository {
	return &Repository{db: q, ttl: ttl}
}

// --- internal helpers ---

func (r *Repository) bumpTTLByCartID(ctx context.Context, cartID int64) {
	_, _ = r.db.Exec(ctx, `
UPDATE carts
SET expires_at = $2,
    updated_at = now()
WHERE id = $1
  AND status = 'active'
`, cartID, time.Now().Add(r.ttl))
}

// Optional public helper (if you want to call from handlers / cron)
func (r *Repository) BumpTTL(ctx context.Context, userID int64) error {
	_, err := r.db.Exec(ctx, `
UPDATE carts
SET expires_at = $2,
    updated_at = now()
WHERE user_id = $1
  AND status = 'active'
  AND (expires_at IS NULL OR expires_at > now())
`, userID, time.Now().Add(r.ttl))
	return err
}

// GetOrCreateCart returns the user's current cart (active or checkout_pending)
// or creates a new active cart if none exists.
//
// Concurrency + uniqueness notes:
//   - You likely have a UNIQUE (partial) constraint like:
//     uniq_active_or_pending_cart_per_user ON (user_id) WHERE status IN ('active','checkout_pending')
//   - Expired carts MUST NOT remain in status='active' forever, otherwise they block new cart creation.
//   - Under high concurrency, multiple GET /store/cart can race: one inserts, others conflict.
//     Those callers should simply fetch the winning row and return it.
//
// This implementation:
// 1) Select valid (non-expired) open cart.
// 2) If none, attempt insert.
// 3) If conflict, select the BLOCKING open cart (even if expired) and repair if needed.
// 4) Retry once.
func (r *Repository) GetOrCreateCart(ctx context.Context, userID int64) (int64, error) {
	const maxAttempts = 2

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// 1) Fast path: return current NON-expired open cart (same rule as GetView).
		if id, ok, err := r.selectOpenCartNonExpired(ctx, userID); err != nil {
			return 0, err
		} else if ok {
			return id, nil
		}

		// 2) No valid open cart. Try to create a new active cart.
		exp := time.Now().Add(r.ttl)

		var id int64
		err := r.db.QueryRow(ctx, `
INSERT INTO carts (user_id, guest_token, status, expires_at)
VALUES ($1, NULL, 'active', $2)
RETURNING id
`, userID, exp).Scan(&id)

		if err == nil {
			return id, nil
		}

		// 3) If insert failed due to UNIQUE conflict, someone else has an open cart row.
		// Fetch that blocking row WITHOUT expiry filter, then decide what to do.
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			block, berr := r.selectBlockingOpenCartEvenIfExpired(ctx, userID)
			if berr != nil {
				return 0, berr
			}
			if block == nil {
				// Extremely rare: conflict but cannot see row.
				// Usually means READ COMMITTED timing / replication lag / or row got updated between checks.
				// Retry loop once.
				continue
			}

			// If checkout is pending, we should return it (payment flow owns this cart).
			if block.Status == "checkout_pending" {
				// Optional: if it's expired checkout_pending, you can decide to unlock it.
				// BUT be careful: you might break a legitimate ongoing payment verification.
				//
				// Safer approach: just return it, and UI can show "payment in progress".
				return block.ID, nil
			}

			// If it's an ACTIVE cart, but expired, abandon it so it stops blocking unique constraint.
			if block.Status == "active" && block.ExpiresAt != nil && !block.ExpiresAt.After(time.Now()) {
				_, uerr := r.db.Exec(ctx, `
UPDATE carts
SET status='abandoned', updated_at=now()
WHERE id = $1
  AND status='active'
`, block.ID)
				if uerr != nil {
					return 0, fmt.Errorf("abandon expired cart: %w", uerr)
				}

				// Now retry insert on next loop iteration.
				continue
			}

			// Active but not expired (or expires_at is NULL): just return it.
			return block.ID, nil
		}

		// Non-conflict DB error:
		return 0, fmt.Errorf("create cart: %w", err)
	}

	return 0, fmt.Errorf("get or create cart: cart not found after conflict")
}

// --- helpers ---

func (r *Repository) selectOpenCartNonExpired(ctx context.Context, userID int64) (int64, bool, error) {
	var id int64
	err := r.db.QueryRow(ctx, `
SELECT id
FROM carts
WHERE user_id = $1
  AND status IN ('active','checkout_pending')
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY
  CASE status WHEN 'checkout_pending' THEN 1 WHEN 'active' THEN 2 ELSE 3 END,
  updated_at DESC
LIMIT 1
`, userID).Scan(&id)

	if err == nil {
		return id, true, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return 0, false, fmt.Errorf("select open cart: %w", err)
}

type blockingCart struct {
	ID        int64
	Status    string
	ExpiresAt *time.Time
}

// Select the row that is currently blocking the unique constraint.
// NOTE: This query intentionally does NOT filter on expires_at.
func (r *Repository) selectBlockingOpenCartEvenIfExpired(ctx context.Context, userID int64) (*blockingCart, error) {
	var c blockingCart
	err := r.db.QueryRow(ctx, `
SELECT id, status, expires_at
FROM carts
WHERE user_id = $1
  AND status IN ('active','checkout_pending')
ORDER BY updated_at DESC
LIMIT 1
`, userID).Scan(&c.ID, &c.Status, &c.ExpiresAt)

	if err == nil {
		return &c, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return nil, fmt.Errorf("select blocking cart: %w", err)
}

// --- User flows ---

// EnsureActive returns an existing active cart id or creates a new one with TTL.
// It only sets expires_at when creating a cart; it does NOT bump TTL for existing carts.
// if you want checkout_pending state too use GetOrCreateCart method
func (r *Repository) EnsureActive(ctx context.Context, userID int64) (int64, error) {
	var id int64

	err := r.db.QueryRow(ctx, `
SELECT id
FROM carts
WHERE user_id = $1
  AND status = 'active'
  AND (expires_at IS NULL OR expires_at > now())
LIMIT 1
`, userID).Scan(&id)

	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		// real DB error
		return 0, fmt.Errorf("get active cart: %w", err)
	}

	// No active cart → create new with fresh TTL
	exp := time.Now().Add(r.ttl)
	if err := r.db.QueryRow(ctx, `
INSERT INTO carts (user_id, guest_token, status, expires_at)
VALUES ($1, NULL, 'active', $2)
RETURNING id
`, userID, exp).Scan(&id); err != nil {
		return 0, fmt.Errorf("ensure active cart: %w", err)
	}

	return id, nil
}

func (r *Repository) AddItem(ctx context.Context, userID, variantID int64, qty int) error {
	if qty <= 0 {
		return fmt.Errorf("qty must be > 0")
	}

	cartID, err := r.EnsureActive(ctx, userID)
	if err != nil {
		return err
	}

	const q = `
WITH pv AS (
  SELECT price_cents
  FROM product_variants
  WHERE id = $1 AND is_active = true
)
INSERT INTO cart_items (cart_id, product_variant_id, quantity, price_cents)
SELECT $2, $1, $3, pv.price_cents
FROM pv
ON CONFLICT (cart_id, product_variant_id)
DO UPDATE SET
  quantity    = cart_items.quantity + EXCLUDED.quantity,
  price_cents = EXCLUDED.price_cents,
  updated_at  = now();
`

	tag, err := r.db.Exec(ctx, q, variantID, cartID, qty)
	if err != nil {
		return fmt.Errorf("add item: %w", err)
	}

	// If pv CTE returned no rows (variant not found or inactive), INSERT will affect 0 rows
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("variant not found or inactive")
	}

	// Successful mutation → bump TTL
	r.bumpTTLByCartID(ctx, cartID)
	return nil
}

func (r *Repository) UpdateItemQty(ctx context.Context, userID, itemID int64, qty int) error {
	if qty <= 0 {
		return fmt.Errorf("qty must be > 0")
	}

	var cartID int64

	err := r.db.QueryRow(ctx, `
UPDATE cart_items ci
SET quantity = $3,
    updated_at = now()
WHERE ci.id = $2
  AND ci.cart_id = (
    SELECT id
    FROM carts
    WHERE user_id = $1
      AND status = 'active'
      AND (expires_at IS NULL OR expires_at > now())
    LIMIT 1
  )
RETURNING ci.cart_id
`, userID, itemID, qty).Scan(&cartID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("item not found")
		}
		return fmt.Errorf("update qty: %w", err)
	}

	r.bumpTTLByCartID(ctx, cartID)
	return nil
}

func (r *Repository) RemoveItem(ctx context.Context, userID, itemID int64) error {
	var cartID int64

	err := r.db.QueryRow(ctx, `
DELETE FROM cart_items
WHERE id = $2
  AND cart_id = (
    SELECT id
    FROM carts
    WHERE user_id = $1
      AND status = 'active'
      AND (expires_at IS NULL OR expires_at > now())
    LIMIT 1
  )
RETURNING cart_id
`, userID, itemID).Scan(&cartID)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return fmt.Errorf("item not found")
		}
		return fmt.Errorf("remove item: %w", err)
	}

	// You *can* bump TTL here too — user is actively managing their cart
	r.bumpTTLByCartID(ctx, cartID)
	return nil
}

func (r *Repository) Clear(ctx context.Context, userID int64) error {
	_, err := r.db.Exec(ctx, `
DELETE FROM cart_items
WHERE cart_id = (
  SELECT id
  FROM carts
  WHERE user_id = $1
    AND status = 'active'
    AND (expires_at IS NULL OR expires_at > now())
  LIMIT 1
)`, userID)
	return err
}

// UnlockCheckoutCart re-opens a cart when online payment fails/cancels.
// Safe to call multiple times (idempotent).
func (r *Repository) UnlockCheckoutCart(ctx context.Context, orderID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE carts
		   SET status='active', checkout_order_id=NULL, updated_at=now()
		 WHERE checkout_order_id=$1 AND status='checkout_pending'
	`, orderID)
	return err
}

// ConvertCheckoutCart finalizes the cart used for checkout AFTER payment is confirmed.
//
// We only convert carts that are explicitly linked to the order via checkout_order_id
// AND currently in 'checkout_pending'. This prevents converting a wrong cart due to bugs
// or race conditions. remember db constraint if converted then checkout_order_id=null
func (r *Repository) ConvertCheckoutCart(ctx context.Context, orderID int64) error {
	_, err := r.db.Exec(ctx, `
		UPDATE carts
		   SET status='converted',
		       checkout_order_id=NULL,
		       updated_at=now()
		 WHERE checkout_order_id=$1
		   AND status='checkout_pending'
	`, orderID)
	return err
}

// Get active cart or checkout_pending view by user
func (r *Repository) GetView(ctx context.Context, userID int64) (*CartView, error) {
	var v CartView

	err := r.db.QueryRow(ctx, `
SELECT id, user_id, guest_token, status, expires_at, created_at, updated_at, checkout_order_id
FROM carts
WHERE user_id = $1
  AND (status = 'active' OR status = 'checkout_pending')
  AND (expires_at IS NULL OR expires_at > now())
ORDER BY 
    CASE status 
        WHEN 'checkout_pending' THEN 1 
        WHEN 'active' THEN 2 
        ELSE 3 
    END,
    updated_at DESC
LIMIT 1
`, userID).Scan(
		&v.Cart.ID,
		&v.Cart.UserID,
		&v.Cart.GuestToken,
		&v.Cart.Status,
		&v.Cart.ExpiresAt,
		&v.Cart.CreatedAt,
		&v.Cart.UpdatedAt,
		&v.Cart.CheckoutOrderID,
	)

	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("get cart: %w", err)
		}
		return nil, nil
	}

	return r.fillLines(ctx, &v, v.Cart.ID)
}

// Get view by cartID (admin or internal)
func (r *Repository) GetViewByCartID(ctx context.Context, cartID int64) (*CartView, error) {
	var v CartView

	if err := r.db.QueryRow(ctx, `
SELECT id, user_id, guest_token, status, expires_at, created_at, updated_at, checkout_order_id
FROM carts
WHERE id = $1
`, cartID).Scan(
		&v.Cart.ID,
		&v.Cart.UserID,
		&v.Cart.GuestToken,
		&v.Cart.Status,
		&v.Cart.ExpiresAt,
		&v.Cart.CreatedAt,
		&v.Cart.UpdatedAt,
		&v.Cart.CheckoutOrderID,
	); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("cart not found")
		}
		return nil, fmt.Errorf("get cart by id: %w", err)
	}

	return r.fillLines(ctx, &v, cartID)
}

// fillLines fetches cart items for any cartID and returns a PRICED CartView.
// This makes checkout UI accurate (shows discount + total that matches order snapshot).
func (r *Repository) fillLines(ctx context.Context, v *CartView, cartID int64) (*CartView, error) {
	rows, err := r.db.Query(ctx, `
WITH cart_lines AS (
  SELECT
    ci.id                AS item_id,
    ci.quantity          AS quantity,
    p.id                 AS product_id,
    pv.id                AS variant_id,
    p.name               AS product_name,
    pv.attributes        AS variant_attributes,
    pv.price_cents       AS list_unit_price_cents,
    (
      SELECT url
      FROM product_images 
      WHERE product_id = p.id
        AND is_primary = true 
      ORDER BY created_at ASC
      LIMIT 1
    ) AS primary_image_url
  FROM cart_items ci
  JOIN product_variants pv ON pv.id = ci.product_variant_id
  JOIN products p         ON p.id  = pv.product_id
  WHERE ci.cart_id = $1
),
eligible_deals AS (
  SELECT
    cl.item_id,
    NULLIF(fi.deal_price_cents, 0) AS deal_price_cents,  -- ✅ 0 means "unset"
    fi.deal_percent,
    fi.badge_text
  FROM cart_lines cl
  JOIN featured_items fi
    ON fi.is_active = true
   AND (fi.starts_at IS NULL OR fi.starts_at <= now())
   AND (fi.ends_at   IS NULL OR fi.ends_at   >= now())
   AND (
        (fi.product_variant_id IS NOT NULL AND fi.product_variant_id = cl.variant_id)
     OR (fi.product_id IS NOT NULL AND fi.product_id = cl.product_id)
   )
  JOIN featured_collections fc
    ON fc.id = fi.collection_id
   AND fc.is_active = true
   AND (fc.starts_at IS NULL OR fc.starts_at <= now())
   AND (fc.ends_at   IS NULL OR fc.ends_at   >= now())
),

best_deal AS (
  SELECT DISTINCT ON (ed.item_id)
    ed.item_id,
    ed.deal_price_cents,
    ed.deal_percent,
    ed.badge_text
  FROM eligible_deals ed
  ORDER BY
    ed.item_id,
    (ed.deal_price_cents IS NULL) ASC,  -- ✅ real price deals first
    ed.deal_price_cents ASC NULLS LAST, -- ✅ lower deal price wins
    ed.deal_percent DESC NULLS LAST     -- ✅ otherwise higher percent wins
),

priced AS (
  SELECT
    cl.*,
    CASE
      -- ✅ Fixed-price deal wins only if valid
      WHEN bd.deal_price_cents IS NOT NULL
           AND bd.deal_price_cents > 0
           AND bd.deal_price_cents < cl.list_unit_price_cents
        THEN bd.deal_price_cents

      -- ✅ Percent deal (guard against 0/100/null)
      WHEN bd.deal_percent IS NOT NULL
           AND bd.deal_percent > 0
           AND bd.deal_percent < 100
        THEN (cl.list_unit_price_cents * (100 - bd.deal_percent) / 100)

      -- ✅ No deal
      ELSE cl.list_unit_price_cents
    END AS final_unit_price_cents,

    bd.badge_text
  FROM cart_lines cl
  LEFT JOIN best_deal bd ON bd.item_id = cl.item_id
)
SELECT
  item_id,
  product_id,
  variant_id,
  product_name,
  variant_attributes,
  quantity,
  list_unit_price_cents,
  final_unit_price_cents,
  (quantity * final_unit_price_cents) AS line_total_cents,
 (quantity * GREATEST(list_unit_price_cents - final_unit_price_cents, 0)) AS line_discount_cents,
  primary_image_url,
  badge_text
FROM priced
ORDER BY item_id ASC;
`, cartID)
	if err != nil {
		return nil, fmt.Errorf("cart lines priced: %w", err)
	}
	defer rows.Close()

	// Reset totals (important if v is reused)
	v.Items = nil
	v.SubtotalCents = 0
	v.DiscountCents = 0
	v.TotalCents = 0

	for rows.Next() {
		var line CartLine
		var attrs []byte
		var url *string
		var badge *string

		if err := rows.Scan(
			&line.ItemID,
			&line.ProductID,
			&line.VariantID,
			&line.ProductName,
			&attrs,
			&line.Quantity,
			&line.ListUnitPriceCents,
			&line.FinalUnitPriceCents,
			&line.LineTotalCents,
			&line.LineDiscountCents,
			&url,
			&badge,
		); err != nil {
			return nil, fmt.Errorf("scan priced line: %w", err)
		}

		_ = json.Unmarshal(attrs, &line.VariantAttrs)
		line.PrimaryImageURL = url
		line.BadgeText = badge

		// ✅ accumulate totals for checkout UI
		v.SubtotalCents += int64(line.Quantity) * line.ListUnitPriceCents
		v.DiscountCents += line.LineDiscountCents
		v.TotalCents += line.LineTotalCents

		v.Items = append(v.Items, line)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("priced lines rows error: %w", err)
	}

	return v, nil
}

// Admin housekeeping: mark expired as abandoned
func (r *Repository) MarkExpiredAsAbandoned(ctx context.Context) (int64, error) {
	tag, err := r.db.Exec(ctx, `
UPDATE carts
SET status = 'abandoned',
    updated_at = now()
WHERE status = 'active'
  AND expires_at IS NOT NULL
  AND expires_at <= now()
`)
	if err != nil {
		return 0, fmt.Errorf("mark abandoned: %w", err)
	}
	return tag.RowsAffected(), nil
}

// List returns carts for admin with optional status filter and expiry inclusion.
func (r *Repository) List(ctx context.Context, status string, includeExpired bool, limit, offset int) ([]Cart, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	if offset < 0 {
		offset = 0
	}

	where := "1=1"
	args := []any{}
	arg := 1

	if status != "" {
		where += fmt.Sprintf(" AND status = $%d", arg)
		args = append(args, status)
		arg++
	}
	if !includeExpired {
		where += " AND (expires_at IS NULL OR expires_at > now())"
	}

	q := fmt.Sprintf(`
SELECT id, user_id, guest_token, status, expires_at, created_at, updated_at,
       COUNT(*) OVER() AS total
FROM carts
WHERE %s
ORDER BY id DESC
LIMIT $%d OFFSET $%d
`, where, arg, arg+1)

	args = append(args, limit, offset)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list carts: %w", err)
	}
	defer rows.Close()

	var out []Cart
	total := 0

	for rows.Next() {
		var c Cart
		var t int
		if err := rows.Scan(
			&c.ID,
			&c.UserID,
			&c.GuestToken,
			&c.Status,
			&c.ExpiresAt,
			&c.CreatedAt,
			&c.UpdatedAt,
			&t,
		); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		if total == 0 {
			total = t
		}
		out = append(out, c)
	}

	return out, total, nil
}

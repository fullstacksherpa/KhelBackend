package orders

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"khel/internal/infra/dbx"
)

type Repository struct {
	q   dbx.Querier
	gen *OrderNumberGenerator
}

func NewRepository(q dbx.Querier, gen *OrderNumberGenerator) *Repository {
	if gen == nil {
		panic("orders: OrderNumberGenerator is nil")
	}
	return &Repository{
		q:   q,
		gen: gen,
	}
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*Order, error) {
	var o Order
	err := r.q.QueryRow(ctx, `
SELECT id,user_id,order_number,status,payment_status,payment_method,paid_at,
       subtotal_cents,discount_cents,tax_cents,shipping_cents,total_cents,created_at
FROM orders WHERE id=$1`, id).
		Scan(&o.ID, &o.UserID, &o.OrderNumber, &o.Status, &o.PaymentStatus, &o.PaymentMethod, &o.PaidAt,
			&o.SubtotalCents, &o.DiscountCents, &o.TaxCents, &o.ShippingCents, &o.TotalCents, &o.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// CreateFromCart creates an order snapshot from the user's ACTIVE cart.
//
// IMPORTANT BEHAVIOR (no migrations needed):
// - Discounts are computed at CHECKOUT time by joining cart lines to featured_items/featured_collections.
// - The result is SNAPSHOTTED into:
//   - orders.subtotal_cents / orders.discount_cents / orders.total_cents
//   - order_items.unit_price_cents (final unit price after discount)
//
// This ensures the payment gateway amount matches what the user is intended to pay.
//
// Assumes this is called INSIDE a transaction.
func (r *Repository) CreateFromCart(
	ctx context.Context,
	userID int64,
	ship ShippingInfo,
	method string, // normalized before calling: "khalti" | "esewa" | "cash_on_delivery"
) (*Order, int64 /*cartID*/, error) {

	// 1) Lock the active cart row (prevents two concurrent checkouts creating two orders).
	//    This acts like a per-user checkout mutex (when you enforce one active cart per user).
	var cartID int64
	err := r.q.QueryRow(ctx, `
		SELECT id
		FROM carts
		WHERE user_id=$1 AND status='active'
		ORDER BY id DESC
		LIMIT 1
		FOR UPDATE
	`, userID).Scan(&cartID)
	if err != nil {
		return nil, 0, fmt.Errorf("no active cart: %w", err)
	}

	// 2) Compute pricing snapshot from cart_items + featured deals.
	//
	// We compute three numbers:
	// - subtotal_cents  = sum(list_price * qty)
	// - discount_cents  = sum((list_price - final_price) * qty)
	// - total_cents     = subtotal_cents - discount_cents  (tax/shipping can be added later)
	//
	// "Best deal" logic:
	// - Eligible deals are active + within starts_at/ends_at for BOTH item and collection.
	// - A deal can target product_variant_id OR product_id.
	// - If multiple deals match a line, we pick the "best" one:
	//     1) lowest deal_price_cents (if present)
	//     2) otherwise highest deal_percent
	var subtotal, discount, total int64
	if err := r.q.QueryRow(ctx, `
WITH cart_lines AS (
  SELECT
    ci.id          AS cart_item_id,
    ci.quantity    AS quantity,
    pv.id          AS variant_id,
    p.id           AS product_id,
    pv.price_cents AS list_unit_price_cents
  FROM cart_items ci
  JOIN product_variants pv ON pv.id = ci.product_variant_id
  JOIN products p          ON p.id  = pv.product_id
  WHERE ci.cart_id = $1
),
eligible_deals AS (
  SELECT
    cl.cart_item_id,
    NULLIF(fi.deal_price_cents, 0) AS deal_price_cents, -- ✅ 0 means "not set"
    fi.deal_percent
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
  SELECT DISTINCT ON (ed.cart_item_id)
    ed.cart_item_id,
    ed.deal_price_cents,
    ed.deal_percent
  FROM eligible_deals ed
  ORDER BY
    ed.cart_item_id,
    (ed.deal_price_cents IS NULL) ASC,           -- ✅ fixed-price beats percent
    ed.deal_price_cents ASC NULLS LAST,          -- ✅ lower fixed price wins
    ed.deal_percent DESC NULLS LAST              -- ✅ otherwise higher percent wins
),
priced AS (
  SELECT
    cl.cart_item_id,
    cl.quantity,
    cl.variant_id,
    cl.product_id,
    cl.list_unit_price_cents,

    -- ✅ Final unit price with hard guards
    CASE
      -- Fixed-price deal wins only if it's a real discount
      WHEN bd.deal_price_cents IS NOT NULL
           AND bd.deal_price_cents > 0
           AND bd.deal_price_cents < cl.list_unit_price_cents
        THEN bd.deal_price_cents

      -- Percent deal (ignore 0/100 and bad data)
      WHEN bd.deal_percent IS NOT NULL
           AND bd.deal_percent > 0
           AND bd.deal_percent < 100
        THEN (cl.list_unit_price_cents * (100 - bd.deal_percent) / 100)

      -- No deal
      ELSE cl.list_unit_price_cents
    END AS final_unit_price_cents

  FROM cart_lines cl
  LEFT JOIN best_deal bd ON bd.cart_item_id = cl.cart_item_id
)
SELECT
  COALESCE(SUM(quantity * list_unit_price_cents), 0) AS subtotal_cents,

  -- ✅ never let discount go negative
  COALESCE(SUM(quantity * GREATEST(list_unit_price_cents - final_unit_price_cents, 0)), 0) AS discount_cents,

  COALESCE(SUM(quantity * final_unit_price_cents), 0) AS total_cents
FROM priced;
	`, cartID).Scan(&subtotal, &discount, &total); err != nil {
		return nil, 0, fmt.Errorf("pricing from cart + featured deals: %w", err)
	}

	if subtotal <= 0 {
		return nil, 0, fmt.Errorf("cart is empty")
	}
	if discount < 0 || discount > subtotal {
		return nil, 0, fmt.Errorf("invalid discount computed")
	}
	if total < 0 {
		return nil, 0, fmt.Errorf("invalid total computed")
	}

	// 3) Build the order snapshot values (immutable once created).
	o := &Order{
		UserID:        userID,
		OrderNumber:   r.gen.Generate(userID),
		SubtotalCents: subtotal,
		DiscountCents: discount,
		TaxCents:      0,
		ShippingCents: 0,
		TotalCents:    total,
	}

	// Choose order status based on payment method.
	// - COD: can move to 'processing' immediately (or 'pending' if you want manual confirm).
	// - Online: should be 'awaiting_payment', payment_status 'pending'.
	orderStatus := "processing"
	paymentStatus := "pending"
	if method != "cash_on_delivery" {
		orderStatus = "awaiting_payment"
		paymentStatus = "pending"
	}

	// 4) Create order row with computed totals.
	if err := r.q.QueryRow(ctx, `
		INSERT INTO orders (
		  user_id, order_number, cart_id, status, payment_status, payment_method,
		  shipping_name, shipping_phone, shipping_address, shipping_city, shipping_postal_code, shipping_country,
		  subtotal_cents, discount_cents, tax_cents, shipping_cents, total_cents
		) VALUES (
		  $1, $2, $3, $4::order_status, $5::payment_status, $6,
		  $7, $8, $9, $10, $11, COALESCE($12,'Nepal'),
		  $13, $14, $15, $16, $17
		)
		RETURNING id, created_at
	`,
		userID, o.OrderNumber, cartID, orderStatus, paymentStatus, method,
		ship.Name, ship.Phone, ship.Address, ship.City, ship.PostalCode, ship.Country,
		o.SubtotalCents, o.DiscountCents, o.TaxCents, o.ShippingCents, o.TotalCents,
	).Scan(&o.ID, &o.CreatedAt); err != nil {
		return nil, 0, fmt.Errorf("create order: %w", err)
	}

	// 5) Copy order_items snapshot using the FINAL (discounted) unit price.
	//    This is the critical fix: don't copy ci.price_cents (it doesn't know discounts).
	if _, err := r.q.Exec(ctx, `
WITH cart_lines AS (
  SELECT
    ci.id          AS cart_item_id,
    ci.quantity    AS quantity,
    pv.id          AS variant_id,
    p.id           AS product_id,
    p.name         AS product_name,
    pv.attributes  AS variant_attributes,
    pv.price_cents AS list_unit_price_cents
  FROM cart_items ci
  JOIN product_variants pv ON pv.id = ci.product_variant_id
  JOIN products p          ON p.id  = pv.product_id
  WHERE ci.cart_id = $1
),
eligible_deals AS (
  SELECT
    cl.cart_item_id,
    NULLIF(fi.deal_price_cents, 0) AS deal_price_cents, -- ✅
    fi.deal_percent
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
  SELECT DISTINCT ON (ed.cart_item_id)
    ed.cart_item_id,
    ed.deal_price_cents,
    ed.deal_percent
  FROM eligible_deals ed
  ORDER BY
    ed.cart_item_id,
    (ed.deal_price_cents IS NULL) ASC,
    ed.deal_price_cents ASC NULLS LAST,
    ed.deal_percent DESC NULLS LAST
),
priced AS (
  SELECT
    cl.product_id,
    cl.variant_id,
    cl.product_name,
    cl.variant_attributes,
    cl.quantity,

    CASE
      WHEN bd.deal_price_cents IS NOT NULL
           AND bd.deal_price_cents > 0
           AND bd.deal_price_cents < cl.list_unit_price_cents
        THEN bd.deal_price_cents

      WHEN bd.deal_percent IS NOT NULL
           AND bd.deal_percent > 0
           AND bd.deal_percent < 100
        THEN (cl.list_unit_price_cents * (100 - bd.deal_percent) / 100)

      ELSE cl.list_unit_price_cents
    END AS final_unit_price_cents

  FROM cart_lines cl
  LEFT JOIN best_deal bd ON bd.cart_item_id = cl.cart_item_id
)
INSERT INTO order_items (
  order_id, product_id, product_variant_id, product_name, variant_attributes,
  quantity, unit_price_cents, total_price_cents
)
SELECT
  $2,
  product_id,
  variant_id,
  product_name,
  variant_attributes,
  quantity,
  final_unit_price_cents,
  quantity * final_unit_price_cents
FROM priced;
	`, cartID, o.ID); err != nil {
		return nil, 0, fmt.Errorf("copy order_items (priced): %w", err)
	}

	// 6) Cart state transition:
	//    - Online payment: lock cart (checkout_pending) so items can't be mutated mid-payment.
	//    - COD: convert immediately (cart is now finalized).
	if method != "cash_on_delivery" {
		cmd, err := r.q.Exec(ctx, `
UPDATE carts
   SET status='checkout_pending',
       checkout_order_id=$2,
       updated_at=now()
 WHERE id=$1
   AND status='active'::cart_status
`, cartID, o.ID)
		if err != nil {
			return nil, 0, fmt.Errorf("lock cart for payment: %w", err)
		}
		if cmd.RowsAffected() == 0 {
			return nil, 0, fmt.Errorf("cart not active (cannot lock)")
		}
	} else {
		cmd, err := r.q.Exec(ctx, `
UPDATE carts
   SET status='converted',
       checkout_order_id=NULL,
       updated_at=now()
 WHERE id=$1
   AND status='active'::cart_status
`, cartID)
		if err != nil {
			return nil, 0, fmt.Errorf("convert cart: %w", err)
		}
		if cmd.RowsAffected() == 0 {
			return nil, 0, fmt.Errorf("cart not active (cannot convert)")
		}
	}

	return o, cartID, nil
}

func (r *Repository) ListByUser(
	ctx context.Context,
	userID int64,
	status string,
	limit, offset int,
) ([]Order, int, error) {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	// If status is empty string => no filter
	rows, err := r.q.Query(ctx, `
SELECT id,user_id,order_number,status,payment_status,payment_method,paid_at,
       subtotal_cents,discount_cents,tax_cents,shipping_cents,total_cents,created_at,
       COUNT(*) OVER() AS total_count
FROM orders
WHERE user_id = $1
  AND ($2 = '' OR status::text = $2)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4`,
		userID, status, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list orders: %w", err)
	}
	defer rows.Close()

	var (
		out   []Order
		total int
	)

	for rows.Next() {
		var o Order
		var t int
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.OrderNumber, &o.Status, &o.PaymentStatus, &o.PaymentMethod, &o.PaidAt,
			&o.SubtotalCents, &o.DiscountCents, &o.TaxCents, &o.ShippingCents, &o.TotalCents, &o.CreatedAt,
			&t,
		); err != nil {
			return nil, 0, fmt.Errorf("scan order: %w", err)
		}

		if total == 0 {
			total = t
		}
		out = append(out, o)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return out, total, nil
}

func (r *Repository) GetDetailForUser(ctx context.Context, userID, orderID int64) (*OrderDetail, error) {
	var o Order
	err := r.q.QueryRow(ctx, `
SELECT id,user_id,order_number,status,payment_status,payment_method,paid_at,
       subtotal_cents,discount_cents,tax_cents,shipping_cents,total_cents,created_at
FROM orders
WHERE id=$1 AND user_id=$2`,
		orderID, userID,
	).Scan(
		&o.ID, &o.UserID, &o.OrderNumber, &o.Status, &o.PaymentStatus, &o.PaymentMethod, &o.PaidAt,
		&o.SubtotalCents, &o.DiscountCents, &o.TaxCents, &o.ShippingCents, &o.TotalCents, &o.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("order not found")
	}

	rows, err := r.q.Query(ctx, `
SELECT id, order_id, product_id, product_variant_id, product_name,
       variant_attributes, quantity, unit_price_cents, total_price_cents
FROM order_items
WHERE order_id=$1
ORDER BY id ASC`,
		orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("order items: %w", err)
	}
	defer rows.Close()

	var items []OrderItem
	for rows.Next() {
		var it OrderItem
		var attrs []byte
		if err := rows.Scan(
			&it.ID, &it.OrderID, &it.ProductID, &it.ProductVariantID, &it.ProductName,
			&attrs, &it.Quantity, &it.UnitPriceCents, &it.TotalPriceCents,
		); err != nil {
			return nil, fmt.Errorf("scan order item: %w", err)
		}
		_ = json.Unmarshal(attrs, &it.VariantAttrs)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return &OrderDetail{
		Order: o,
		Items: items,
	}, nil
}

func (r *Repository) loadItems(ctx context.Context, orderID int64) ([]OrderItem, error) {
	rows, err := r.q.Query(ctx, `
SELECT id, order_id, product_id, product_variant_id, product_name,
       variant_attributes, quantity, unit_price_cents, total_price_cents
FROM order_items
WHERE order_id=$1
ORDER BY id ASC`, orderID)
	if err != nil {
		return nil, fmt.Errorf("order items: %w", err)
	}
	defer rows.Close()

	var items []OrderItem
	for rows.Next() {
		var it OrderItem
		var attrs []byte
		if err := rows.Scan(
			&it.ID, &it.OrderID, &it.ProductID, &it.ProductVariantID, &it.ProductName,
			&attrs, &it.Quantity, &it.UnitPriceCents, &it.TotalPriceCents,
		); err != nil {
			return nil, fmt.Errorf("scan order item: %w", err)
		}
		_ = json.Unmarshal(attrs, &it.VariantAttrs)
		items = append(items, it)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

// ListAll: admin – optional filter by status, with pagination,default limit is 30
func (r *Repository) ListAll(ctx context.Context, status string, limit, offset int) ([]Order, int, error) {
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

	q := fmt.Sprintf(`
SELECT id,user_id,order_number,status,payment_status,payment_method,paid_at,
       subtotal_cents,discount_cents,tax_cents,shipping_cents,total_cents,created_at,
       COUNT(*) OVER() AS total_count
FROM orders
WHERE %s
ORDER BY created_at DESC
LIMIT $%d OFFSET $%d`, where, arg, arg+1)

	args = append(args, limit, offset)

	rows, err := r.q.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("admin list orders: %w", err)
	}
	defer rows.Close()

	var (
		out   []Order
		total int
	)
	for rows.Next() {
		var o Order
		var t int
		if err := rows.Scan(
			&o.ID, &o.UserID, &o.OrderNumber, &o.Status, &o.PaymentStatus, &o.PaymentMethod, &o.PaidAt,
			&o.SubtotalCents, &o.DiscountCents, &o.TaxCents, &o.ShippingCents, &o.TotalCents, &o.CreatedAt,
			&t,
		); err != nil {
			return nil, 0, fmt.Errorf("scan admin order: %w", err)
		}
		if total == 0 {
			total = t
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, nil
}

func (r *Repository) GetDetail(ctx context.Context, orderID int64) (*OrderDetail, error) {
	var o Order

	err := r.q.QueryRow(ctx, `
SELECT id,user_id,order_number,status,payment_status,payment_method,paid_at,
       subtotal_cents,discount_cents,tax_cents,shipping_cents,total_cents,created_at
FROM orders
WHERE id=$1
`, orderID).Scan(
		&o.ID, &o.UserID, &o.OrderNumber, &o.Status, &o.PaymentStatus, &o.PaymentMethod, &o.PaidAt,
		&o.SubtotalCents, &o.DiscountCents, &o.TaxCents, &o.ShippingCents, &o.TotalCents, &o.CreatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("order not found")
		}
		return nil, fmt.Errorf("get order detail: %w", err)
	}

	items, err := r.loadItems(ctx, orderID)
	if err != nil {
		return nil, fmt.Errorf("load order items: %w", err)
	}

	return &OrderDetail{Order: o, Items: items}, nil
}

type UpdateStatusOpts struct {
	CancelledReason *string
	Note            *string // optional future: status history note
}

func (r *Repository) UpdateStatus(ctx context.Context, orderID int64, status string, opts UpdateStatusOpts) error {
	_, err := r.q.Exec(ctx, `
UPDATE orders
SET status = $2::order_status,
    cancelled_reason = CASE WHEN $2 = 'cancelled' THEN $3 ELSE NULL END,
    cancelled_at     = CASE WHEN $2 = 'cancelled' THEN now() ELSE NULL END,
    updated_at       = now()
WHERE id = $1`,
		orderID, status, opts.CancelledReason,
	)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	return nil
}

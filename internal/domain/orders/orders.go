package orders

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"khel/internal/infra/dbx"
	"time"

	"github.com/jackc/pgx/v5"
)

type Repository struct{ q dbx.Querier }

func NewRepository(q dbx.Querier) *Repository { return &Repository{q: q} }

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

// CreateFromCart copies items from user's active cart into order+order_items,
// marks cart converted. Assumes this is called INSIDE a transaction.
func (r *Repository) CreateFromCart(ctx context.Context, userID int64, ship ShippingInfo, method *string) (*Order, error) {
	// compute totals from cart_items
	var subtotal int64
	if err := r.q.QueryRow(ctx, `
SELECT COALESCE(SUM(ci.quantity * ci.price_cents),0)
FROM cart_items ci
JOIN carts c ON c.id=ci.cart_id
WHERE c.user_id=$1 AND c.status='active'`, userID).Scan(&subtotal); err != nil {
		return nil, fmt.Errorf("subtotal from cart: %w", err)
	}
	if subtotal <= 0 {
		return nil, fmt.Errorf("cart is empty")
	}

	var cartID int64
	if err := r.q.QueryRow(ctx, `SELECT id FROM carts WHERE user_id=$1 AND status='active' LIMIT 1`, userID).Scan(&cartID); err != nil {
		return nil, fmt.Errorf("no active cart: %w", err)
	}

	// create order
	var o Order
	o.UserID = userID
	o.OrderNumber = fmt.Sprintf("KHEL-%d-%d", userID, time.Now().Unix())
	o.SubtotalCents = subtotal
	o.DiscountCents = 0
	o.TaxCents = 0
	o.ShippingCents = 0
	o.TotalCents = subtotal

	err := r.q.QueryRow(ctx, `
INSERT INTO orders (
  user_id, order_number, cart_id, status, payment_status, payment_method,
  shipping_name, shipping_phone, shipping_address, shipping_city, shipping_postal_code, shipping_country,
  subtotal_cents, discount_cents, tax_cents, shipping_cents, total_cents
) VALUES (
  $1, $2, $3, 'pending', 'pending', $4,
  $5, $6, $7, $8, $9, COALESCE($10,'Nepal'),
  $11, $12, $13, $14, $15
) RETURNING id, created_at`,
		userID, o.OrderNumber, cartID, method,
		ship.Name, ship.Phone, ship.Address, ship.City, ship.PostalCode, ship.Country,
		o.SubtotalCents, o.DiscountCents, o.TaxCents, o.ShippingCents, o.TotalCents).
		Scan(&o.ID, &o.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create order: %w", err)
	}

	// copy order_items from cart_items snapshot
	_, err = r.q.Exec(ctx, `
INSERT INTO order_items (
  order_id, product_id, product_variant_id, product_name, variant_attributes,
  quantity, unit_price_cents, total_price_cents
)
SELECT
  $1,
  p.id, pv.id, p.name, pv.attributes,
  ci.quantity, ci.price_cents, ci.quantity*ci.price_cents
FROM cart_items ci
JOIN product_variants pv ON pv.id = ci.product_variant_id
JOIN products p ON p.id = pv.product_id
WHERE ci.cart_id = $2
`, o.ID, cartID)
	if err != nil {
		return nil, fmt.Errorf("copy order_items: %w", err)
	}

	// mark cart converted
	if _, err := r.q.Exec(ctx, `UPDATE carts SET status='converted', updated_at=now() WHERE id=$1`, cartID); err != nil {
		return nil, fmt.Errorf("convert cart: %w", err)
	}

	return &o, nil
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
  AND ($2 = '' OR status = $2)
ORDER BY created_at DESC
LIMIT $3 OFFSET $4`,
		userID, status, limit, offset, pgx.QueryExecModeSimpleProtocol,
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

func (r *Repository) UpdateStatus(ctx context.Context, orderID int64, status string, cancelledReason *string) error {
	// You can validate status here or let DB enum reject invalid values.

	_, err := r.q.Exec(ctx, `
UPDATE orders
SET status = $2,
    cancelled_reason = CASE WHEN $2 = 'cancelled' THEN $3 ELSE NULL END,
    cancelled_at     = CASE WHEN $2 = 'cancelled' THEN now() ELSE NULL END,
    updated_at       = now()
WHERE id = $1`,
		orderID, status, cancelledReason,
	)
	if err != nil {
		return fmt.Errorf("update order status: %w", err)
	}
	return nil
}

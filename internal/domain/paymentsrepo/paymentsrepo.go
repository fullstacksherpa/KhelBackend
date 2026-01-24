// paymentsrepo/repository.go
package paymentsrepo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"khel/internal/infra/dbx"

	"github.com/jackc/pgx/v5"
)

type Repository struct{ q dbx.Querier }

func NewRepository(q dbx.Querier) *Repository { return &Repository{q: q} }

func (r *Repository) Create(ctx context.Context, p *Payment) (*Payment, error) {
	if err := r.q.QueryRow(ctx, `
		INSERT INTO payments (order_id, provider, amount_cents, currency, status)
		VALUES (
			$1,
			$2,
			$3,
			COALESCE($4,'NPR'),
			COALESCE($5,'pending')::payment_status
		)
		RETURNING id, created_at, updated_at
	`, p.OrderID, p.Provider, p.AmountCents, p.Currency, p.Status).
		Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}
	return p, nil
}

func (r *Repository) GetByID(ctx context.Context, id int64) (*Payment, error) {
	var p Payment
	err := r.q.QueryRow(ctx, `
		SELECT id, order_id, provider, provider_ref, amount_cents, currency, status,
		       gateway_response, created_at, updated_at
		FROM payments WHERE id=$1
	`, id).Scan(
		&p.ID, &p.OrderID, &p.Provider, &p.ProviderRef, &p.AmountCents, &p.Currency, &p.Status,
		&p.GatewayResp, &p.CreatedAt, &p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get payment: %w", err)
	}
	return &p, nil
}

func (r *Repository) GetByOrderID(ctx context.Context, orderID int64) ([]*Payment, error) {
	rows, err := r.q.Query(ctx, `
		SELECT id, order_id, provider, provider_ref, amount_cents, currency, status,
		       gateway_response, created_at, updated_at
		FROM payments WHERE order_id=$1 ORDER BY id ASC
	`, orderID)
	if err != nil {
		return nil, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()

	var out []*Payment
	for rows.Next() {
		var p Payment
		if err := rows.Scan(
			&p.ID, &p.OrderID, &p.Provider, &p.ProviderRef, &p.AmountCents, &p.Currency, &p.Status,
			&p.GatewayResp, &p.CreatedAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan payment: %w", err)
		}
		out = append(out, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *Repository) SetPrimaryToOrder(ctx context.Context, orderID, paymentID int64) error {
	_, err := r.q.Exec(ctx, `
		UPDATE orders SET primary_payment_id=$2, updated_at=now() WHERE id=$1
	`, orderID, paymentID)
	return err
}

func (r *Repository) MarkPaid(ctx context.Context, paymentID int64) error {
	// 1️⃣ Mark payment as paid
	_, err := r.q.Exec(ctx, `
		UPDATE payments
		   SET status='paid'::payment_status,
		       updated_at=now()
		 WHERE id=$1
	`, paymentID)
	if err != nil {
		return fmt.Errorf("mark payment paid: %w", err)
	}

	// 2️⃣ Mark order as paid + processing
	_, err = r.q.Exec(ctx, `
		UPDATE orders
		   SET payment_status='paid'::payment_status,
		       status='processing'::order_status,
		       paid_at=now(),
		       updated_at=now()
		 WHERE id = (
		   SELECT order_id
		     FROM payments
		    WHERE id=$1
		 )
	`, paymentID)
	if err != nil {
		return fmt.Errorf("mark order paid: %w", err)
	}

	return nil
}

func (r *Repository) SetStatus(ctx context.Context, paymentID int64, status string) error {
	_, err := r.q.Exec(ctx, `
		UPDATE payments
		   SET status=$2::payment_status, updated_at=now()
		 WHERE id=$1
	`, paymentID, status)
	return err
}

// List returns payments with optional filters:
// - status: if "" => no status filter
// - since: if nil => no time filter, else created_at >= *since
// Includes pagination via limit/offset and returns total count for pagination UI.
func (r *Repository) List(
	ctx context.Context,
	status string,
	since *time.Time,
	limit, offset int,
) ([]*Payment, int, error) {
	// Defensive defaults (same style you use elsewhere)
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}

	rows, err := r.q.Query(ctx, `
SELECT
  id,
  order_id,
  provider,
  provider_ref,
  amount_cents,
  currency,
  status,
  gateway_response,
  created_at,
  updated_at,
  COUNT(*) OVER() AS total_count
FROM payments
WHERE
  ($1 = '' OR status = $1::payment_status)
  AND ($2::timestamptz IS NULL OR created_at >= $2::timestamptz)
ORDER BY created_at DESC, id DESC
LIMIT $3 OFFSET $4
`,
		status,
		since, // nil is okay: $2 becomes NULL
		limit,
		offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("list payments: %w", err)
	}
	defer rows.Close()

	var (
		out   []*Payment
		total int
	)

	for rows.Next() {
		var p Payment
		var t int

		if err := rows.Scan(
			&p.ID,
			&p.OrderID,
			&p.Provider,
			&p.ProviderRef,
			&p.AmountCents,
			&p.Currency,
			&p.Status,
			&p.GatewayResp,
			&p.CreatedAt,
			&p.UpdatedAt,
			&t,
		); err != nil {
			return nil, 0, fmt.Errorf("scan payment: %w", err)
		}

		if total == 0 {
			total = t
		}
		out = append(out, &p)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("rows error: %w", err)
	}

	return out, total, nil
}

func (r *Repository) SetProviderRef(ctx context.Context, paymentID int64, ref string, raw any) error {
	var jb []byte
	if raw != nil {
		if b, err := json.Marshal(raw); err == nil {
			jb = b
		}
	}
	_, err := r.q.Exec(ctx, `
		UPDATE payments SET provider_ref=$2, gateway_response=$3, updated_at=now() WHERE id=$1
	`, paymentID, ref, jb)
	return err
}

func (r *Repository) GetByProviderRef(ctx context.Context, provider, ref string) (*Payment, error) {
	var p Payment

	err := r.q.QueryRow(ctx, `
		SELECT id, order_id, provider, provider_ref, amount_cents, currency, status,
		       gateway_response, created_at, updated_at
		FROM payments
		WHERE provider = $1 AND provider_ref = $2
		LIMIT 1
	`, provider, ref).Scan(
		&p.ID,
		&p.OrderID,
		&p.Provider,
		&p.ProviderRef,
		&p.AmountCents,
		&p.Currency,
		&p.Status,
		&p.GatewayResp, // Scan directly into json.RawMessage
		&p.CreatedAt,
		&p.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("get payment by provider_ref: %w", err)
	}

	return &p, nil
}

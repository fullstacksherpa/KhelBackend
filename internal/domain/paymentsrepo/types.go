package paymentsrepo

import (
	"context"
	"time"
)

type Payment struct {
	ID          int64     `json:"id"`
	OrderID     int64     `json:"order_id"`
	Provider    string    `json:"provider"`     // khalti, esewa, ...
	ProviderRef *string   `json:"provider_ref"` // pidx, etc
	AmountCents int64     `json:"amount_cents"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"` // pending, paid, failed, refunded...
	GatewayResp any       `json:"gateway_response,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Store interface {
	Create(ctx context.Context, p *Payment) (*Payment, error)
	SetPrimaryToOrder(ctx context.Context, orderID, paymentID int64) error
	MarkPaid(ctx context.Context, paymentID int64) error
	SetProviderRef(ctx context.Context, paymentID int64, ref string, raw any) error

	GetByID(ctx context.Context, id int64) (*Payment, error)
	GetByOrderID(ctx context.Context, orderID int64) ([]*Payment, error)
	SetStatus(ctx context.Context, paymentID int64, status string) error
	GetByProviderRef(ctx context.Context, provider, ref string) (*Payment, error)
}

package orders

import (
	"context"
	"time"
)

type Order struct {
	ID            int64      `json:"id"`
	UserID        int64      `json:"user_id"`
	OrderNumber   string     `json:"order_number"`
	Status        string     `json:"status"`
	PaymentStatus string     `json:"payment_status"`
	PaymentMethod *string    `json:"payment_method,omitempty"`
	PaidAt        *time.Time `json:"paid_at,omitempty"`
	SubtotalCents int64      `json:"subtotal_cents"`
	DiscountCents int64      `json:"discount_cents"`
	TaxCents      int64      `json:"tax_cents"`
	ShippingCents int64      `json:"shipping_cents"`
	TotalCents    int64      `json:"total_cents"`
	CreatedAt     time.Time  `json:"created_at"`
}

type ShippingInfo struct {
	Name       string
	Phone      string
	Address    string
	City       string
	PostalCode *string
	Country    *string
}

// Items from order_items table
type OrderItem struct {
	ID               int64          `json:"id"`
	OrderID          int64          `json:"order_id"`
	ProductID        *int64         `json:"product_id,omitempty"`
	ProductVariantID *int64         `json:"product_variant_id,omitempty"`
	ProductName      string         `json:"product_name"`
	VariantAttrs     map[string]any `json:"variant_attributes"`
	Quantity         int            `json:"quantity"`
	UnitPriceCents   int64          `json:"unit_price_cents"`
	TotalPriceCents  int64          `json:"total_price_cents"`
}

// Detailed view: order + items
type OrderDetail struct {
	Order Order       `json:"order"`
	Items []OrderItem `json:"items"`
}

type Store interface {
	// Checkout
	CreateFromCart(ctx context.Context, userID int64, ship ShippingInfo, method *string) (*Order, error)

	// Basic
	GetByID(ctx context.Context, id int64) (*Order, error)

	// USER-facing
	ListByUser(ctx context.Context, userID int64, status string, limit, offset int) ([]Order, int, error)
	GetDetailForUser(ctx context.Context, userID, orderID int64) (*OrderDetail, error)

	// ADMIN-facing
	ListAll(ctx context.Context, status string, limit, offset int) ([]Order, int, error)
	GetDetail(ctx context.Context, orderID int64) (*OrderDetail, error)
	UpdateStatus(ctx context.Context, orderID int64, status string, cancelledReason *string) error
}

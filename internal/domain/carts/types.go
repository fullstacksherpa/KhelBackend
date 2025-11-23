package carts

import (
	"context"
	"time"
)

type Cart struct {
	ID         int64      `json:"id"`
	UserID     *int64     `json:"user_id,omitempty"`
	GuestToken *string    `json:"guest_token,omitempty"`
	Status     string     `json:"status"` // active, converted, abandoned
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

type CartItem struct {
	ID               int64     `json:"id"`
	CartID           int64     `json:"cart_id"`
	ProductVariantID int64     `json:"product_variant_id"`
	Quantity         int       `json:"quantity"`
	PriceCents       int64     `json:"price_cents"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type CartView struct {
	Cart       Cart       `json:"cart"`
	Items      []CartLine `json:"items"`
	TotalCents int64      `json:"total_cents"`
}

type CartLine struct {
	ItemID          int64          `json:"item_id"`
	ProductID       int64          `json:"product_id"`
	VariantID       int64          `json:"variant_id"`
	ProductName     string         `json:"product_name"`
	VariantAttrs    map[string]any `json:"variant_attributes"`
	Quantity        int            `json:"quantity"`
	UnitPriceCents  int64          `json:"unit_price_cents"`
	LineTotalCents  int64          `json:"line_total_cents"`
	PrimaryImageURL *string        `json:"primary_image_url,omitempty"`
}

type Store interface {
	// --- User-level operations ---
	EnsureActive(ctx context.Context, userID int64) (int64, error)
	AddItem(ctx context.Context, userID, variantID int64, qty int) error
	UpdateItemQty(ctx context.Context, userID, itemID int64, qty int) error
	RemoveItem(ctx context.Context, userID, itemID int64) error
	Clear(ctx context.Context, userID int64) error
	GetView(ctx context.Context, userID int64) (*CartView, error)

	// TTL / housekeeping
	BumpTTL(ctx context.Context, userID int64) error

	// --- Admin / internal operations ---
	GetViewByCartID(ctx context.Context, cartID int64) (*CartView, error)
	List(ctx context.Context, status string, includeExpired bool, limit, offset int) ([]Cart, int, error)
	MarkExpiredAsAbandoned(ctx context.Context) (int64, error)
}

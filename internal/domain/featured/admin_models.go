package featured

import (
	"errors"
	"time"

	"khel/internal/params"
)

var (
	// Existing in types.go file:
	// ErrNotFound = errors.New("featured collection not found")

	ErrItemNotFound           = errors.New("featured item not found")
	ErrNoFieldsToUpdate       = errors.New("no fields to update")
	ErrDuplicateCollectionKey = errors.New("featured collection key already exists")
	ErrDuplicateItemPosition  = errors.New("featured item position already exists in this collection")
)

// FeaturedCollection is the admin CRUD model (source of truth table: featured_collections).
type FeaturedCollection struct {
	ID          int64      `json:"id"`
	Key         string     `json:"key"`
	Title       string     `json:"title"`
	Type        string     `json:"type"`
	Description *string    `json:"description,omitempty"`
	IsActive    bool       `json:"is_active"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// FeaturedItem is the admin CRUD model (source of truth table: featured_items).
type FeaturedItem struct {
	ID             int64   `json:"id"`
	CollectionID   int64   `json:"collection_id"`
	Position       int     `json:"position"`
	BadgeText      *string `json:"badge_text,omitempty"`
	Subtitle       *string `json:"subtitle,omitempty"`
	DealPriceCents *int64  `json:"deal_price_cents,omitempty"`
	DealPercent    *int    `json:"deal_percent,omitempty"`

	ProductID        *int64 `json:"product_id,omitempty"`
	ProductVariantID *int64 `json:"product_variant_id,omitempty"`

	IsActive  bool       `json:"is_active"`
	StartsAt  *time.Time `json:"starts_at,omitempty"`
	EndsAt    *time.Time `json:"ends_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// ---------- Create / Update Requests ----------

type CreateCollectionRequest struct {
	Key         string     `json:"key"`
	Title       string     `json:"title"`
	Type        string     `json:"type"`
	Description *string    `json:"description,omitempty"`
	IsActive    *bool      `json:"is_active,omitempty"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
}

type UpdateCollectionRequest struct {
	Key         *string    `json:"key,omitempty"`
	Title       *string    `json:"title,omitempty"`
	Type        *string    `json:"type,omitempty"`
	Description *string    `json:"description,omitempty"`
	IsActive    *bool      `json:"is_active,omitempty"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
}

type CreateItemRequest struct {
	CollectionID int64 `json:"collection_id"`

	Position       int     `json:"position"`
	BadgeText      *string `json:"badge_text,omitempty"`
	Subtitle       *string `json:"subtitle,omitempty"`
	DealPriceCents *int64  `json:"deal_price_cents,omitempty"`
	DealPercent    *int    `json:"deal_percent,omitempty"`

	ProductID        *int64 `json:"product_id,omitempty"`
	ProductVariantID *int64 `json:"product_variant_id,omitempty"`

	IsActive *bool      `json:"is_active,omitempty"`
	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
}

type UpdateItemRequest struct {
	Position       *int    `json:"position,omitempty"`
	BadgeText      *string `json:"badge_text,omitempty"`
	Subtitle       *string `json:"subtitle,omitempty"`
	DealPriceCents *int64  `json:"deal_price_cents,omitempty"`
	DealPercent    *int    `json:"deal_percent,omitempty"`

	ProductID        *int64 `json:"product_id,omitempty"`
	ProductVariantID *int64 `json:"product_variant_id,omitempty"`

	IsActive *bool      `json:"is_active,omitempty"`
	StartsAt *time.Time `json:"starts_at,omitempty"`
	EndsAt   *time.Time `json:"ends_at,omitempty"`
}

// ---------- Admin List Responses (use your params.Pagination) ----------

type CollectionFilters struct {
	Search *string // matches key/title
	Type   *string
	Active *bool
}

type ItemFilters struct {
	Active *bool
}

type CollectionList struct {
	Collections []FeaturedCollection `json:"collections"`
	Pagination  PaginationInfo       `json:"pagination"`
}

type ItemList struct {
	CollectionID int64          `json:"collection_id"`
	Items        []FeaturedItem `json:"items"`
	Pagination   PaginationInfo `json:"pagination"`
}

// Helper: convert your params.Pagination -> API PaginationInfo
func ToPaginationInfo(p params.Pagination) PaginationInfo {
	return PaginationInfo{
		Page:       p.Page,
		Limit:      p.Limit,
		Offset:     p.Offset,
		TotalItems: p.Total,
		TotalPages: p.TotalPages,
		HasNext:    p.HasNext,
		HasPrev:    p.HasPrev,
	}
}

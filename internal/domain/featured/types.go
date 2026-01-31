package featured

import (
	"context"
	"khel/internal/params"
	"time"
)

// CollectionPreview = home rails (up to 10 items + total count)
type CollectionPreview struct {
	Key         string           `json:"key"`
	Title       string           `json:"title"`
	Type        string           `json:"type"`
	Description *string          `json:"description,omitempty"`
	TotalItems  int              `json:"total_items"`
	Items       []CollectionItem `json:"items"`
	CachedAt    time.Time        `json:"cached_at"`
}

// CollectionDetail = collection page (paginated list)
type CollectionDetail struct {
	Collection CollectionInfo   `json:"collection"`
	Pagination PaginationInfo   `json:"pagination"`
	Items      []CollectionItem `json:"items"`
	CachedAt   time.Time        `json:"cached_at"`
}

type CollectionInfo struct {
	Key         string  `json:"key"`
	Title       string  `json:"title"`
	Type        string  `json:"type"`
	Description *string `json:"description,omitempty"`
}

type PaginationInfo struct {
	Page       int  `json:"page"`
	Limit      int  `json:"limit"`
	Offset     int  `json:"offset"`
	TotalItems int  `json:"total_items"`
	TotalPages int  `json:"total_pages"`
	HasNext    bool `json:"has_next"`
	HasPrev    bool `json:"has_prev"`
}

// CollectionItem = one card/tile on the UI
type CollectionItem struct {
	ItemID int64 `json:"item_id"`

	ProductID   int64   `json:"product_id"`
	ProductName string  `json:"product_name"`
	ProductSlug *string `json:"product_slug,omitempty"`

	VariantID  int64 `json:"variant_id"`
	PriceCents int64 `json:"price_cents"`

	ImageURL *string `json:"image_url,omitempty"`

	DealPriceCents *int64  `json:"deal_price_cents,omitempty"`
	DealPercent    *int    `json:"deal_percent,omitempty"`
	BadgeText      *string `json:"badge_text,omitempty"`
	Subtitle       *string `json:"subtitle,omitempty"`

	Position int `json:"position"`
}

type Store interface {
	// Public app reads (from MV cache)
	GetHomeCollections(ctx context.Context) ([]CollectionPreview, error)
	GetCollectionItems(ctx context.Context, collectionKey string, p params.Pagination) (*CollectionDetail, error)

	// MV refresh
	RefreshCache(ctx context.Context) error

	// Admin CRUD - collections (source of truth tables)
	CreateCollection(ctx context.Context, req CreateCollectionRequest) (*FeaturedCollection, error)
	GetCollectionByID(ctx context.Context, id int64) (*FeaturedCollection, error)
	UpdateCollection(ctx context.Context, id int64, req UpdateCollectionRequest) (*FeaturedCollection, error)
	DeleteCollection(ctx context.Context, id int64) error
	ListCollections(ctx context.Context, p params.Pagination, f CollectionFilters) (*CollectionList, error)

	// Admin CRUD - items
	CreateItem(ctx context.Context, req CreateItemRequest) (*FeaturedItem, error)
	GetItemByID(ctx context.Context, id int64) (*FeaturedItem, error)
	UpdateItem(ctx context.Context, id int64, req UpdateItemRequest) (*FeaturedItem, error)
	DeleteItem(ctx context.Context, id int64) error
	ListItemsByCollection(ctx context.Context, collectionID int64, p params.Pagination, f ItemFilters) (*ItemList, error)
}

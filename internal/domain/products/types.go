package products

import "time"

type Brand struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Slug        *string   `json:"slug"`
	Description *string   `json:"description,omitempty"`
	LogoURL     *string   `json:"logo_url,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type Category struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Slug      string    `json:"slug"`
	ParentID  *int64    `json:"parent_id,omitempty"`
	ImageURLs []string  `json:"image_urls,omitempty"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// CategoryWithRank includes relevance score for full-text search
type CategoryWithRank struct {
	Category
	Rank float32 `json:"rank,omitempty"` // Relevance score from 0.0 to 1.0
}

type CategoryWithChildren struct {
	ID        int64                   `json:"id"`
	Name      string                  `json:"name"`
	Slug      string                  `json:"slug"`
	ParentID  *int64                  `json:"parent_id,omitempty"`
	ImageURLs []string                `json:"image_urls,omitempty"`
	IsActive  bool                    `json:"is_active"`
	Level     int                     `json:"level"`
	Path      []int64                 `json:"path"`
	Children  []*CategoryWithChildren `json:"children,omitempty"`
	CreatedAt time.Time               `json:"created_at"`
	UpdatedAt time.Time               `json:"updated_at"`
}

type Product struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Slug        string    `json:"slug"`
	Description *string   `json:"description,omitempty"`
	CategoryID  *int64    `json:"category_id,omitempty"`
	BrandID     *int64    `json:"brand_id,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type ProductVariant struct {
	ID         int64          `json:"id"`
	ProductID  int64          `json:"product_id"`
	PriceCents int64          `json:"price_cents"`
	Attributes map[string]any `json:"attributes,omitempty"`
	IsActive   bool           `json:"is_active"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
}

type ProductImage struct {
	ID               int64     `json:"id"`
	ProductID        int64     `json:"product_id"`
	ProductVariantID *int64    `json:"product_variant_id,omitempty"`
	URL              string    `json:"url"`
	Alt              *string   `json:"alt,omitempty"`
	IsPrimary        bool      `json:"is_primary"`
	SortOrder        int       `json:"sort_order"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

// Lightweight “card” for lists
type ProductCard struct {
	ID              int64     `json:"id"`
	Name            string    `json:"name"`
	Slug            string    `json:"slug"`
	Description     *string   `json:"description,omitempty"`
	BrandID         *int64    `json:"brand_id,omitempty"`
	BrandName       *string   `json:"brand_name,omitempty"`
	CategoryID      *int64    `json:"category_id,omitempty"`
	CategoryName    *string   `json:"category_name,omitempty"`
	MinPriceCents   *int64    `json:"min_price_cents,omitempty"`
	PrimaryImageURL *string   `json:"primary_image_url,omitempty"`
	IsActive        bool      `json:"is_active"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ProductDetail struct {
	Product  *Product          `json:"product"`
	Brand    *Brand            `json:"brand,omitempty"`
	Category *Category         `json:"category,omitempty"`
	Variants []*ProductVariant `json:"variants"`
	Images   []*ProductImage   `json:"images"`
}

type ProductCardWithRank struct {
	ProductCard
	Rank float64 `json:"rank"`
}

type ProductWithRank struct {
	Product
	Rank float64 `json:"rank"`
}

type AdminProductCard struct {
	ProductCard
	VariantsCount int `json:"variants_count"`
	ImagesCount   int `json:"images_count"`
}

package ads

import "time"

// Ad represents the ads table structure
type Ad struct {
	ID           int64     `json:"id"`
	Title        string    `json:"title"`
	Description  *string   `json:"description"`
	ImageURL     string    `json:"image_url"`
	ImageAlt     *string   `json:"image_alt"`
	Link         *string   `json:"link"`
	Active       bool      `json:"active"`
	DisplayOrder int       `json:"display_order"`
	Impressions  int       `json:"impressions"`
	Clicks       int       `json:"clicks"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CreateAdRequest represents the request payload for creating an ad
type CreateAdRequest struct {
	Title        string  `json:"title"`
	Description  *string `json:"description"`
	ImageURL     string  `json:"image_url"`
	ImageAlt     *string `json:"image_alt"`
	Link         *string `json:"link"`
	DisplayOrder int     `json:"display_order"`
}

// UpdateAdRequest represents the request payload for updating an ad
type UpdateAdRequest struct {
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	ImageURL     *string `json:"image_url"`
	ImageAlt     *string `json:"image_alt"`
	Link         *string `json:"link"`
	Active       *bool   `json:"active"`
	DisplayOrder *int    `json:"display_order"`
}

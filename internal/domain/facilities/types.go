package facilities

import (
	"context"
	"errors"
	"time"
)

var ErrFacilityNotFound = errors.New("facility not found")

// Facility is a bookable unit inside a venue.
// Example: Ground A, Ground B, Court 1, Cricket Net 2.
type Facility struct {
	ID          int64   `json:"id"`
	VenueID     int64   `json:"venue_id"`
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
	Sport       *string `json:"sport,omitempty"`

	SurfaceType *string  `json:"surface_type,omitempty"`
	Capacity    *int     `json:"capacity,omitempty"`
	ImageURLs   []string `json:"image_urls,omitempty"`

	IsDefault bool      `json:"is_default"`
	IsActive  bool      `json:"is_active"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type CreateFacilityInput struct {
	VenueID     int64
	Name        string
	Description *string
	Sport       *string
	SurfaceType *string
	Capacity    *int
	ImageURLs   []string
	IsDefault   bool
}

type UpdateFacilityInput struct {
	Name        *string
	Description *string
	Sport       *string
	SurfaceType *string
	Capacity    *int
	ImageURLs   []string
	IsActive    *bool
	IsDefault   *bool
}

type Store interface {
	Create(ctx context.Context, input CreateFacilityInput) (*Facility, error)
	GetByID(ctx context.Context, venueID, facilityID int64) (*Facility, error)
	GetDefaultByVenueID(ctx context.Context, venueID int64) (*Facility, error)
	ListByVenueID(ctx context.Context, venueID int64) ([]Facility, error)
	Update(ctx context.Context, venueID, facilityID int64, input UpdateFacilityInput) (*Facility, error)
	Delete(ctx context.Context, venueID, facilityID int64) error
	BelongsToVenue(ctx context.Context, venueID, facilityID int64) (bool, error)
}

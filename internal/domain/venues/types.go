package venues

import (
	"context"
	"errors"
	"time"
)

var ErrVenueNotFound = errors.New("venue not found")

// Venue represents a venue in the database
type Venue struct {
	ID          int64     `json:"id"`
	OwnerID     int64     `json:"owner_id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	Location    []float64 `json:"location"` // PostGIS point (longitude, latitude)
	Description *string   `json:"description,omitempty"`
	PhoneNumber string    `json:"phone_number"`
	Amenities   []string  `json:"amenities,omitempty"` // Array of strings
	OpenTime    *string   `json:"open_time,omitempty"`
	Sport       string    `json:"sport"`
	ImageURLs   []string  `json:"image_urls,omitempty"` // Array of image URLs
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type VenueInfo struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Address     string    `json:"address"`
	Location    []float64 `json:"location"` // PostGIS point (longitude, latitude)
	Description *string   `json:"description,omitempty"`
	PhoneNumber string    `json:"phone_number"`
	Amenities   []string  `json:"amenities,omitempty"` // Array of strings
	OpenTime    *string   `json:"open_time,omitempty"`
	Status      string    `json:"status"`
}

// VenueDetail extends Venue with aggregation fields from reviews and games.
type VenueDetail struct {
	Venue
	TotalReviews   int     `json:"total_reviews"`
	AverageRating  float64 `json:"average_rating"`
	UpcomingGames  int     `json:"upcoming_games"`
	CompletedGames int     `json:"completed_games"`
}

type VenueFilter struct {
	Sport     *string
	Latitude  *float64
	Longitude *float64
	Distance  *float64 // meters
	Page      int
	Limit     int
}

type VenueListing struct {
	ID            int64
	Name          string
	Address       string
	Longitude     float64
	Latitude      float64
	ImageURLs     []string
	OpenTime      *string
	PhoneNumber   string
	Sport         string
	TotalReviews  int
	AverageRating float64
}

// FavoriteVenue represents a favorite venue record.
type FavoriteVenue struct {
	UserID    int64     `json:"user_id"`
	VenueID   int64     `json:"venue_id"`
	CreatedAt time.Time `json:"created_at"`
}

type VenueListingWithRank struct {
	VenueListing
	Rank float64 `json:"rank"`
}

type Store interface {
	Create(context.Context, *Venue) error
	UpdateImageURLs(ctx context.Context, venueID int64, urls []string) error
	Delete(ctx context.Context, venueID int64) error
	Update(ctx context.Context, venueID int64, updateData map[string]interface{}) error
	CheckIfVenueExists(context.Context, string, int64) (bool, error)
	RemovePhotoURL(ctx context.Context, venueID int64, photoURL string) error
	AddPhotoURL(ctx context.Context, venueID int64, photoURL string) error
	IsOwner(ctx context.Context, venueID int64, userID int64) (bool, error)
	GetVenueByID(ctx context.Context, venueID int64) (*Venue, error)
	GetOwnedVenueIDs(ctx context.Context, userID int64) ([]int64, error)
	List(ctx context.Context, filter VenueFilter) ([]VenueListing, error)
	GetVenueDetail(ctx context.Context, venueID int64) (*VenueDetail, error)
	GetImageURLs(ctx context.Context, venueID int64) ([]string, error)
	GetVenueInfo(ctx context.Context, venueID int64) (*VenueInfo, error)
	GetOwnerIDFromVenueID(ctx context.Context, venueID int64) (int64, error)
	// ... favourite venues
	AddFavorite(ctx context.Context, userID, venueID int64) error
	RemoveFavorite(ctx context.Context, userID, venueID int64) error
	GetFavoritesByUser(ctx context.Context, userID int64) ([]Venue, error)
	GetFavoriteVenueIDsByUser(ctx context.Context, userID int64) (map[int64]struct{}, error)

	// Search Functionality
	SearchVenues(ctx context.Context, query string) ([]VenueListing, error)
	FullTextSearchVenues(ctx context.Context, query string) ([]VenueListingWithRank, error)
}

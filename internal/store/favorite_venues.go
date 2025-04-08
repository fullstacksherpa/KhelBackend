package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

// FavoriteVenue represents a favorite venue record.
type FavoriteVenue struct {
	UserID    int64     `json:"user_id"`
	VenueID   int64     `json:"venue_id"`
	CreatedAt time.Time `json:"created_at"`
}

// FavoriteVenuesStore handles database operations for favorite venues.
type FavoriteVenuesStore struct {
	db *sql.DB
}

// AddFavorite inserts a record into the favorite_venues table.
func (s *FavoriteVenuesStore) AddFavorite(ctx context.Context, userID, venueID int64) error {
	query := `
		INSERT INTO favorite_venues (user_id, venue_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := s.db.ExecContext(ctx, query, userID, venueID)
	if err != nil {
		return fmt.Errorf("failed to add favorite: %w", err)
	}
	return nil
}

// RemoveFavorite deletes a record from the favorite_venues table.
func (s *FavoriteVenuesStore) RemoveFavorite(ctx context.Context, userID, venueID int64) error {
	query := `
		DELETE FROM favorite_venues
		WHERE user_id = $1 AND venue_id = $2
	`
	_, err := s.db.ExecContext(ctx, query, userID, venueID)
	if err != nil {
		return fmt.Errorf("failed to remove favorite: %w", err)
	}
	return nil
}

// TODO: add phone number here
// GetFavoritesByUser returns all venues that a user has favorited.
// It performs a join between favorite_venues and venues.
func (s *FavoriteVenuesStore) GetFavoritesByUser(ctx context.Context, userID int64) ([]Venue, error) {
	query := `
		SELECT v.id, v.owner_id, v.name, v.address, v.description, v.amenities,
		       v.open_time, v.image_urls, v.sport, v.created_at, v.updated_at
		FROM venues v
		JOIN favorite_venues f ON v.id = f.venue_id
		WHERE f.user_id = $1
		ORDER BY f.created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get favorites: %w", err)
	}
	defer rows.Close()

	var favorites []Venue
	for rows.Next() {
		var v Venue
		var amenitiesJSON []byte
		var imageURLsJSON []byte
		// Scan the venue fields – be sure to match the order and types.
		if err := rows.Scan(
			&v.ID, &v.OwnerID, &v.Name, &v.Address, &v.Description,
			&amenitiesJSON, &v.OpenTime, &imageURLsJSON, &v.Sport,
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan venue row: %w", err)
		}
		// Unmarshal JSON arrays if needed.
		if err := json.Unmarshal(amenitiesJSON, &v.Amenities); err != nil {
			return nil, fmt.Errorf("failed to unmarshal amenities: %w", err)
		}
		if err := json.Unmarshal(imageURLsJSON, &v.ImageURLs); err != nil {
			return nil, fmt.Errorf("failed to unmarshal image_urls: %w", err)
		}
		favorites = append(favorites, v)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return favorites, nil
}

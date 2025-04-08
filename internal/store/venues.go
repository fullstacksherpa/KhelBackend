package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype" // For PostGIS GEOGRAPHY
	"github.com/lib/pq"
)

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

type VenuesStore struct {
	db *sql.DB
}

// CheckIfVenueExists checks if a venue with the same name and owner already exists
func (s *VenuesStore) CheckIfVenueExists(ctx context.Context, name string, ownerID int64) (bool, error) {
	query := `
		SELECT id FROM venues WHERE name = $1 AND owner_id = $2
	`

	var existingVenueID int64
	err := s.db.QueryRowContext(ctx, query, name, ownerID).Scan(&existingVenueID)

	// If an error is not found, it means the venue exists
	if err == nil {
		return true, nil
	}

	// If no rows are found, then no venue with the same name exists
	if err == sql.ErrNoRows {
		return false, nil
	}

	// If there is any other error, return it
	return false, err
}

// Create creates a new venue in the database
func (s *VenuesStore) Create(ctx context.Context, venue *Venue) error {
	// Check if the venue with the same name and owner already exists
	exists, err := s.CheckIfVenueExists(ctx, venue.Name, venue.OwnerID)
	if err != nil {
		return fmt.Errorf("error checking if venue exists: %w", err)
	}
	if exists {
		return fmt.Errorf("a venue with this name already exists for this owner")
	}

	// Proceed with insertion if venue does not exist
	query := `
	INSERT INTO venues (owner_id, name, address, location, description, amenities, open_time, image_urls, sport, phone_number)
	VALUES ($1, $2, $3, ST_SetSRID(ST_MakePoint($4, $5), 4326), $6, $7, $8, $9, $10, $11)
	RETURNING id, created_at, updated_at
`

	// Create a pgtype.Point for PostGIS geography
	point := pgtype.Point{
		P: pgtype.Vec2{
			X: venue.Location[0], // Longitude
			Y: venue.Location[1], // Latitude
		},
		Valid: true, // Make sure the point is valid
	}

	err = s.db.QueryRowContext(
		ctx, query,
		venue.OwnerID,
		venue.Name,
		venue.Address,
		point.P.X, // Longitude
		point.P.Y, // Latitude
		venue.Description,
		pq.Array(venue.Amenities),
		venue.OpenTime,
		pq.Array(venue.ImageURLs),
		venue.Sport,
		venue.PhoneNumber,
	).Scan(
		&venue.ID,
		&venue.CreatedAt,
		&venue.UpdatedAt,
	)

	if err != nil {
		return err
	}
	return nil
}

// RemovePhotoURL removes a specific photo URL from a venue's image_urls array
func (s *VenuesStore) RemovePhotoURL(ctx context.Context, venueID int64, photoURL string) error {
	query := `
		UPDATE venues
		SET image_urls = array_remove(image_urls, $1)
		WHERE id = $2
	`
	_, err := s.db.ExecContext(ctx, query, photoURL, venueID)
	if err != nil {
		return fmt.Errorf("failed to remove photo URL: %w", err)
	}
	return nil
}

// AddPhotoURL adds a new photo URL to a venue's image_urls array
func (s *VenuesStore) AddPhotoURL(ctx context.Context, venueID int64, photoURL string) error {
	query := `
		UPDATE venues
		SET image_urls = array_append(image_urls, $1)
		WHERE id = $2
	`
	_, err := s.db.ExecContext(ctx, query, photoURL, venueID)
	if err != nil {
		return fmt.Errorf("failed to add photo URL: %w", err)
	}
	return nil
}

// Update updates a venue's data in the database
func (s *VenuesStore) Update(ctx context.Context, venueID int64, updateData map[string]interface{}) error {
	// Start building the SQL query
	query := "UPDATE venues SET "
	args := []interface{}{}
	argCounter := 1

	// Iterate over the updateData map to build the query dynamically
	for key, value := range updateData {
		switch key {
		case "name":
			query += fmt.Sprintf("name = $%d, ", argCounter)
			args = append(args, value)
			argCounter++
		case "address":
			query += fmt.Sprintf("address = $%d, ", argCounter)
			args = append(args, value)
			argCounter++
		case "location":
			// Handle location as a PostGIS point
			if location, ok := value.([]float64); ok && len(location) == 2 {
				query += fmt.Sprintf("location = ST_SetSRID(ST_MakePoint($%d, $%d), 4326), ", argCounter, argCounter+1)
				args = append(args, location[0], location[1]) // Longitude, Latitude
				argCounter += 2
			} else {
				return fmt.Errorf("invalid location data")
			}
		case "description":
			query += fmt.Sprintf("description = $%d, ", argCounter)
			args = append(args, value)
			argCounter++
		case "amenities":
			// Handle amenities as an array of strings
			if amenities, ok := value.([]string); ok {
				query += fmt.Sprintf("amenities = $%d, ", argCounter)
				args = append(args, pq.Array(amenities))
				argCounter++
			} else {
				return fmt.Errorf("invalid amenities data")
			}
		case "open_time":
			query += fmt.Sprintf("open_time = $%d, ", argCounter)
			args = append(args, value)
			argCounter++
		case "sport":
			query += fmt.Sprintf("sport = $%d, ", argCounter)
			args = append(args, value)
			argCounter++
		case "phone_number":
			query += fmt.Sprintf("phone_number = $%d, ", argCounter)
			args = append(args, value)
			argCounter++
		default:
			return fmt.Errorf("unsupported field: %s", key)
		}
	}

	// Remove the trailing comma and space
	query = query[:len(query)-2]

	// Add the WHERE clause
	query += fmt.Sprintf(" WHERE id = $%d", argCounter)
	args = append(args, venueID)

	// Execute the query
	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to update venue: %w", err)
	}

	return nil
}

// IsOwner checks if the user is the owner of the given venue
func (s *VenuesStore) IsOwner(ctx context.Context, venueID int64, userID int64) (bool, error) {
	var ownerID int64

	err := s.db.QueryRowContext(ctx, `SELECT owner_id FROM venues WHERE id = $1`, venueID).Scan(&ownerID)
	if err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("venue not found")
		}

		return false, err
	}

	// Check if the user ID matches the owner ID
	if ownerID == userID {
		return true, nil
	}

	// If the user is not the owner
	return false, nil
}

// IsOwnerOfAnyVenue checks if the user owns any venue
func (s *VenuesStore) IsOwnerOfAnyVenue(ctx context.Context, userID int64) (bool, error) {
	query := `SELECT 1 FROM venues WHERE owner_id = $1 LIMIT 1`
	var dummy int
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&dummy)
	if err == sql.ErrNoRows {
		return false, nil
	}
	return err == nil, err
}

// GetVenueByID retrieves a venue by its ID.
func (s *VenuesStore) GetVenueByID(ctx context.Context, venueID int64) (*Venue, error) {
	query := `
	SELECT id, owner_id, name, address, description, amenities, open_time, image_urls, sport, phone_number, created_at, updated_at 
	FROM venues 
	WHERE id = $1`
	row := s.db.QueryRowContext(ctx, query, venueID)
	var v Venue
	var amenitiesJSON []byte
	var imageURLsJSON []byte
	if err := row.Scan(&v.ID, &v.OwnerID, &v.Name, &v.Address, &v.Description, &amenitiesJSON, &v.OpenTime, &imageURLsJSON, &v.Sport, &v.PhoneNumber, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	// Unmarshal JSON arrays.
	if err := json.Unmarshal(amenitiesJSON, &v.Amenities); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(imageURLsJSON, &v.ImageURLs); err != nil {
		return nil, err
	}
	return &v, nil
}

type VenueFilter struct {
	Sport          *string
	Latitude       *float64
	Longitude      *float64
	Distance       *float64 // meters
	FavoriteUserID *int64
	Page           int
	Limit          int
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

func (s *VenuesStore) List(ctx context.Context, filter VenueFilter) ([]VenueListing, error) {
	baseQuery := `
        SELECT 
            v.id,
            v.name,
            v.address,
            ST_X(v.location) as longitude,
            ST_Y(v.location) as latitude,
            v.image_urls,
            v.open_time,
            v.phone_number,
            v.sport,
            COUNT(r.id) as total_reviews,
            COALESCE(AVG(r.rating), 0) as average_rating
        FROM venues v
        LEFT JOIN reviews r ON v.id = r.venue_id
    `

	var where []string
	var args []interface{}
	argCounter := 1

	// Sport filter
	if filter.Sport != nil {
		where = append(where, fmt.Sprintf("v.sport = $%d", argCounter))
		args = append(args, *filter.Sport)
		argCounter++
	}

	// Location filter
	if filter.Latitude != nil && filter.Longitude != nil && filter.Distance != nil {
		where = append(where,
			fmt.Sprintf("ST_DWithin(v.location::geography, ST_MakePoint($%d, $%d)::geography, $%d)",
				argCounter, argCounter+1, argCounter+2))
		args = append(args, *filter.Longitude, *filter.Latitude, *filter.Distance)
		argCounter += 3
	}

	// Favorite filter
	if filter.FavoriteUserID != nil {
		where = append(where,
			fmt.Sprintf("EXISTS (SELECT 1 FROM favorites f WHERE f.venue_id = v.id AND f.user_id = $%d)",
				argCounter))
		args = append(args, *filter.FavoriteUserID)
		argCounter++
	}

	// Build final query
	query := baseQuery
	if len(where) > 0 {
		query += " WHERE " + strings.Join(where, " AND ")
	}
	query += " GROUP BY v.id"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying venues: %w", err)
	}
	defer rows.Close()

	var venues []VenueListing
	for rows.Next() {
		var v VenueListing
		var openTime sql.NullString

		err := rows.Scan(
			&v.ID,
			&v.Name,
			&v.Address,
			&v.Longitude,
			&v.Latitude,
			pq.Array(&v.ImageURLs),
			&openTime,
			&v.PhoneNumber,
			&v.Sport,
			&v.TotalReviews,
			&v.AverageRating,
		)

		if err != nil {
			return nil, fmt.Errorf("error scanning venue row: %w", err)
		}

		if openTime.Valid {
			v.OpenTime = &openTime.String
		}

		venues = append(venues, v)
	}

	return venues, nil
}

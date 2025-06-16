package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	// For PostGIS GEOGRAPHY
	"github.com/jackc/pgx/v5/pgxpool"
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

type VenuesStore struct {
	db *pgxpool.Pool
}

// CheckIfVenueExists checks if a venue with the same name and owner already exists
func (s *VenuesStore) CheckIfVenueExists(ctx context.Context, name string, ownerID int64) (bool, error) {
	query := `
		SELECT id FROM venues WHERE name = $1 AND owner_id = $2
	`

	var existingVenueID int64
	err := s.db.QueryRow(ctx, query, name, ownerID).Scan(&existingVenueID)

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

	const query = `
    INSERT INTO venues (
      owner_id, name, address, location,
      description, amenities, open_time,
      image_urls, sport, phone_number
    ) VALUES (
      $1, $2, $3,
      ST_SetSRID(ST_MakePoint($4, $5), 4326),
      $6, $7, $8, $9, $10, $11
    )
    RETURNING id, created_at, updated_at
    `

	// Build the args array—make absolutely sure you have exactly 11 items here:
	args := []interface{}{
		venue.OwnerID,
		venue.Name,
		venue.Address,
		venue.Location[0], // longitude
		venue.Location[1], // latitude
		venue.Description,
		venue.Amenities,
		venue.OpenTime,
		[]string{}, // initial empty image_urls
		venue.Sport,
		venue.PhoneNumber,
	}
	fmt.Println("🔨 Raw SQL:", query)
	fmt.Printf("📦 ARGS: %#v\n", args)
	row := s.db.QueryRow(ctx, query, args...)
	if err := row.Scan(&venue.ID, &venue.CreatedAt, &venue.UpdatedAt); err != nil {
		fmt.Println("❌ Scan error:", err)
		if errors.Is(err, sql.ErrNoRows) {
			// Insert succeeded, but didn’t RETURN any row
			return fmt.Errorf("venue insert returned no rows — please verify the SQL & table schema: %w", err)
		}
		return fmt.Errorf("error scanning insert result: %w", err)
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
	_, err := s.db.Exec(ctx, query, photoURL, venueID)
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
	_, err := s.db.Exec(ctx, query, photoURL, venueID)
	if err != nil {
		return fmt.Errorf("failed to add photo URL: %w", err)
	}
	return nil
}

// Update updates a venue's data in the database
func (s *VenuesStore) Update(ctx context.Context, venueID int64, updateData map[string]interface{}) error {
	query := "UPDATE venues SET "
	args := []interface{}{}
	argCounter := 1

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
			if loc, ok := value.([]float64); ok && len(loc) == 2 {
				query += fmt.Sprintf("location = ST_SetSRID(ST_MakePoint($%d, $%d), 4326), ", argCounter, argCounter+1)
				args = append(args, loc[0], loc[1])
				argCounter += 2
			} else {
				return fmt.Errorf("invalid location data")
			}
		case "description":
			query += fmt.Sprintf("description = $%d, ", argCounter)
			args = append(args, value)
			argCounter++
		case "amenities":
			if raw, ok := value.([]interface{}); ok {
				var amenities []string
				for _, item := range raw {
					if str, ok := item.(string); ok {
						amenities = append(amenities, str)
					} else {
						return fmt.Errorf("invalid item in amenities array")
					}
				}
				query += fmt.Sprintf("amenities = $%d, ", argCounter)
				args = append(args, amenities)
				argCounter++
			} else {
				return fmt.Errorf("invalid amenities data")
			}
		case "open_time":
			query += fmt.Sprintf("open_time = $%d, ", argCounter)
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
	// Trim trailing comma & space
	query = strings.TrimSuffix(query, ", ")
	query += fmt.Sprintf(" WHERE id = $%d", argCounter)
	args = append(args, venueID)

	if _, err := s.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to update venue: %w", err)
	}
	return nil
}

// IsOwner checks if the user is the owner of the given venue
func (s *VenuesStore) IsOwner(ctx context.Context, venueID int64, userID int64) (bool, error) {
	var ownerID int64

	err := s.db.QueryRow(ctx, `SELECT owner_id FROM venues WHERE id = $1`, venueID).Scan(&ownerID)
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

func (s *VenuesStore) GetOwnedVenueIDs(ctx context.Context, userID int64) ([]int64, error) {
	query := `SELECT id FROM venues WHERE owner_id = $1`
	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var venueIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		venueIDs = append(venueIDs, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return venueIDs, nil
}

// GetVenueByID retrieves a venue by its ID.
func (s *VenuesStore) GetVenueByID(ctx context.Context, venueID int64) (*Venue, error) {
	query := `
	SELECT id, owner_id, name, address, description, amenities, open_time, image_urls, sport, phone_number, created_at, updated_at 
	FROM venues 
	WHERE id = $1`
	row := s.db.QueryRow(ctx, query, venueID)
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

// List returns a paginated slice of VenueListing, optionally filtered by sport
// and/or by a geographic radius, and—when a location is provided—sorted
// nearest-first.
func (s *VenuesStore) List(ctx context.Context, filter VenueFilter) ([]VenueListing, error) {
	var (
		where      []string
		args       []interface{}
		argCounter int = 1
		orderBy    string
	)

	// 1) Sport filter
	if filter.Sport != nil {
		where = append(where, fmt.Sprintf("v.sport = $%d", argCounter))
		args = append(args, *filter.Sport)
		argCounter++
	}

	// 2) Location filter
	hasLocation := filter.Latitude != nil && filter.Longitude != nil && filter.Distance != nil
	var lonPos, latPos int
	if hasLocation {
		where = append(where, fmt.Sprintf(
			"ST_DWithin(v.location::geography, ST_MakePoint($%d, $%d)::geography, $%d)",
			argCounter, argCounter+1, argCounter+2,
		))
		args = append(args,
			*filter.Longitude,
			*filter.Latitude,
			*filter.Distance,
		)
		lonPos, latPos = argCounter, argCounter+1
		argCounter += 3

		orderBy = fmt.Sprintf(
			"ST_Distance(v.location::geography, ST_MakePoint($%d, $%d)::geography) ASC",
			lonPos, latPos,
		)
	}

	// 3) Build query using WITH clause for pre-aggregated stats
	query := `
		WITH venue_stats AS (
			SELECT venue_id, COUNT(*) AS total_reviews, AVG(rating) AS average_rating
			FROM reviews
			GROUP BY venue_id
		)
		SELECT
			v.id,
			v.name,
			v.address,
			ST_X(v.location::geometry)    AS longitude,
			ST_Y(v.location::geometry)    AS latitude,
			v.image_urls,
			v.open_time,
			v.phone_number,
			v.sport,
			COALESCE(vs.total_reviews, 0),
			COALESCE(vs.average_rating, 0)
		FROM venues v
		LEFT JOIN venue_stats vs ON v.id = vs.venue_id
	`

	where = append(where, "v.status = 'active'")
	query += " WHERE " + strings.Join(where, " AND ")

	if hasLocation {
		query += " ORDER BY " + orderBy
	} else {
		query += " ORDER BY v.name"
	}

	// 4) Pagination
	query += fmt.Sprintf(" LIMIT $%d OFFSET $%d", argCounter, argCounter+1)
	args = append(args,
		filter.Limit,
		(filter.Page-1)*filter.Limit,
	)

	// 5) Execute query
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("error querying venues: %w", err)
	}
	defer rows.Close()

	// 6) Scan results
	var venues []VenueListing
	for rows.Next() {
		var v VenueListing
		var openTime sql.NullString

		if err := rows.Scan(
			&v.ID,
			&v.Name,
			&v.Address,
			&v.Longitude,
			&v.Latitude,
			&v.ImageURLs,
			&openTime,
			&v.PhoneNumber,
			&v.Sport,
			&v.TotalReviews,
			&v.AverageRating,
		); err != nil {
			return nil, fmt.Errorf("error scanning venue row: %w", err)
		}
		if openTime.Valid {
			v.OpenTime = &openTime.String
		}
		venues = append(venues, v)
	}

	return venues, nil
}

// UpdateImageURLs updates the venue record with the provided image URLs.
func (s *VenuesStore) UpdateImageURLs(ctx context.Context, venueID int64, urls []string) error {
	query := `UPDATE venues SET image_urls = $1 WHERE id = $2`
	_, err := s.db.Exec(ctx, query, urls, venueID)
	return err
}

// Delete removes the venue with the given ID from the database.
func (s *VenuesStore) Delete(ctx context.Context, venueID int64) error {
	query := `DELETE FROM venues WHERE id = $1`
	_, err := s.db.Exec(ctx, query, venueID)
	return err
}

// GetVenueDetail retrieves a venue by its ID while joining reviews and games
// to aggregate total_reviews, average_rating, upcoming_games, and completed_games.
func (s *VenuesStore) GetVenueDetail(ctx context.Context, venueID int64) (*VenueDetail, error) {
	query := `
	SELECT 
		v.id,
		v.owner_id,
		v.name,
		v.address,
		ST_X(v.location::geometry) as longitude,
		ST_Y(v.location::geometry) as latitude,
		v.description,
		v.phone_number,
		v.amenities,
		v.open_time,
		v.sport,
		v.image_urls,
		v.created_at,
		v.updated_at,
		COUNT(DISTINCT r.id) AS total_reviews,
		COALESCE(AVG(r.rating), 0) AS average_rating,
		COUNT(DISTINCT CASE WHEN g.start_time > NOW() THEN g.id END) AS upcoming_games,
		COUNT(DISTINCT CASE WHEN g.status = 'completed' THEN g.id END) AS completed_games
	FROM venues v
	LEFT JOIN reviews r ON v.id = r.venue_id
	LEFT JOIN games g ON v.id = g.venue_id
	WHERE v.id = $1
	GROUP BY 
		v.id, v.owner_id, v.name, v.address, v.location, v.description,
		v.phone_number, v.amenities, v.open_time, v.sport, v.image_urls, v.created_at, v.updated_at
	`

	var vd VenueDetail
	var longitude, latitude float64

	err := s.db.QueryRow(ctx, query, venueID).Scan(
		&vd.ID,
		&vd.OwnerID,
		&vd.Name,
		&vd.Address,
		&longitude,
		&latitude,
		&vd.Description,
		&vd.PhoneNumber,
		&vd.Amenities,
		&vd.OpenTime,
		&vd.Sport,
		&vd.ImageURLs,
		&vd.CreatedAt,
		&vd.UpdatedAt,
		&vd.TotalReviews,
		&vd.AverageRating,
		&vd.UpcomingGames,
		&vd.CompletedGames,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("venue not found")
		}
		return nil, err
	}

	// Set the Location slice with [latitude, longitude]. Adjust the order if necessary.
	vd.Location = []float64{latitude, longitude}

	return &vd, nil
}

func (s *VenuesStore) GetImageURLs(ctx context.Context, venueID int64) ([]string, error) {
	query := `SELECT image_urls FROM venues WHERE id = $1`

	var urls []string
	if err := s.db.QueryRow(ctx, query, venueID).Scan(&urls); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("venue %d not found", venueID)
		}
		return nil, err
	}
	return urls, nil
}

// just get basic venueinfo without images and rating and rate count, much
// lighter than get venue details method.
func (s *VenuesStore) GetVenueInfo(ctx context.Context, venueID int64) (*VenueInfo, error) {
	query := `SELECT id, name, address, ST_X(location::geometry) as longitude,
		ST_Y(location::geometry) as latitude, description, amenities, open_time, phone_number, status FROM venues WHERE venues.id = $1`

	var VenueInfo VenueInfo
	var longitude, latitude float64

	err := s.db.QueryRow(ctx, query, venueID).Scan(
		&VenueInfo.ID,
		&VenueInfo.Name,
		&VenueInfo.Address,
		&longitude,
		&latitude,
		&VenueInfo.Description,
		&VenueInfo.Amenities,
		&VenueInfo.OpenTime,
		&VenueInfo.PhoneNumber,
		&VenueInfo.Status,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrVenueNotFound
		}
		return nil, err
	}

	VenueInfo.Location = []float64{latitude, longitude}

	return &VenueInfo, nil
}

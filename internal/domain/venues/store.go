package venues

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

// CheckIfVenueExists checks if a venue with the same name and owner already exists
func (r *Repository) CheckIfVenueExists(ctx context.Context, name string, ownerID int64) (bool, error) {
	query := `
		SELECT id FROM venues WHERE name = $1 AND owner_id = $2
	`

	var existingVenueID int64
	err := r.db.QueryRow(ctx, query, name, ownerID).Scan(&existingVenueID)

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
func (r *Repository) Create(ctx context.Context, venue *Venue) error {

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
	row := r.db.QueryRow(ctx, query, args...)
	if err := row.Scan(&venue.ID, &venue.CreatedAt, &venue.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Insert succeeded, but didn’t RETURN any row
			return fmt.Errorf("venue insert returned no rows — please verify the SQL & table schema: %w", err)
		}
		return fmt.Errorf("error scanning insert result: %w", err)
	}
	return nil
}

// RemovePhotoURL removes a specific photo URL from a venue's image_urls array
func (r *Repository) RemovePhotoURL(ctx context.Context, venueID int64, photoURL string) error {
	query := `
		UPDATE venues
		SET image_urls = array_remove(image_urls, $1)
		WHERE id = $2
	`
	_, err := r.db.Exec(ctx, query, photoURL, venueID)
	if err != nil {
		return fmt.Errorf("failed to remove photo URL: %w", err)
	}
	return nil
}

// AddPhotoURL adds a new photo URL to a venue's image_urls array
func (r *Repository) AddPhotoURL(ctx context.Context, venueID int64, photoURL string) error {
	query := `
		UPDATE venues
		SET image_urls = array_append(image_urls, $1)
		WHERE id = $2
	`
	_, err := r.db.Exec(ctx, query, photoURL, venueID)
	if err != nil {
		return fmt.Errorf("failed to add photo URL: %w", err)
	}
	return nil
}

// Update updates a venue's data in the database
func (r *Repository) Update(ctx context.Context, venueID int64, updateData map[string]interface{}) error {
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

	if _, err := r.db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("failed to update venue: %w", err)
	}
	return nil
}

// IsOwner checks if the user is the owner of the given venue
func (r *Repository) IsOwner(ctx context.Context, venueID int64, userID int64) (bool, error) {
	var ownerID int64

	err := r.db.QueryRow(ctx, `SELECT owner_id FROM venues WHERE id = $1`, venueID).Scan(&ownerID)
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

func (r *Repository) GetOwnedVenueIDs(ctx context.Context, userID int64) ([]int64, error) {
	query := `SELECT id FROM venues WHERE owner_id = $1`
	rows, err := r.db.Query(ctx, query, userID)
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
func (r *Repository) GetVenueByID(ctx context.Context, venueID int64) (*Venue, error) {
	query := `
	SELECT id, owner_id, name, address, description, amenities, open_time, image_urls, sport, phone_number, created_at, updated_at 
	FROM venues 
	WHERE id = $1`
	row := r.db.QueryRow(ctx, query, venueID)
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

// List returns a paginated slice of VenueListing, optionally filtered by sport
// and/or by a geographic radius, and—when a location is provided—sorted
// nearest-first.
func (r *Repository) List(ctx context.Context, filter VenueFilter) ([]VenueListing, error) {
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
	rows, err := r.db.Query(ctx, query, args...)
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
func (r *Repository) UpdateImageURLs(ctx context.Context, venueID int64, urls []string) error {
	query := `UPDATE venues SET image_urls = $1 WHERE id = $2`
	_, err := r.db.Exec(ctx, query, urls, venueID)
	return err
}

// Delete removes the venue with the given ID from the database.
func (r *Repository) Delete(ctx context.Context, venueID int64) error {
	query := `DELETE FROM venues WHERE id = $1`
	_, err := r.db.Exec(ctx, query, venueID)
	return err
}

// GetVenueDetail retrieves a venue by its ID while joining reviews and games
// to aggregate total_reviews, average_rating, upcoming_games, and completed_games.
func (r *Repository) GetVenueDetail(ctx context.Context, venueID int64) (*VenueDetail, error) {
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

	err := r.db.QueryRow(ctx, query, venueID).Scan(
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

func (r *Repository) ListWithTotal(ctx context.Context, filter AdminVenueFilter) (*AdminVenueListResult, error) {
	var (
		where      []string
		args       []interface{}
		argCounter = 1
	)

	// ✅ Default behavior: if status not provided, show only active.
	if filter.Status == nil || strings.TrimSpace(*filter.Status) == "" {
		where = append(where, "v.status = 'active'")
	} else {
		// ✅ allow explicit status filter
		where = append(where, fmt.Sprintf("v.status = $%d", argCounter))
		args = append(args, strings.TrimSpace(*filter.Status))
		argCounter++
	}

	// ✅ optional sport filter
	if filter.Sport != nil && strings.TrimSpace(*filter.Sport) != "" {
		where = append(where, fmt.Sprintf("v.sport = $%d", argCounter))
		args = append(args, strings.TrimSpace(*filter.Sport))
		argCounter++
	}

	whereSQL := " WHERE " + strings.Join(where, " AND ")

	// ---- 1) total count ----
	countQ := `SELECT COUNT(*) FROM venues v` + whereSQL

	var total int
	if err := r.db.QueryRow(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, fmt.Errorf("count venues: %w", err)
	}

	// ---- 2) list data ----
	limitPos := argCounter
	offsetPos := argCounter + 1

	dataQ := fmt.Sprintf(`
		WITH venue_stats AS (
			SELECT venue_id, COUNT(*) AS total_reviews, AVG(rating) AS average_rating
			FROM reviews
			GROUP BY venue_id
		)
		SELECT
			v.id,
			v.name,
			v.address,
			ST_X(v.location::geometry) AS longitude,
			ST_Y(v.location::geometry) AS latitude,
			v.image_urls,
			v.open_time,
			v.phone_number,
			v.sport,
			COALESCE(vs.total_reviews, 0) AS total_reviews,
			COALESCE(vs.average_rating, 0) AS average_rating
		FROM venues v
		LEFT JOIN venue_stats vs ON v.id = vs.venue_id
		%s
		ORDER BY v.created_at DESC
		LIMIT $%d OFFSET $%d
	`, whereSQL, limitPos, offsetPos)

	args2 := append([]interface{}{}, args...)
	args2 = append(args2, filter.Pagination.Limit, filter.Pagination.Offset)

	rows, err := r.db.Query(ctx, dataQ, args2...)
	if err != nil {
		return nil, fmt.Errorf("list venues: %w", err)
	}
	defer rows.Close()

	var out []VenueListing
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
			return nil, fmt.Errorf("scan venue row: %w", err)
		}

		if openTime.Valid {
			v.OpenTime = &openTime.String
		}
		out = append(out, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows venues: %w", err)
	}

	return &AdminVenueListResult{Venues: out, Total: total}, nil
}

func (r *Repository) GetImageURLs(ctx context.Context, venueID int64) ([]string, error) {
	query := `SELECT image_urls FROM venues WHERE id = $1`

	var urls []string
	if err := r.db.QueryRow(ctx, query, venueID).Scan(&urls); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("venue %d not found", venueID)
		}
		return nil, err
	}
	return urls, nil
}

// just get basic venueinfo without images and rating and rate count, much
// lighter than get venue details method.
func (r *Repository) GetVenueInfo(ctx context.Context, venueID int64) (*VenueInfo, error) {
	query := `SELECT id, name, address, ST_X(location::geometry) as longitude,
		ST_Y(location::geometry) as latitude, description, amenities, open_time, phone_number, status FROM venues WHERE venues.id = $1`

	var VenueInfo VenueInfo
	var longitude, latitude float64

	err := r.db.QueryRow(ctx, query, venueID).Scan(
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

// GetOwnerIDFromVenueID returns a OwnerID from the provided venueID
func (r *Repository) GetOwnerIDFromVenueID(ctx context.Context, venueID int64) (int64, error) {
	var ownerID int64
	err := r.db.QueryRow(ctx, `SELECT owner_id FROM venues WHERE id = $1`, venueID).Scan(&ownerID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return 0, fmt.Errorf("venue not found")
		}
		return 0, err
	}
	return ownerID, nil
}

// AddFavorite inserts a record into the favorite_venues table.
func (r *Repository) AddFavorite(ctx context.Context, userID, venueID int64) error {
	query := `
		INSERT INTO favorite_venues (user_id, venue_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := r.db.Exec(ctx, query, userID, venueID)
	if err != nil {
		return fmt.Errorf("failed to add favorite: %w", err)
	}
	return nil
}

// RemoveFavorite deletes a record from the favorite_venues table.
func (r *Repository) RemoveFavorite(ctx context.Context, userID, venueID int64) error {
	query := `
		DELETE FROM favorite_venues
		WHERE user_id = $1 AND venue_id = $2
	`
	_, err := r.db.Exec(ctx, query, userID, venueID)
	if err != nil {
		return fmt.Errorf("failed to remove favorite: %w", err)
	}
	return nil
}

// TODO: add phone number here
// GetFavoritesByUser returns all venues that a user has favorited.
// It performs a join between favorite_venues and venues.
func (r *Repository) GetFavoritesByUser(ctx context.Context, userID int64) ([]Venue, error) {
	query := `
		SELECT v.id, v.owner_id, v.name, v.address, v.description, v.amenities,
		       v.open_time, v.image_urls, v.sport, v.created_at, v.updated_at
		FROM venues v
		JOIN favorite_venues f ON v.id = f.venue_id
		WHERE f.user_id = $1
		ORDER BY f.created_at DESC
	`

	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get favorites: %w", err)
	}
	defer rows.Close()

	var favorites []Venue
	for rows.Next() {
		var v Venue

		// Scan the venue fields – be sure to match the order and types.
		if err := rows.Scan(
			&v.ID, &v.OwnerID, &v.Name, &v.Address, &v.Description,
			&v.Amenities, &v.OpenTime, &v.ImageURLs, &v.Sport,
			&v.CreatedAt, &v.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan venue row: %w", err)
		}

		favorites = append(favorites, v)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return favorites, nil
}

// GetFavoriteVenueIDsByUser returns the venue IDs a user has favorited.
func (r *Repository) GetFavoriteVenueIDsByUser(ctx context.Context, userID int64) (map[int64]struct{}, error) {
	query := `
        SELECT venue_id
        FROM favorite_venues
        WHERE user_id = $1
    `
	rows, err := r.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get favorite ids: %w", err)
	}
	defer rows.Close()

	favs := make(map[int64]struct{})
	for rows.Next() {
		var vid int64
		if err := rows.Scan(&vid); err != nil {
			return nil, fmt.Errorf("failed to scan favorite id: %w", err)
		}
		favs[vid] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return favs, nil
}

// 8 is limit you can change from data access layer
func (r *Repository) SearchVenues(ctx context.Context, query string) ([]VenueListing, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("search query is required")
	}

	const limit = 8

	sqlQuery := `
	WITH venue_stats AS (
		SELECT venue_id, COUNT(*) AS total_reviews, AVG(rating) AS average_rating
		FROM reviews
		GROUP BY venue_id
	)
	SELECT
		v.id,
		v.name,
		v.address,
		ST_X(v.location::geometry) AS longitude,
		ST_Y(v.location::geometry) AS latitude,
		v.image_urls,
		v.open_time,
		v.phone_number,
		v.sport,
		COALESCE(vs.total_reviews, 0) AS total_reviews,
		COALESCE(vs.average_rating, 0) AS average_rating
	FROM venues v
	LEFT JOIN venue_stats vs ON v.id = vs.venue_id
	WHERE v.status = 'active'
	  AND (
		v.name ILIKE '%' || $1 || '%'
		OR v.sport ILIKE '%' || $1 || '%'
	  )
	ORDER BY v.id DESC
	LIMIT $2;
	`

	rows, err := r.db.Query(ctx, sqlQuery, q, limit)
	if err != nil {
		return nil, fmt.Errorf("search venues: %w", err)
	}
	defer rows.Close()

	var out []VenueListing
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
			return nil, fmt.Errorf("scan venues: %w", err)
		}

		if openTime.Valid {
			v.OpenTime = &openTime.String
		}

		out = append(out, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows venues: %w", err)
	}

	return out, nil
}

// 8 is limit you can change from data access layer
func (r *Repository) FullTextSearchVenues(ctx context.Context, query string) ([]VenueListingWithRank, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, fmt.Errorf("search query is required")
	}

	const limit = 8

	sqlQuery := `
	WITH venue_stats AS (
		SELECT venue_id, COUNT(*) AS total_reviews, AVG(rating) AS average_rating
		FROM reviews
		GROUP BY venue_id
	),
	ranked AS (
		SELECT
			v.id,
			v.name,
			v.address,
			v.location,
			v.image_urls,
			v.open_time,
			v.phone_number,
			v.sport,
			ts_rank_cd(v.fts, plainto_tsquery('english', $1)) AS rank
		FROM venues v
		WHERE v.status = 'active'
		  AND v.fts @@ plainto_tsquery('english', $1)
	)
	SELECT
		r.id,
		r.name,
		r.address,
		ST_X(r.location::geometry) AS longitude,
		ST_Y(r.location::geometry) AS latitude,
		r.image_urls,
		r.open_time,
		r.phone_number,
		r.sport,
		COALESCE(vs.total_reviews, 0) AS total_reviews,
		COALESCE(vs.average_rating, 0) AS average_rating,
		r.rank
	FROM ranked r
	LEFT JOIN venue_stats vs ON r.id = vs.venue_id
	ORDER BY r.rank DESC
	LIMIT $2;
	`

	rows, err := r.db.Query(ctx, sqlQuery, q, limit)
	if err != nil {
		return nil, fmt.Errorf("fts search venues: %w", err)
	}
	defer rows.Close()

	var out []VenueListingWithRank
	for rows.Next() {
		var v VenueListingWithRank
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
			&v.Rank,
		); err != nil {
			return nil, fmt.Errorf("scan venues fts: %w", err)
		}

		if openTime.Valid {
			v.OpenTime = &openTime.String
		}

		out = append(out, v)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows venues fts: %w", err)
	}

	return out, nil
}

func (r *Repository) UpdateVenueStatusOwner(
	ctx context.Context,
	venueID int64,
	ownerID int64,
	nextStatus string,
) error {
	nextStatus = strings.TrimSpace(nextStatus)

	// ✅ Owner is only allowed requested <-> active
	if nextStatus != string(VenueStatusRequested) && nextStatus != string(VenueStatusActive) {
		return fmt.Errorf("invalid status transition")
	}

	/**
	 * ✅ Enforce transitions at SQL level:
	 * - requested -> active
	 * - active -> requested
	 * ✅ Ensure owner_id matches (only the owner can mutate).
	 * ✅ CAST: text -> venue_status
	 */
	q := `
		UPDATE venues
		SET status = $1::venue_status,
		    updated_at = NOW()
		WHERE id = $2
		  AND owner_id = $3
		  AND (
				($1::venue_status = 'active'::venue_status     AND status = 'requested'::venue_status)
			OR  ($1::venue_status = 'requested'::venue_status  AND status = 'active'::venue_status)
		  )
	`

	ct, err := r.db.Exec(ctx, q, nextStatus, venueID, ownerID)
	if err != nil {
		return fmt.Errorf("update venue status: %w", err)
	}

	if ct.RowsAffected() == 0 {
		// Could be: venue not found, not owner, or invalid transition
		return fmt.Errorf("status change not allowed")
	}

	return nil
}

package facilities

import (
	"context"
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

func scanFacility(row pgx.Row) (*Facility, error) {
	var f Facility

	err := row.Scan(
		&f.ID,
		&f.VenueID,
		&f.Name,
		&f.Description,
		&f.Sport,
		&f.SurfaceType,
		&f.Capacity,
		&f.ImageURLs,
		&f.IsDefault,
		&f.IsActive,
		&f.CreatedAt,
		&f.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFacilityNotFound
		}
		return nil, err
	}

	return &f, nil
}

func (r *Repository) Create(ctx context.Context, input CreateFacilityInput) (*Facility, error) {
	const query = `
		INSERT INTO facilities (
			venue_id,
			name,
			description,
			sport,
			surface_type,
			capacity,
			image_urls,
			is_default,
			is_active
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,TRUE)
		RETURNING
			id,
			venue_id,
			name,
			description,
			sport,
			surface_type,
			capacity,
			image_urls,
			is_default,
			is_active,
			created_at,
			updated_at
	`

	return scanFacility(r.db.QueryRow(
		ctx,
		query,
		input.VenueID,
		input.Name,
		input.Description,
		input.Sport,
		input.SurfaceType,
		input.Capacity,
		input.ImageURLs,
		input.IsDefault,
	))
}

func (r *Repository) GetByID(ctx context.Context, venueID, facilityID int64) (*Facility, error) {
	const query = `
		SELECT
			id,
			venue_id,
			name,
			description,
			sport,
			surface_type,
			capacity,
			image_urls,
			is_default,
			is_active,
			created_at,
			updated_at
		FROM facilities
		WHERE id = $1
		  AND venue_id = $2
	`

	return scanFacility(r.db.QueryRow(ctx, query, facilityID, venueID))
}

func (r *Repository) GetDefaultByVenueID(ctx context.Context, venueID int64) (*Facility, error) {
	const query = `
		SELECT
			id,
			venue_id,
			name,
			description,
			sport,
			surface_type,
			capacity,
			image_urls,
			is_default,
			is_active,
			created_at,
			updated_at
		FROM facilities
		WHERE venue_id = $1
		  AND is_default = TRUE
		LIMIT 1
	`

	return scanFacility(r.db.QueryRow(ctx, query, venueID))
}

func (r *Repository) ListByVenueID(ctx context.Context, venueID int64) ([]Facility, error) {
	const query = `
		SELECT
			id,
			venue_id,
			name,
			description,
			sport,
			surface_type,
			capacity,
			image_urls,
			is_default,
			is_active,
			created_at,
			updated_at
		FROM facilities
		WHERE venue_id = $1
		ORDER BY is_default DESC, id ASC
	`

	rows, err := r.db.Query(ctx, query, venueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []Facility

	for rows.Next() {
		var f Facility

		if err := rows.Scan(
			&f.ID,
			&f.VenueID,
			&f.Name,
			&f.Description,
			&f.Sport,
			&f.SurfaceType,
			&f.Capacity,
			&f.ImageURLs,
			&f.IsDefault,
			&f.IsActive,
			&f.CreatedAt,
			&f.UpdatedAt,
		); err != nil {
			return nil, err
		}

		result = append(result, f)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return result, nil
}

func (r *Repository) Update(ctx context.Context, venueID, facilityID int64, input UpdateFacilityInput) (*Facility, error) {
	set := make([]string, 0)
	args := make([]any, 0)
	arg := 1

	if input.Name != nil {
		set = append(set, fmt.Sprintf("name = $%d", arg))
		args = append(args, strings.TrimSpace(*input.Name))
		arg++
	}

	if input.Description != nil {
		set = append(set, fmt.Sprintf("description = $%d", arg))
		args = append(args, input.Description)
		arg++
	}

	if input.Sport != nil {
		set = append(set, fmt.Sprintf("sport = $%d", arg))
		args = append(args, input.Sport)
		arg++
	}

	if input.SurfaceType != nil {
		set = append(set, fmt.Sprintf("surface_type = $%d", arg))
		args = append(args, input.SurfaceType)
		arg++
	}

	if input.Capacity != nil {
		set = append(set, fmt.Sprintf("capacity = $%d", arg))
		args = append(args, input.Capacity)
		arg++
	}

	if input.ImageURLs != nil {
		set = append(set, fmt.Sprintf("image_urls = $%d", arg))
		args = append(args, input.ImageURLs)
		arg++
	}

	if input.IsActive != nil {
		set = append(set, fmt.Sprintf("is_active = $%d", arg))
		args = append(args, *input.IsActive)
		arg++
	}

	if input.IsDefault != nil {
		set = append(set, fmt.Sprintf("is_default = $%d", arg))
		args = append(args, *input.IsDefault)
		arg++
	}

	if len(set) == 0 {
		return r.GetByID(ctx, venueID, facilityID)
	}

	set = append(set, "updated_at = NOW()")

	query := fmt.Sprintf(`
		UPDATE facilities
		SET %s
		WHERE id = $%d
		  AND venue_id = $%d
		RETURNING
			id,
			venue_id,
			name,
			description,
			sport,
			surface_type,
			capacity,
			image_urls,
			is_default,
			is_active,
			created_at,
			updated_at
	`, strings.Join(set, ", "), arg, arg+1)

	args = append(args, facilityID, venueID)

	return scanFacility(r.db.QueryRow(ctx, query, args...))
}

func (r *Repository) Delete(ctx context.Context, venueID, facilityID int64) error {
	const query = `
		DELETE FROM facilities
		WHERE id = $1
		  AND venue_id = $2
		  AND is_default = FALSE
	`

	res, err := r.db.Exec(ctx, query, facilityID, venueID)
	if err != nil {
		return err
	}

	if res.RowsAffected() == 0 {
		return ErrFacilityNotFound
	}

	return nil
}

func (r *Repository) BelongsToVenue(ctx context.Context, venueID, facilityID int64) (bool, error) {
	const query = `
		SELECT EXISTS (
			SELECT 1
			FROM facilities
			WHERE id = $1
			  AND venue_id = $2
			  AND is_active = TRUE
		)
	`

	var ok bool
	if err := r.db.QueryRow(ctx, query, facilityID, venueID).Scan(&ok); err != nil {
		return false, err
	}

	return ok, nil
}

func (r *Repository) SetDefault(ctx context.Context, venueID, facilityID int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	// First make sure the facility exists, belongs to this venue, and is active.
	// We do this before unsetting the old default so we do not accidentally leave
	// the venue with no default facility if the new facility is invalid.
	var exists bool
	err = tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM facilities
			WHERE id = $1
			  AND venue_id = $2
			  AND is_active = TRUE
		)
	`, facilityID, venueID).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check facility exists: %w", err)
	}

	if !exists {
		return ErrFacilityNotFound
	}

	// Unset the current default facility for this venue, if one exists.
	_, err = tx.Exec(ctx, `
		UPDATE facilities
		SET is_default = FALSE,
		    updated_at = NOW()
		WHERE venue_id = $1
		  AND is_default = TRUE
	`, venueID)
	if err != nil {
		return fmt.Errorf("unset current default facility: %w", err)
	}

	// Set the selected active facility as the new default.
	tag, err := tx.Exec(ctx, `
		UPDATE facilities
		SET is_default = TRUE,
		    updated_at = NOW()
		WHERE id = $1
		  AND venue_id = $2
		  AND is_active = TRUE
	`, facilityID, venueID)
	if err != nil {
		return fmt.Errorf("set new default facility: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrFacilityNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit set default facility: %w", err)
	}

	return nil
}

// RemovePhotoURL removes one specific photo URL from a facility's image_urls array.
//
// It only removes the photo from the facility that belongs to the given venue.
// This is safer because it checks both venue_id and facility_id.
//
// Example:
// facilityID = 10
// photoURL = "https://example.com/image1.jpg"
//
// Before:
// image_urls = ["image1.jpg", "image2.jpg"]
//
// After:
// image_urls = ["image2.jpg"]
func (r *Repository) RemovePhotoURL(ctx context.Context, venueID, facilityID int64, photoURL string) error {
	query := `
		UPDATE facilities
		SET
			image_urls = array_remove(image_urls, $1),
			updated_at = NOW()
		WHERE venue_id = $2
		  AND id = $3
	`

	result, err := r.db.Exec(ctx, query, photoURL, venueID, facilityID)
	if err != nil {
		return fmt.Errorf("failed to remove facility photo URL: %w", err)
	}

	// If no row was updated, it means the facility does not exist
	// or it does not belong to this venue.
	if result.RowsAffected() == 0 {
		return ErrFacilityNotFound
	}

	return nil
}

func (r *Repository) GetImageURLs(ctx context.Context, venueID, facilityID int64) ([]string, error) {
	const query = `
		SELECT image_urls
		FROM facilities
		WHERE venue_id = $1
		  AND id = $2
	`

	var urls []string

	err := r.db.QueryRow(ctx, query, venueID, facilityID).Scan(&urls)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrFacilityNotFound
		}
		return nil, fmt.Errorf("failed to get facility image URLs: %w", err)
	}

	return urls, nil
}

func (r *Repository) AddPhotoURL(ctx context.Context, venueID, facilityID int64, photoURL string) error {
	const query = `
		UPDATE facilities
		SET
			image_urls = array_append(COALESCE(image_urls, '{}'), $1),
			updated_at = NOW()
		WHERE venue_id = $2
		  AND id = $3
	`

	result, err := r.db.Exec(ctx, query, photoURL, venueID, facilityID)
	if err != nil {
		return fmt.Errorf("failed to add facility photo URL: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrFacilityNotFound
	}

	return nil
}

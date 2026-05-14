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

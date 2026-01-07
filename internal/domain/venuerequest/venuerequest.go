package venuerequest

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) RequestStore {
	return &Repository{db: db}
}

func (r *Repository) CreateRequest(ctx context.Context, in *CreateVenueRequestInput) (*VenueRequest, error) {
	if len(in.Location) != 2 {
		return nil, fmt.Errorf("location must be [lon, lat]")
	}

	const q = `
		INSERT INTO venue_requests (
			name, address, location,
			description, amenities, open_time,
			sport, phone_number,
			requester_ip, requester_user_agent
		) VALUES (
			$1, $2, ST_SetSRID(ST_MakePoint($3, $4), 4326),
			$5, $6, $7,
			$8, $9,
			$10, $11
		)
		RETURNING
			id, status, created_at, updated_at
	`

	vr := &VenueRequest{
		Name:               in.Name,
		Address:            in.Address,
		Location:           in.Location,
		Description:        in.Description,
		Amenities:          in.Amenities,
		OpenTime:           in.OpenTime,
		Sport:              in.Sport,
		PhoneNumber:        in.PhoneNumber,
		RequesterIP:        in.RequesterIP,
		RequesterUserAgent: in.RequesterUserAgent,
	}

	err := r.db.QueryRow(ctx, q,
		in.Name,
		in.Address,
		in.Location[0], // lon
		in.Location[1], // lat
		in.Description,
		in.Amenities,
		in.OpenTime,
		in.Sport,
		in.PhoneNumber,
		in.RequesterIP,
		in.RequesterUserAgent,
	).Scan(
		&vr.ID,
		&vr.Status,
		&vr.CreatedAt,
		&vr.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("create venue_request: %w", err)
	}

	return vr, nil
}

func (r *Repository) GetRequestByID(ctx context.Context, requestID int64) (*VenueRequest, error) {
	const q = `
		SELECT
			id,
			name,
			address,
			ST_X(location::geometry) AS longitude,
			ST_Y(location::geometry) AS latitude,
			description,
			amenities,
			open_time,
			sport,
			phone_number,
			status,
			admin_note,
			requester_ip,
			requester_user_agent,
			created_at,
			updated_at,
			approved_at,
			approved_by,
			rejected_at,
			rejected_by
		FROM venue_requests
		WHERE id = $1
	`

	var (
		vr         VenueRequest
		lon        float64
		lat        float64
		adminNote  sql.NullString
		reqIP      sql.NullString
		reqUA      sql.NullString
		approvedAt sql.NullTime
		approvedBy sql.NullInt64
		rejectedAt sql.NullTime
		rejectedBy sql.NullInt64
	)

	err := r.db.QueryRow(ctx, q, requestID).Scan(
		&vr.ID,
		&vr.Name,
		&vr.Address,
		&lon,
		&lat,
		&vr.Description,
		&vr.Amenities,
		&vr.OpenTime,
		&vr.Sport,
		&vr.PhoneNumber,
		&vr.Status,
		&adminNote,
		&reqIP,
		&reqUA,
		&vr.CreatedAt,
		&vr.UpdatedAt,
		&approvedAt,
		&approvedBy,
		&rejectedAt,
		&rejectedBy,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrVenueRequestNotFound
		}
		return nil, fmt.Errorf("get venue_request: %w", err)
	}

	vr.Location = []float64{lon, lat}

	if adminNote.Valid {
		vr.AdminNote = &adminNote.String
	}
	if reqIP.Valid {
		vr.RequesterIP = &reqIP.String
	}
	if reqUA.Valid {
		vr.RequesterUserAgent = &reqUA.String
	}
	if approvedAt.Valid {
		t := approvedAt.Time
		vr.ApprovedAt = &t
	}
	if approvedBy.Valid {
		id := approvedBy.Int64
		vr.ApprovedBy = &id
	}
	if rejectedAt.Valid {
		t := rejectedAt.Time
		vr.RejectedAt = &t
	}
	if rejectedBy.Valid {
		id := rejectedBy.Int64
		vr.RejectedBy = &id
	}

	return &vr, nil
}

func (r *Repository) ListRequests(ctx context.Context, filter VenueRequestFilter) ([]VenueRequest, error) {
	if filter.Page <= 0 {
		filter.Page = 1
	}
	if filter.Limit <= 0 || filter.Limit > 60 {
		filter.Limit = 20
	}

	where := []string{"1=1"}
	args := []interface{}{}
	arg := 1

	if filter.Status != nil {
		where = append(where, fmt.Sprintf("status = $%d", arg))
		args = append(args, string(*filter.Status))
		arg++
	}

	// pagination
	limitPos := arg
	offsetPos := arg + 1
	args = append(args, filter.Limit, (filter.Page-1)*filter.Limit)

	q := fmt.Sprintf(`
		SELECT
			id,
			name,
			address,
			ST_X(location::geometry) AS longitude,
			ST_Y(location::geometry) AS latitude,
			description,
			amenities,
			open_time,
			sport,
			phone_number,
			status,
			admin_note,
			created_at,
			updated_at
		FROM venue_requests
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d
	`, strings.Join(where, " AND "), limitPos, offsetPos)

	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list venue_requests: %w", err)
	}
	defer rows.Close()

	var out []VenueRequest
	for rows.Next() {
		var vr VenueRequest
		var lon, lat float64
		var adminNote sql.NullString

		if err := rows.Scan(
			&vr.ID,
			&vr.Name,
			&vr.Address,
			&lon,
			&lat,
			&vr.Description,
			&vr.Amenities,
			&vr.OpenTime,
			&vr.Sport,
			&vr.PhoneNumber,
			&vr.Status,
			&adminNote,
			&vr.CreatedAt,
			&vr.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan venue_requests: %w", err)
		}

		vr.Location = []float64{lon, lat}
		if adminNote.Valid {
			vr.AdminNote = &adminNote.String
		}

		out = append(out, vr)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows venue_requests: %w", err)
	}

	return out, nil
}

func (r *Repository) MarkRequestApproved(ctx context.Context, requestID int64, approvedBy int64, adminNote *string) error {
	now := time.Now()

	const q = `
		UPDATE venue_requests
		SET status = 'approved',
		    admin_note = $1,
		    approved_at = $2,
		    approved_by = $3,
		    updated_at = NOW()
		WHERE id = $4
	`
	ct, err := r.db.Exec(ctx, q, adminNote, now, approvedBy, requestID)
	if err != nil {
		return fmt.Errorf("approve venue_request: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrVenueRequestNotFound
	}
	return nil
}

func (r *Repository) MarkRequestRejected(ctx context.Context, requestID int64, rejectedBy int64, adminNote *string) error {
	now := time.Now()

	const q = `
		UPDATE venue_requests
		SET status = 'rejected',
		    admin_note = $1,
		    rejected_at = $2,
		    rejected_by = $3,
		    updated_at = NOW()
		WHERE id = $4
	`
	ct, err := r.db.Exec(ctx, q, adminNote, now, rejectedBy, requestID)
	if err != nil {
		return fmt.Errorf("reject venue_request: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return ErrVenueRequestNotFound
	}
	return nil
}

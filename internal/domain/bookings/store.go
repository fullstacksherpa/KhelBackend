package bookings

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/database"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	GetBookingOwner(ctx context.Context, venueID, bookingID int64) (int64, error)

	GetPricingSlots(ctx context.Context, venueID, facilityID int64, dayOfWeek string) ([]PricingSlot, error)
	CreatePricingSlotsBatch(ctx context.Context, slots []*PricingSlot) error
	UpdatePricing(ctx context.Context, p *PricingSlot) error
	DeletePricingSlot(ctx context.Context, venueID, facilityID, pricingID int64) error

	GetBookingsForDate(ctx context.Context, venueID, facilityID int64, date time.Time) ([]Interval, error)
	CreateBooking(ctx context.Context, booking *Booking) (int64, error)
	GetBookingByID(ctx context.Context, bookingID int64) (*Booking, error)

	GetPendingBookingsForVenueDate(ctx context.Context, venueID, facilityID int64, date time.Time) ([]PendingBooking, error)
	GetCanceledBookingsForVenueDate(ctx context.Context, venueID, facilityID int64, date time.Time) ([]CanceledBooking, error)
	GetScheduledBookingsForVenueDate(ctx context.Context, venueID, facilityID int64, date time.Time) ([]ScheduledBooking, error)

	UpdateBookingStatus(ctx context.Context, venueID, bookingID int64, status string) error
	AcceptBooking(ctx context.Context, venueID, bookingID int64) error
	RejectBooking(ctx context.Context, venueID, bookingID int64) error
	CancelBooking(ctx context.Context, venueID, bookingID int64) error

	GetBookingsByUser(ctx context.Context, userID int64, filter BookingFilter) ([]UserBooking, error)
	GetVenueOwnerIDFromBookingID(ctx context.Context, bookingID int64) (int64, error)

	CloseBooking(ctx context.Context, venueID int64, bookingID int64, method string, paidAmount int, finalAmount int) error
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

func (r *Repository) CloseBooking(
	ctx context.Context,
	venueID int64,
	bookingID int64,
	method string,
	paidAmount int,
	finalAmount int,
) error {
	query := `
		UPDATE bookings
		SET
			status = 'done',
			payment_method = $1,
			paid_amount = $2,
			final_amount = $3,
			paid_at = NOW(),
			updated_at = NOW()
		WHERE id = $4
		  AND venue_id = $5
		  AND status = 'confirmed'
	`

	result, err := r.db.Exec(
		ctx,
		query,
		method,
		paidAmount,
		finalAmount,
		bookingID,
		venueID,
	)
	if err != nil {
		return fmt.Errorf("close booking: %w", err)
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("booking not found or already closed")
	}

	return nil
}

func (r *Repository) GetBookingOwner(ctx context.Context, venueID, bookingID int64) (int64, error) {
	const query = `SELECT user_id FROM bookings WHERE id = $1 AND venue_id = $2`
	var userID int64
	err := r.db.QueryRow(ctx, query, bookingID, venueID).Scan(&userID)
	if err != nil {
		return 0, err
	}
	return userID, nil
}

func (r *Repository) GetPricingSlots(ctx context.Context, venueID, facilityID int64, dayOfWeek string) ([]PricingSlot, error) {
	query := `
		SELECT
			id,
			venue_id,
			facility_id,
			day_of_week,
			start_time,
			end_time,
			price
		FROM venue_pricing
		WHERE venue_id = $1
		  AND facility_id = $2
	`

	args := []any{venueID, facilityID}

	if strings.TrimSpace(dayOfWeek) != "" {
		query += ` AND day_of_week = $3`
		args = append(args, strings.ToLower(strings.TrimSpace(dayOfWeek)))
	}

	query += ` ORDER BY day_of_week, start_time`

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slots []PricingSlot

	for rows.Next() {
		var ps PricingSlot

		if err := rows.Scan(
			&ps.ID,
			&ps.VenueID,
			&ps.FacilityID,
			&ps.DayOfWeek,
			&ps.StartTime,
			&ps.EndTime,
			&ps.Price,
		); err != nil {
			return nil, err
		}

		slots = append(slots, ps)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return slots, nil
}

func (r *Repository) GetBookingsForDate(ctx context.Context, venueID, facilityID int64, date time.Time) ([]Interval, error) {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return nil, fmt.Errorf("failed to load Kathmandu timezone: %w", err)
	}

	localDate := date.In(loc)
	startOfDayLocal := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, loc)
	endOfDayLocal := startOfDayLocal.Add(24 * time.Hour)

	startOfDayUTC := startOfDayLocal.UTC()
	endOfDayUTC := endOfDayLocal.UTC()

	query := `
		SELECT start_time, end_time
		FROM bookings
		WHERE venue_id = $1
		  AND facility_id = $2
		  AND status IN ('pending', 'confirmed')
		  AND start_time < $3
		  AND end_time > $4
		ORDER BY start_time
	`

	rows, err := r.db.Query(ctx, query, venueID, facilityID, endOfDayUTC, startOfDayUTC)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var intervals []Interval

	for rows.Next() {
		var start, end time.Time

		if err := rows.Scan(&start, &end); err != nil {
			return nil, err
		}

		intervals = append(intervals, Interval{
			Start: start,
			End:   end,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return intervals, nil
}

// CreateBooking inserts a booking record into the database.
func (r *Repository) CreateBooking(ctx context.Context, booking *Booking) (int64, error) {
	query := `
		INSERT INTO bookings (
			venue_id,
			facility_id,
			user_id,
			start_time,
			end_time,
			total_price,
			status,
			customer_name,
			customer_phone,
			note
		)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		RETURNING id, created_at, updated_at
	`

	err := r.db.QueryRow(
		ctx,
		query,
		booking.VenueID,
		booking.FacilityID,
		booking.UserID,
		booking.StartTime,
		booking.EndTime,
		booking.TotalPrice,
		booking.Status,
		booking.CustomerName,
		booking.CustomerPhone,
		booking.Note,
	).Scan(&booking.ID, &booking.CreatedAt, &booking.UpdatedAt)

	if err != nil {
		return 0, err
	}

	return booking.ID, nil
}

// UpdatePricing updates a pricing slot in the database.
func (r *Repository) UpdatePricing(ctx context.Context, p *PricingSlot) error {
	query := `
		UPDATE venue_pricing
		SET
			day_of_week = $1,
			start_time = $2,
			end_time = $3,
			price = $4
		WHERE id = $5
		  AND venue_id = $6
		  AND facility_id = $7
	`

	result, err := r.db.Exec(
		ctx,
		query,
		p.DayOfWeek,
		p.StartTime.Format("15:04:05"),
		p.EndTime.Format("15:04:05"),
		p.Price,
		p.ID,
		p.VenueID,
		p.FacilityID,
	)
	if err != nil {
		return err
	}

	if result.RowsAffected() == 0 {
		return fmt.Errorf("pricing slot not found")
	}

	return nil
}

// CreatePricingSlotsBatch uses pgx.Batch to insert multiple pricing slots in one round-trip.
func (r *Repository) CreatePricingSlotsBatch(ctx context.Context, slots []*PricingSlot) error {
	return database.WithTx(r.db, ctx, func(tx pgx.Tx) error {
		const sql = `
			INSERT INTO venue_pricing (
				venue_id,
				facility_id,
				day_of_week,
				start_time,
				end_time,
				price
			)
			VALUES ($1,$2,$3,$4,$5,$6)
			RETURNING id
		`

		batch := &pgx.Batch{}

		for _, slot := range slots {
			batch.Queue(
				sql,
				slot.VenueID,
				slot.FacilityID,
				slot.DayOfWeek,
				slot.StartTime.Format("15:04:05"),
				slot.EndTime.Format("15:04:05"),
				slot.Price,
			)
		}

		br := tx.SendBatch(ctx, batch)
		defer br.Close()

		for i, slot := range slots {
			if err := br.QueryRow().Scan(&slot.ID); err != nil {
				return fmt.Errorf("batch insert pricing slot[%d]: %w", i, err)
			}
		}

		return nil
	})
}

func (r *Repository) DeletePricingSlot(ctx context.Context, venueID, facilityID, pricingID int64) error {
	const q = `
		DELETE FROM venue_pricing
		WHERE id = $1
		  AND venue_id = $2
		  AND facility_id = $3
	`

	res, err := r.db.Exec(ctx, q, pricingID, venueID, facilityID)
	if err != nil {
		return fmt.Errorf("failed to delete pricing slot: %w", err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("no pricing slot found with id=%d for venue_id=%d facility_id=%d", pricingID, venueID, facilityID)
	}

	return nil
}

// GetBookingByID retrieves a single booking by its ID
func (r *Repository) GetBookingByID(ctx context.Context, bookingID int64) (*Booking, error) {
	const query = `
		SELECT
			id,
			venue_id,
			facility_id,
			user_id,
			start_time,
			end_time,
			total_price,
			status,
			created_at,
			updated_at,
			customer_name,
			customer_phone,
			note
		FROM bookings
		WHERE id = $1
	`

	var booking Booking

	err := r.db.QueryRow(ctx, query, bookingID).Scan(
		&booking.ID,
		&booking.VenueID,
		&booking.FacilityID,
		&booking.UserID,
		&booking.StartTime,
		&booking.EndTime,
		&booking.TotalPrice,
		&booking.Status,
		&booking.CreatedAt,
		&booking.UpdatedAt,
		&booking.CustomerName,
		&booking.CustomerPhone,
		&booking.Note,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("booking not found")
		}
		return nil, fmt.Errorf("failed to get booking: %w", err)
	}

	return &booking, nil
}

func (r *Repository) GetVenueOwnerIDFromBookingID(ctx context.Context, bookingID int64) (int64, error) {
	const query = `
		SELECT v.owner_id
		FROM bookings b
		JOIN facilities f ON f.id = b.facility_id
		JOIN venues v ON v.id = f.venue_id
		WHERE b.id = $1
	`

	var ownerID int64

	err := r.db.QueryRow(ctx, query, bookingID).Scan(&ownerID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, fmt.Errorf("booking not found or has no associated venue")
		}
		return 0, fmt.Errorf("failed to get venue owner: %w", err)
	}

	return ownerID, nil
}

// GetPendingBookingsForVenueDate returns pending booking requests for one facility on a specific date.
//
// Facility-level filtering is required because one venue can now have multiple facilities.
// Example:
//   - Venue: ABC Sports Center
//   - Facility 1: Ground A
//   - Facility 2: Ground B
//
// Pending bookings for Ground A should not appear inside Ground B's schedule.
func (r *Repository) GetPendingBookingsForVenueDate(
	ctx context.Context,
	venueID int64,
	facilityID int64,
	date time.Time,
) ([]PendingBooking, error) {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return nil, fmt.Errorf("failed to load Kathmandu timezone: %w", err)
	}

	localDate := date.In(loc)

	startOfDayLocal := time.Date(
		localDate.Year(),
		localDate.Month(),
		localDate.Day(),
		0, 0, 0, 0,
		loc,
	)

	endOfDayLocal := startOfDayLocal.Add(24 * time.Hour)

	startOfDayUTC := startOfDayLocal.UTC()
	endOfDayUTC := endOfDayLocal.UTC()

	query := `
		SELECT
			b.id,
			b.user_id,
			COALESCE(u.first_name, '') AS user_name,
			u.profile_picture_url,
			COALESCE(u.phone, '') AS user_phone,
			b.total_price,
			b.created_at,
			b.start_time,
			b.end_time
		FROM bookings b
		JOIN users u ON u.id = b.user_id
		WHERE b.venue_id = $1
		  AND b.facility_id = $2
		  AND b.status = 'pending'
		  AND b.start_time >= $3
		  AND b.start_time < $4
		ORDER BY b.created_at ASC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		venueID,
		facilityID,
		startOfDayUTC,
		endOfDayUTC,
	)
	if err != nil {
		return nil, fmt.Errorf("get pending bookings: %w", err)
	}
	defer rows.Close()

	var pendingBookings []PendingBooking

	for rows.Next() {
		var b PendingBooking

		if err := rows.Scan(
			&b.BookingID,
			&b.UserID,
			&b.UserName,
			&b.UserImageURL,
			&b.UserPhone,
			&b.Price,
			&b.RequestedAt,
			&b.StartTime,
			&b.EndTime,
		); err != nil {
			return nil, fmt.Errorf("scan pending booking: %w", err)
		}

		pendingBookings = append(pendingBookings, b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending bookings: %w", err)
	}

	return pendingBookings, nil
}

// GetCanceledBookingsForVenueDate returns canceled bookings for one facility on a specific date.
//
// Important:
// Before the refactor, bookings were filtered only by venue_id.
// After moving to Venue -> Facilities -> Bookings, we must filter by both:
//   - venue_id: keeps the booking inside the correct venue
//   - facility_id: keeps the booking inside the exact ground/court/facility
//
// Without facility_id, canceled bookings from Ground A and Ground B would be mixed together.
func (r *Repository) GetCanceledBookingsForVenueDate(
	ctx context.Context,
	venueID int64,
	facilityID int64,
	date time.Time,
) ([]CanceledBooking, error) {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return nil, fmt.Errorf("failed to load Kathmandu timezone: %w", err)
	}

	// Convert requested date to Nepal local day boundary.
	// This avoids bugs where UTC date and Nepal date are different.
	localDate := date.In(loc)

	startOfDayLocal := time.Date(
		localDate.Year(),
		localDate.Month(),
		localDate.Day(),
		0, 0, 0, 0,
		loc,
	)

	endOfDayLocal := startOfDayLocal.Add(24 * time.Hour)

	// DB stores timestamptz, so compare using UTC boundaries.
	startOfDayUTC := startOfDayLocal.UTC()
	endOfDayUTC := endOfDayLocal.UTC()

	query := `
		SELECT
			b.id,
			b.user_id,
			COALESCE(u.first_name, '') AS user_name,
			u.profile_picture_url,
			COALESCE(u.phone, '') AS user_phone,
			b.total_price,
			b.created_at,
			b.start_time,
			b.end_time
		FROM bookings b
		JOIN users u ON u.id = b.user_id
		WHERE b.venue_id = $1
		  AND b.facility_id = $2
		  AND b.status = 'canceled'
		  AND b.start_time >= $3
		  AND b.start_time < $4
		ORDER BY b.start_time ASC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		venueID,
		facilityID,
		startOfDayUTC,
		endOfDayUTC,
	)
	if err != nil {
		return nil, fmt.Errorf("get canceled bookings: %w", err)
	}
	defer rows.Close()

	var canceledBookings []CanceledBooking

	for rows.Next() {
		var b CanceledBooking

		if err := rows.Scan(
			&b.BookingID,
			&b.UserID,
			&b.UserName,
			&b.UserImageURL,
			&b.UserPhone,
			&b.Price,
			&b.RequestedAt,
			&b.StartTime,
			&b.EndTime,
		); err != nil {
			return nil, fmt.Errorf("scan canceled booking: %w", err)
		}

		canceledBookings = append(canceledBookings, b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate canceled bookings: %w", err)
	}

	return canceledBookings, nil
}

// GetScheduledBookingsForVenueDate returns confirmed bookings for one facility on a specific date.
//
// Scheduled means accepted/confirmed booking.
// We filter by venue_id and facility_id so the venue owner can manage each ground/court separately.
func (r *Repository) GetScheduledBookingsForVenueDate(
	ctx context.Context,
	venueID int64,
	facilityID int64,
	date time.Time,
) ([]ScheduledBooking, error) {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return nil, fmt.Errorf("failed to load Kathmandu timezone: %w", err)
	}

	localDate := date.In(loc)

	startOfDayLocal := time.Date(
		localDate.Year(),
		localDate.Month(),
		localDate.Day(),
		0, 0, 0, 0,
		loc,
	)

	endOfDayLocal := startOfDayLocal.Add(24 * time.Hour)

	startOfDayUTC := startOfDayLocal.UTC()
	endOfDayUTC := endOfDayLocal.UTC()

	query := `
		SELECT
			b.id,
			b.user_id,
			COALESCE(u.first_name, '') AS user_name,
			u.profile_picture_url,
			COALESCE(u.phone, '') AS user_phone,
			b.total_price,
			b.updated_at,
			b.start_time,
			b.end_time,
			b.customer_name,
			b.customer_phone,
			b.note
		FROM bookings b
		JOIN users u ON u.id = b.user_id
		WHERE b.venue_id = $1
		  AND b.facility_id = $2
		  AND b.status = 'confirmed'
		  AND b.start_time >= $3
		  AND b.start_time < $4
		ORDER BY b.start_time ASC
	`

	rows, err := r.db.Query(
		ctx,
		query,
		venueID,
		facilityID,
		startOfDayUTC,
		endOfDayUTC,
	)
	if err != nil {
		return nil, fmt.Errorf("get scheduled bookings: %w", err)
	}
	defer rows.Close()

	var scheduledBookings []ScheduledBooking

	for rows.Next() {
		var b ScheduledBooking

		if err := rows.Scan(
			&b.BookingID,
			&b.UserID,
			&b.UserName,
			&b.UserImageURL,
			&b.UserPhone,
			&b.Price,
			&b.AcceptedAt,
			&b.StartTime,
			&b.EndTime,
			&b.CustomerName,
			&b.CustomerPhone,
			&b.Note,
		); err != nil {
			return nil, fmt.Errorf("scan scheduled booking: %w", err)
		}

		scheduledBookings = append(scheduledBookings, b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate scheduled bookings: %w", err)
	}

	return scheduledBookings, nil
}

// UpdateBookingStatus sets a new status ("confirmed", "rejected", etc.) on a booking.
func (r *Repository) UpdateBookingStatus(ctx context.Context, venueID, bookingID int64, status string) error {
	const q = `
      UPDATE bookings
      SET status    = $1,
          updated_at = NOW()
      WHERE id       = $2
        AND venue_id = $3
    `
	res, err := r.db.Exec(ctx, q, status, bookingID, venueID)
	if err != nil {
		return err
	}
	rows := res.RowsAffected()

	if rows == 0 {
		return fmt.Errorf("failed to update booking status for bookingID=%d and venueID=%d", bookingID, venueID)
	}
	return nil
}

// AcceptBooking marks a pending booking as confirmed.
func (r *Repository) AcceptBooking(ctx context.Context, venueID, bookingID int64) error {
	return r.UpdateBookingStatus(ctx, venueID, bookingID, "confirmed")
}

// RejectBooking marks a pending booking as rejected.
func (r *Repository) RejectBooking(ctx context.Context, venueID, bookingID int64) error {
	return r.UpdateBookingStatus(ctx, venueID, bookingID, "rejected")
}

// RejectBooking marks a pending booking as rejected.
func (r *Repository) CancelBooking(ctx context.Context, venueID, bookingID int64) error {
	return r.UpdateBookingStatus(ctx, venueID, bookingID, "canceled")
}

func (r *Repository) GetBookingsByUser(ctx context.Context, userID int64, filter BookingFilter) ([]UserBooking, error) {
	// build a dynamic WHERE clause
	base := `
      SELECT
        b.id,
        b.venue_id,
        v.name,
        v.address,
        b.start_time,
        b.end_time,
        b.total_price,
        b.status,
        b.created_at
      FROM bookings b
      JOIN venues v ON v.id = b.venue_id
      WHERE b.user_id = $1`

	// we’ll collect args in a slice
	args := []interface{}{userID}
	idx := 2

	if filter.Status != nil {
		base += fmt.Sprintf(" AND b.status = $%d", idx)
		args = append(args, *filter.Status)
		idx++
	}

	// add ordering + limit/offset
	base += fmt.Sprintf(" ORDER BY b.created_at DESC LIMIT $%d OFFSET $%d", idx, idx+1)
	args = append(args, filter.Limit, filter.offset())

	rows, err := r.db.Query(ctx, base, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []UserBooking
	for rows.Next() {
		var ub UserBooking
		if err := rows.Scan(
			&ub.BookingID,
			&ub.VenueID,
			&ub.VenueName,
			&ub.VenueAddress,
			&ub.StartTime,
			&ub.EndTime,
			&ub.TotalPrice,
			&ub.Status,
			&ub.CreatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ub)
	}
	return out, rows.Err()
}

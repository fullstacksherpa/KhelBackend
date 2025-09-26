package bookings

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/database"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	GetBookingOwner(ctx context.Context, venueID, bookingID int64) (int64, error)
	GetPricingSlots(ctx context.Context, venueID int64, dayOfWeek string) ([]PricingSlot, error)
	GetBookingsForDate(ctx context.Context, venueID int64, date time.Time) ([]Interval, error)
	CreateBooking(ctx context.Context, booking *Booking) (int64, error)
	UpdatePricing(ctx context.Context, p *PricingSlot) error
	DeletePricingSlot(ctx context.Context, venueID, pricingID int64) error
	CreatePricingSlotsBatch(ctx context.Context, slots []*PricingSlot) error
	GetPendingBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]PendingBooking, error)
	GetCanceledBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]CanceledBooking, error)
	UpdateBookingStatus(ctx context.Context, venueID, bookingID int64, status string) error
	AcceptBooking(ctx context.Context, venueID, bookingID int64) error
	RejectBooking(ctx context.Context, venueID, bookingID int64) error
	CancelBooking(ctx context.Context, venueID, bookingID int64) error
	GetScheduledBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]ScheduledBooking, error)
	GetBookingsByUser(ctx context.Context, userID int64, filter BookingFilter) ([]UserBooking, error)
	GetBookingByID(ctx context.Context, bookingID int64) (*Booking, error)
	GetVenueOwnerIDFromBookingID(ctx context.Context, bookingID int64) (int64, error)
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
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

func (r *Repository) GetPricingSlots(ctx context.Context, venueID int64, dayOfWeek string) ([]PricingSlot, error) {
	query := `
        SELECT id, venue_id, day_of_week, start_time, end_time, price 
        FROM venue_pricing 
        WHERE venue_id = $1 AND day_of_week = $2
        ORDER BY start_time`
	rows, err := r.db.Query(ctx, query, venueID, dayOfWeek)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var slots []PricingSlot
	for rows.Next() {
		var ps PricingSlot
		var start, end time.Time
		if err := rows.Scan(&ps.ID, &ps.VenueID, &ps.DayOfWeek, &start, &end, &ps.Price); err != nil {
			return nil, err
		}
		// Parse the time values (assumes format "15:04:05")
		ps.StartTime = start
		ps.EndTime = end
		slots = append(slots, ps)
	}
	return slots, nil
}

func (r *Repository) GetBookingsForDate(ctx context.Context, venueID int64, date time.Time) ([]Interval, error) {
	// Load Kathmandu location
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return nil, fmt.Errorf("failed to load Kathmandu timezone: %w", err)
	}

	// Convert input date to Kathmandu time
	localDate := date.In(loc)

	// Compute start and end of day in Kathmandu time
	startOfDayLocal := time.Date(localDate.Year(), localDate.Month(), localDate.Day(), 0, 0, 0, 0, loc)
	endOfDayLocal := startOfDayLocal.Add(24 * time.Hour)

	// Convert to UTC for querying Postgres
	startOfDayUTC := startOfDayLocal.UTC()
	endOfDayUTC := endOfDayLocal.UTC()

	// Query for bookings
	query := `
        SELECT start_time, end_time
        FROM bookings
        WHERE venue_id = $1 AND status = 'confirmed'
          AND start_time < $2 AND end_time > $3
        ORDER BY start_time
    `
	rows, err := r.db.Query(ctx, query, venueID, endOfDayUTC, startOfDayUTC)
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
		intervals = append(intervals, Interval{Start: start, End: end})
	}
	return intervals, nil
}

// CreateBooking inserts a booking record into the database.
func (r *Repository) CreateBooking(ctx context.Context, booking *Booking) (int64, error) {
	query := `
        INSERT INTO bookings (
            venue_id, user_id, start_time, end_time, total_price, status,
            customer_name, customer_phone, note
        ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
        RETURNING id, created_at, updated_at
    `
	err := r.db.QueryRow(ctx, query,
		booking.VenueID,
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
		SET day_of_week = $1, start_time = $2, end_time = $3, price = $4
		WHERE id = $5 AND venue_id = $6`
	result, err := r.db.Exec(ctx, query, p.DayOfWeek, p.StartTime.Format("15:04:05"), p.EndTime.Format("15:04:05"), p.Price, p.ID, p.VenueID)
	if err != nil {
		return err
	}
	rowsAffected := result.RowsAffected()

	if rowsAffected == 0 {
		return fmt.Errorf("failed to update Pricing for this venue with id=%d", p.VenueID)
	}
	return nil
}

// CreatePricingSlotsBatch uses pgx.Batch to insert multiple pricing slots in one round-trip.
func (r *Repository) CreatePricingSlotsBatch(ctx context.Context, slots []*PricingSlot) error {
	// Use your withTx helper for transaction
	return database.WithTx(r.db, ctx, func(tx pgx.Tx) error {
		// Prepare the SQL once
		const sql = `
		INSERT INTO venue_pricing
		  (venue_id, day_of_week, start_time, end_time, price)
		VALUES ($1,$2,$3,$4,$5)
		RETURNING id;
		`

		// Build a batch
		batch := &pgx.Batch{}
		for _, slot := range slots {
			batch.Queue(sql,
				slot.VenueID,
				slot.DayOfWeek,
				slot.StartTime.Format("15:04:05"),
				slot.EndTime.Format("15:04:05"),
				slot.Price,
			)
		}

		// Send the batch
		br := tx.SendBatch(ctx, batch)
		defer br.Close()

		// Read all results and populate IDs
		for i, slot := range slots {
			// You can also use br.QueryRow()
			if err := br.QueryRow().Scan(&slot.ID); err != nil {
				return fmt.Errorf("batch insert slot[%d]: %w", i, err)
			}
		}

		return nil
	})
}

func (r *Repository) DeletePricingSlot(ctx context.Context, venueID, pricingID int64) error {
	const q = `
         DELETE FROM venue_pricing WHERE id = $1 AND venue_id = $2
`

	res, err := r.db.Exec(ctx, q, pricingID, venueID)
	if err != nil {
		return fmt.Errorf("failed to delete pricing slot: %w", err)
	}

	if res.RowsAffected() == 0 {
		return fmt.Errorf("no pricing slot found with  id=%d for venue_id=%d", pricingID, venueID)
	}
	return nil
}

// GetBookingByID retrieves a single booking by its ID
func (r *Repository) GetBookingByID(ctx context.Context, bookingID int64) (*Booking, error) {
	const query = `
        SELECT 
            id, venue_id, user_id, start_time, end_time, 
            total_price, status, created_at, updated_at,
            customer_name, customer_phone, note
        FROM bookings
        WHERE id = $1
    `

	var booking Booking
	err := r.db.QueryRow(ctx, query, bookingID).Scan(
		&booking.ID,
		&booking.VenueID,
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
        JOIN venues v ON b.venue_id = v.id
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

// GetPendingBookingsForVenueDate loads all bookings with status='pending'
// for the given venue on the given date.
func (r *Repository) GetPendingBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]PendingBooking, error) {
	// normalize to date only
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	const q = `
      SELECT
        b.id,
        b.user_id,
        u.first_name || ' ' || u.last_name   AS user_name,
        u.profile_picture_url,
        u.phone,
        b.total_price,
        b.created_at,
        b.start_time,
        b.end_time
      FROM bookings b
      JOIN users u ON u.id = b.user_id
      WHERE
        b.venue_id    = $1
        AND b.status  = 'pending'
        AND b.start_time >= $2
        AND b.start_time <  $3
      ORDER BY b.created_at
    `
	rows, err := r.db.Query(ctx, q, venueID, startOfDay, endOfDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []PendingBooking
	for rows.Next() {
		var pb PendingBooking
		if err := rows.Scan(
			&pb.BookingID,
			&pb.UserID,
			&pb.UserName,
			&pb.UserImageURL,
			&pb.UserPhone,
			&pb.Price,
			&pb.RequestedAt,
			&pb.StartTime,
			&pb.EndTime,
		); err != nil {
			return nil, err
		}
		out = append(out, pb)
	}
	return out, rows.Err()
}

func (r *Repository) GetCanceledBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]CanceledBooking, error) {
	// normalize to date only
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, date.Location())
	endOfDay := startOfDay.Add(24 * time.Hour)

	const q = `
      SELECT
        b.id,
        b.user_id,
        u.first_name || ' ' || u.last_name   AS user_name,
        u.profile_picture_url,
        u.phone,
        b.total_price,
        b.created_at,
        b.start_time,
        b.end_time
      FROM bookings b
      JOIN users u ON u.id = b.user_id
      WHERE
        b.venue_id    = $1
        AND b.status  = 'canceled'
        AND b.start_time >= $2
        AND b.start_time <  $3
      ORDER BY b.created_at
    `
	rows, err := r.db.Query(ctx, q, venueID, startOfDay, endOfDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CanceledBooking
	for rows.Next() {
		var pb CanceledBooking
		if err := rows.Scan(
			&pb.BookingID,
			&pb.UserID,
			&pb.UserName,
			&pb.UserImageURL,
			&pb.UserPhone,
			&pb.Price,
			&pb.RequestedAt,
			&pb.StartTime,
			&pb.EndTime,
		); err != nil {
			return nil, err
		}
		out = append(out, pb)
	}
	return out, rows.Err()
}

func (r *Repository) GetScheduledBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]ScheduledBooking, error) {
	// normalize to date only
	startOfDay := date.In(time.UTC)
	endOfDay := date.Add(24 * time.Hour).In(time.UTC)

	const q = `
      SELECT
        b.id,
        b.user_id,
        u.first_name || ' ' || u.last_name   AS user_name,
        u.profile_picture_url,
        u.phone,
        b.total_price,
        b.updated_at,
        b.start_time,
        b.end_time,
		b.customer_name,
			b.customer_phone,
			b.note
      FROM bookings b
      JOIN users u ON u.id = b.user_id
      WHERE
        b.venue_id    = $1
        AND b.status  = 'confirmed'
        AND b.start_time >= $2
        AND b.start_time <  $3
      ORDER BY b.start_time
    `
	rows, err := r.db.Query(ctx, q, venueID, startOfDay, endOfDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ScheduledBooking
	for rows.Next() {
		var sb ScheduledBooking
		if err := rows.Scan(
			&sb.BookingID,
			&sb.UserID,
			&sb.UserName,
			&sb.UserImageURL,
			&sb.UserPhone,
			&sb.Price,
			&sb.AcceptedAt,
			&sb.StartTime,
			&sb.EndTime,
			&sb.CustomerName,
			&sb.CustomerPhone,
			&sb.Note,
		); err != nil {
			return nil, err
		}
		out = append(out, sb)
	}
	return out, rows.Err()
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

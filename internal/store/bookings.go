package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type PricingSlot struct {
	ID        int64
	VenueID   int64
	DayOfWeek string
	// Note: start_time and end_time are stored as TIME in the DB.
	// We use time.Time to hold the time part.
	StartTime time.Time
	EndTime   time.Time
	Price     int
}

// Booking represents a booking record.
type Booking struct {
	ID         int64     `json:"id"`
	VenueID    int64     `json:"venue_id"`
	UserID     int64     `json:"user_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	TotalPrice int       `json:"total_price"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// AvailableTimeSlot represents a free time interval for booking.
type AvailableTimeSlot struct {
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	PricePerHour int       `json:"price_per_hour"`
}

// Interval is used for time arithmetic.
type Interval struct {
	Start time.Time
	End   time.Time
}

// PendingBooking is the view model for a pending booking request.
type PendingBooking struct {
	BookingID    int64     `json:"booking_id"`
	UserID       int64     `json:"user_id"`
	UserName     string    `json:"user_name"`
	UserImageURL *string   `json:"user_image"` // nullable
	UserPhone    string    `json:"user_number"`
	Price        int       `json:"price"`
	RequestedAt  time.Time `json:"requested_at"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
}

type ScheduledBooking struct {
	BookingID    int64     `json:"booking_id"`
	UserID       int64     `json:"user_id"`
	UserName     string    `json:"user_name"`
	UserImageURL *string   `json:"user_image"`
	UserPhone    string    `json:"user_number"`
	Price        int       `json:"price"`
	AcceptedAt   time.Time `json:"accepted_at"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
}

type UserBooking struct {
	BookingID    int64     `json:"booking_id"`
	VenueID      int64     `json:"venue_id"`
	VenueName    string    `json:"venue_name"`
	VenueAddress string    `json:"venue_address"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	TotalPrice   int       `json:"total_price"`
	Status       string    `json:"status"`
	CreatedAt    time.Time `json:"created_at"`
}

type BookingStore struct {
	db *pgxpool.Pool
}

func (s *BookingStore) GetPricingSlots(ctx context.Context, venueID int64, dayOfWeek string) ([]PricingSlot, error) {
	query := `
        SELECT id, venue_id, day_of_week, start_time, end_time, price 
        FROM venue_pricing 
        WHERE venue_id = $1 AND day_of_week = $2
        ORDER BY start_time`
	rows, err := s.db.Query(ctx, query, venueID, dayOfWeek)
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

// GetBookingsForDate retrieves existing bookings for a venue on a specific date.
// Returns a slice of Interval representing booked time periods.
func (s *BookingStore) GetBookingsForDate(ctx context.Context, venueID int64, date time.Time) ([]Interval, error) {
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour)

	query := `
        SELECT start_time, end_time
        FROM bookings
        WHERE venue_id = $1 AND start_time < $2 AND end_time > $3
        ORDER BY start_time
    `
	rows, err := s.db.Query(ctx, query, venueID, endOfDay, startOfDay)
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
func (s *BookingStore) CreateBooking(ctx context.Context, booking *Booking) error {
	query := `
        INSERT INTO bookings (venue_id, user_id, start_time, end_time, total_price, status)
        VALUES ($1, $2, $3, $4, $5, $6)
        RETURNING id, created_at, updated_at
    `
	return s.db.QueryRow(ctx, query,
		booking.VenueID,
		booking.UserID,
		booking.StartTime,
		booking.EndTime,
		booking.TotalPrice,
		booking.Status,
	).Scan(&booking.ID, &booking.CreatedAt, &booking.UpdatedAt)
}

// UpdatePricing updates a pricing slot in the database.
func (s *BookingStore) UpdatePricing(ctx context.Context, p *PricingSlot) error {
	query := `
		UPDATE venue_pricing 
		SET day_of_week = $1, start_time = $2, end_time = $3, price = $4
		WHERE id = $5 AND venue_id = $6`
	result, err := s.db.Exec(ctx, query, p.DayOfWeek, p.StartTime.Format("15:04:05"), p.EndTime.Format("15:04:05"), p.Price, p.ID, p.VenueID)
	if err != nil {
		return err
	}
	rowsAffected := result.RowsAffected()

	if rowsAffected == 0 {
		return fmt.Errorf("failed to update Pricing for this venue with id=%d", p.VenueID)
	}
	return nil
}

func (s *BookingStore) CreatePricingSlot(ctx context.Context, p *PricingSlot) error {
	query := `
    INSERT INTO venue_pricing
      (venue_id, day_of_week, start_time, end_time, price)
    VALUES ($1,$2,$3,$4,$5)
    RETURNING id`
	return s.db.QueryRow(ctx, query,
		p.VenueID,
		p.DayOfWeek,
		p.StartTime.Format("15:04:05"),
		p.EndTime.Format("15:04:05"),
		p.Price,
	).Scan(&p.ID)
}

// GetPendingBookingsForVenueDate loads all bookings with status='pending'
// for the given venue on the given date.
func (s *BookingStore) GetPendingBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]PendingBooking, error) {
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
	rows, err := s.db.Query(ctx, q, venueID, startOfDay, endOfDay)
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

func (s *BookingStore) GetScheduledBookingsForVenueDate(ctx context.Context, venueID int64, date time.Time) ([]ScheduledBooking, error) {
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
        b.updated_at,
        b.start_time,
        b.end_time
      FROM bookings b
      JOIN users u ON u.id = b.user_id
      WHERE
        b.venue_id    = $1
        AND b.status  = 'confirmed'
        AND b.start_time >= $2
        AND b.start_time <  $3
      ORDER BY b.start_time
    `
	rows, err := s.db.Query(ctx, q, venueID, startOfDay, endOfDay)
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
		); err != nil {
			return nil, err
		}
		out = append(out, sb)
	}
	return out, rows.Err()
}

// UpdateBookingStatus sets a new status ("confirmed", "rejected", etc.) on a booking.
func (s *BookingStore) UpdateBookingStatus(ctx context.Context, venueID, bookingID int64, status string) error {
	const q = `
      UPDATE bookings
      SET status    = $1,
          updated_at = NOW()
      WHERE id       = $2
        AND venue_id = $3
    `
	res, err := s.db.Exec(ctx, q, status, bookingID, venueID)
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
func (s *BookingStore) AcceptBooking(ctx context.Context, venueID, bookingID int64) error {
	return s.UpdateBookingStatus(ctx, venueID, bookingID, "confirmed")
}

// RejectBooking marks a pending booking as rejected.
func (s *BookingStore) RejectBooking(ctx context.Context, venueID, bookingID int64) error {
	return s.UpdateBookingStatus(ctx, venueID, bookingID, "rejected")
}

func (s *BookingStore) GetBookingsByUserID(ctx context.Context, userID int64) ([]UserBooking, error) {
	const q = `
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
      WHERE b.user_id = $1
      ORDER BY b.created_at DESC
    `
	rows, err := s.db.Query(ctx, q, userID)
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

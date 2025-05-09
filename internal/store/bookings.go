package store

import (
	"context"
	"database/sql"
	"time"
)

// PricingSlot represents a pricing and availability slot for a venue.
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

type BookingStore struct {
	db *sql.DB
}

func (s *BookingStore) GetPricingSlots(ctx context.Context, venueID int64, dayOfWeek string) ([]PricingSlot, error) {
	query := `
        SELECT id, venue_id, day_of_week, start_time, end_time, price 
        FROM venue_pricing 
        WHERE venue_id = $1 AND day_of_week = $2
        ORDER BY start_time`
	rows, err := s.db.QueryContext(ctx, query, venueID, dayOfWeek)
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
	rows, err := s.db.QueryContext(ctx, query, venueID, endOfDay, startOfDay)
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
	return s.db.QueryRowContext(ctx, query,
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
	result, err := s.db.ExecContext(ctx, query, p.DayOfWeek, p.StartTime.Format("15:04:05"), p.EndTime.Format("15:04:05"), p.Price, p.ID, p.VenueID)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *BookingStore) CreatePricingSlot(ctx context.Context, p *PricingSlot) error {
	query := `
    INSERT INTO venue_pricing
      (venue_id, day_of_week, start_time, end_time, price)
    VALUES ($1,$2,$3,$4,$5)
    RETURNING id`
	return s.db.QueryRowContext(ctx, query,
		p.VenueID,
		p.DayOfWeek,
		p.StartTime.Format("15:04:05"),
		p.EndTime.Format("15:04:05"),
		p.Price,
	).Scan(&p.ID)
}

package bookings

import "time"

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
	ID            int64     `json:"id"`
	VenueID       int64     `json:"venue_id"`
	UserID        int64     `json:"user_id"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	TotalPrice    int       `json:"total_price"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	CustomerName  *string   `json:"customer_name,omitempty" swaggertype:"string"`  // optional
	CustomerPhone *string   `json:"customer_phone,omitempty" swaggertype:"string"` // optional
	Note          *string   `json:"note,omitempty" swaggertype:"string"`           // optional
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

type CanceledBooking struct {
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
	BookingID     int64     `json:"booking_id"`
	UserID        int64     `json:"user_id"`
	UserName      string    `json:"user_name"`
	UserImageURL  *string   `json:"user_image"`
	UserPhone     string    `json:"user_number"`
	Price         int       `json:"price"`
	AcceptedAt    time.Time `json:"accepted_at"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	CustomerName  *string   `json:"customer_name,omitempty" swaggertype:"string"`  // optional
	CustomerPhone *string   `json:"customer_phone,omitempty" swaggertype:"string"` // optional
	Note          *string   `json:"note,omitempty" swaggertype:"string"`           // optional
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

type BookingFilter struct {
	Status *string // nil = no filtering
	Page   int     // 1-based
	Limit  int     // max items per page
}

func (f BookingFilter) offset() int {
	return (f.Page - 1) * f.Limit
}

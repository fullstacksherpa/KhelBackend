package bookings

import "time"

type PricingSlot struct {
	ID         int64     `json:"id"`
	VenueID    int64     `json:"venue_id"`    // kept for backward compatibility
	FacilityID int64     `json:"facility_id"` // new real booking/pricing scope
	DayOfWeek  string    `json:"day_of_week"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	Price      int       `json:"price"`
}

// Booking represents a booking record.
type Booking struct {
	ID         int64 `json:"id"`
	VenueID    int64 `json:"venue_id"`    // keep for now
	FacilityID int64 `json:"facility_id"` // new real booking scope

	UserID     int64     `json:"user_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	TotalPrice int       `json:"total_price"`
	Status     string    `json:"status"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	CustomerName  *string `json:"customer_name,omitempty" swaggertype:"string"`
	CustomerPhone *string `json:"customer_phone,omitempty" swaggertype:"string"`
	Note          *string `json:"note,omitempty" swaggertype:"string"`
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
	FacilityID   int64     `json:"facility_id"`
	VenueName    string    `json:"venue_name"`
	FacilityName string    `json:"facility_name"`
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

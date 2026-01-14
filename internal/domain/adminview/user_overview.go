package adminview

import "time"

//this is for types only to prevent from circular import

type UserDTO struct {
	ID                int64     `json:"id"`
	FirstName         string    `json:"first_name"`
	LastName          string    `json:"last_name"`
	Email             string    `json:"email"`
	Phone             string    `json:"phone"`
	ProfilePictureURL *string   `json:"profile_picture_url,omitempty"`
	SkillLevel        *string   `json:"skill_level,omitempty"`
	NoOfGames         *int      `json:"no_of_games,omitempty"`
	IsActive          bool      `json:"is_active"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type UserStats struct {
	OrdersCount   int `json:"orders_count"`
	BookingsCount int `json:"bookings_count"`
	GamesCount    int `json:"games_count"`

	TotalSpentCents int64 `json:"total_spent_cents"`

	LastOrderAt   *time.Time `json:"last_order_at,omitempty"`
	LastBookingAt *time.Time `json:"last_booking_at,omitempty"`
	LastGameAt    *time.Time `json:"last_game_at,omitempty"`
}

type OrderDTO struct {
	ID            int64     `json:"id"`
	OrderNumber   string    `json:"order_number"`
	Status        string    `json:"status"`
	PaymentStatus string    `json:"payment_status"`
	TotalCents    int64     `json:"total_cents"`
	CreatedAt     time.Time `json:"created_at"`
}

type BookingDTO struct {
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

type GameDTO struct {
	ID        int64     `json:"id"`
	SportType string    `json:"sport_type"`
	VenueID   int64     `json:"venue_id"`
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	Status    string    `json:"status"`
}

type UserRecent struct {
	Orders   []OrderDTO   `json:"orders"`
	Bookings []BookingDTO `json:"bookings"`
	Games    []GameDTO    `json:"games"`
}

type UserOverview struct {
	User   UserDTO    `json:"user"`
	Stats  UserStats  `json:"stats"`
	Recent UserRecent `json:"recent"`
}

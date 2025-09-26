package venuereviews

import "time"

type Review struct {
	ID        int64     `json:"id"`
	VenueID   int64     `json:"venue_id"`
	UserID    int64     `json:"user_id"`
	Rating    int       `json:"rating"` // 1-5
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Joined fields
	UserName  string  `json:"user_name,omitempty"`
	AvatarURL *string `json:"avatar_url,omitempty"`
}

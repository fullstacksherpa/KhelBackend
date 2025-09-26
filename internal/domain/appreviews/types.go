package appreviews

import "time"

type AppReview struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Rating    int       `json:"rating"`
	Feedback  string    `json:"feedback"`
	CreatedAt time.Time `json:"created_at"`
}

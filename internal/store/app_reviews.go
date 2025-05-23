package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Review represents an app review submitted by a user.
type AppReview struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Rating    int       `json:"rating"`
	Feedback  string    `json:"feedback"`
	CreatedAt time.Time `json:"created_at"`
}

// ReviewsStore handles database operations for app reviews.
type AppReviewStore struct {
	db *pgxpool.Pool
}

// AddReview inserts a new review record into the app_reviews table.
func (s *AppReviewStore) AddReview(ctx context.Context, userID int64, rating int, feedback string) error {
	query := `
        INSERT INTO app_reviews (user_id, rating, feedback)
        VALUES ($1, $2, $3)
    `
	if _, err := s.db.Exec(ctx, query, userID, rating, feedback); err != nil {
		return fmt.Errorf("failed to insert review: %w", err)
	}
	return nil
}

// GetAllReviews retrieves all reviews ordered by created_at descending.
func (s *AppReviewStore) GetAllReviews(ctx context.Context) ([]AppReview, error) {
	query := `
        SELECT id, user_id, rating, feedback, created_at
        FROM app_reviews
        ORDER BY created_at DESC
    `

	rows, err := s.db.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query reviews: %w", err)
	}
	defer rows.Close()

	var reviews []AppReview
	for rows.Next() {
		var r AppReview
		if err := rows.Scan(&r.ID, &r.UserID, &r.Rating, &r.Feedback, &r.CreatedAt); err != nil {
			return nil, fmt.Errorf("failed to scan review row: %w", err)
		}
		reviews = append(reviews, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return reviews, nil
}

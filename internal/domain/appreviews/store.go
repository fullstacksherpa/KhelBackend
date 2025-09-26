package appreviews

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	AddReview(ctx context.Context, userID int64, rating int, feedback string) error
	GetAllReviews(ctx context.Context) ([]AppReview, error)
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

// AddReview inserts a new review record into the app_reviews table.
func (r *Repository) AddReview(ctx context.Context, userID int64, rating int, feedback string) error {
	query := `
        INSERT INTO app_reviews (user_id, rating, feedback)
        VALUES ($1, $2, $3)
    `
	if _, err := r.db.Exec(ctx, query, userID, rating, feedback); err != nil {
		return fmt.Errorf("failed to insert review: %w", err)
	}
	return nil
}

// GetAllReviews retrieves all reviews ordered by created_at descending.
func (r *Repository) GetAllReviews(ctx context.Context) ([]AppReview, error) {
	query := `
        SELECT id, user_id, rating, feedback, created_at
        FROM app_reviews
        ORDER BY created_at DESC
    `

	rows, err := r.db.Query(ctx, query)
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

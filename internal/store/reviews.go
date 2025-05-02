package store

import (
	"context"
	"database/sql"
	"time"
)

type Review struct {
	ID        int64     `json:"id"`
	VenueID   int64     `json:"venue_id"`
	UserID    int64     `json:"user_id"`
	Rating    int       `json:"rating"` // 1-5
	Comment   string    `json:"comment"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	// Joined fields
	UserName  string `json:"user_name,omitempty"`
	AvatarURL string `json:"avatar_url,omitempty"`
}
type ReviewStore struct {
	db *sql.DB
}

func (s *ReviewStore) CreateReview(ctx context.Context, review *Review) error {
	query := `
        INSERT INTO reviews (venue_id, user_id, rating, comment)
        VALUES ($1, $2, $3, $4)
        RETURNING id, created_at, updated_at
    `
	return s.db.QueryRowContext(ctx, query,
		review.VenueID,
		review.UserID,
		review.Rating,
		review.Comment,
	).Scan(&review.ID, &review.CreatedAt, &review.UpdatedAt)
}

func (s *ReviewStore) GetReviews(ctx context.Context, venueID int64) ([]Review, error) {
	query := `
        SELECT vr.id, vr.venue_id, vr.user_id, vr.rating, vr.comment, 
               vr.created_at, vr.updated_at, u.name, u.avatar_url
        FROM reviews vr
        JOIN users u ON u.id = vr.user_id
        WHERE vr.venue_id = $1
        ORDER BY vr.created_at DESC
    `
	rows, err := s.db.QueryContext(ctx, query, venueID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var reviews []Review
	for rows.Next() {
		var review Review
		err := rows.Scan(
			&review.ID,
			&review.VenueID,
			&review.UserID,
			&review.Rating,
			&review.Comment,
			&review.CreatedAt,
			&review.UpdatedAt,
			&review.UserName,
			&review.AvatarURL,
		)
		if err != nil {
			return nil, err
		}
		reviews = append(reviews, review)
	}
	return reviews, nil
}

func (s *ReviewStore) DeleteReview(ctx context.Context, reviewID, userID int64) error {
	query := `
        DELETE FROM reviews 
        WHERE id = $1 AND user_id = $2
    `
	result, err := s.db.ExecContext(ctx, query, reviewID, userID)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *ReviewStore) GetReviewStats(ctx context.Context, venueID int64) (total int, average float64, err error) {
	query := `
        SELECT 
            COUNT(id) as total_reviews,
            COALESCE(AVG(rating), 0) as average_rating
        FROM reviews
        WHERE venue_id = $1
    `
	err = s.db.QueryRowContext(ctx, query, venueID).Scan(&total, &average)
	return total, average, err
}

func (s *ReviewStore) IsReviewOwner(ctx context.Context, reviewID int64, userID int64) (bool, error) {
	var reviewUserID int64
	err := s.db.QueryRowContext(ctx, `SELECT user_id FROM reviews WHERE id = $1`, reviewID).Scan(&reviewUserID)
	if err != nil {
		return false, err
	}

	return reviewUserID == userID, nil
}

// HasReview returns true if a review by this user on this venue already exists.
func (s *ReviewStore) HasReview(ctx context.Context, venueID, userID int64) (bool, error) {
	var exists bool
	query := `
        SELECT EXISTS (
          SELECT 1 FROM reviews
          WHERE venue_id = $1 AND user_id = $2
        )
    `
	err := s.db.QueryRowContext(ctx, query, venueID, userID).Scan(&exists)
	return exists, err
}

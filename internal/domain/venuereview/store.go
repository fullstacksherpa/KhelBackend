package venuereviews

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	CreateReview(context.Context, *Review) error
	GetReviews(context.Context, int64) ([]Review, error)
	DeleteReview(context.Context, int64, int64) error
	GetReviewStats(context.Context, int64) (int, float64, error)
	IsReviewOwner(ctx context.Context, reviewID int64, userID int64) (bool, error)
	HasReview(ctx context.Context, venueID, userID int64) (bool, error)
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

func (r *Repository) CreateReview(ctx context.Context, review *Review) error {
	query := `
        INSERT INTO reviews (venue_id, user_id, rating, comment)
        VALUES ($1, $2, $3, $4)
        RETURNING id, created_at, updated_at
    `
	return r.db.QueryRow(ctx, query,
		review.VenueID,
		review.UserID,
		review.Rating,
		review.Comment,
	).Scan(&review.ID, &review.CreatedAt, &review.UpdatedAt)
}

func (r *Repository) GetReviews(ctx context.Context, venueID int64) ([]Review, error) {
	query := `
        SELECT vr.id, vr.venue_id, vr.user_id, vr.rating, vr.comment, 
               vr.created_at, vr.updated_at, u.first_name, u.profile_picture_url
        FROM reviews vr
        JOIN users u ON u.id = vr.user_id
        WHERE vr.venue_id = $1
        ORDER BY vr.created_at DESC
    `
	rows, err := r.db.Query(ctx, query, venueID)
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

func (r *Repository) DeleteReview(ctx context.Context, reviewID, userID int64) error {
	query := `
        DELETE FROM reviews 
        WHERE id = $1 AND user_id = $2
    `
	result, err := r.db.Exec(ctx, query, reviewID, userID)
	if err != nil {
		return err
	}

	rowsAffected := result.RowsAffected()

	if rowsAffected == 0 {
		return fmt.Errorf("no review found for deletion: id=%d user_id=%d", reviewID, userID)
	}
	return nil
}

func (r *Repository) GetReviewStats(ctx context.Context, venueID int64) (total int, average float64, err error) {
	query := `
        SELECT 
            COUNT(id) as total_reviews,
            COALESCE(AVG(rating), 0) as average_rating
        FROM reviews
        WHERE venue_id = $1
    `
	err = r.db.QueryRow(ctx, query, venueID).Scan(&total, &average)
	return total, average, err
}

func (r *Repository) IsReviewOwner(ctx context.Context, reviewID int64, userID int64) (bool, error) {
	var reviewUserID int64
	err := r.db.QueryRow(ctx, `SELECT user_id FROM reviews WHERE id = $1`, reviewID).Scan(&reviewUserID)
	if err != nil {
		return false, err
	}

	return reviewUserID == userID, nil
}

// HasReview returns true if a review by this user on this venue already exists.
func (r *Repository) HasReview(ctx context.Context, venueID, userID int64) (bool, error) {
	var exists bool
	query := `
        SELECT EXISTS (
          SELECT 1 FROM reviews
          WHERE venue_id = $1 AND user_id = $2
        )
    `
	err := r.db.QueryRow(ctx, query, venueID, userID).Scan(&exists)
	return exists, err
}

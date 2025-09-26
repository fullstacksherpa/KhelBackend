package followers

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	Follow(ctx context.Context, followerID, userID int64) error
	Unfollow(ctx context.Context, followerID, userID int64) error
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

func (r *Repository) Follow(ctx context.Context, followerID, userID int64) error {
	query := `
           INSERT INTO followers (user_id, follower_id) VALUES ($1, $2)
   `

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := r.db.Exec(ctx, query, userID, followerID)
	if err != nil {
		log.Printf("Follow query failed for userID=%d, followerID=%d: %v", userID, followerID, err)

		return fmt.Errorf("failed to follow user: %w", err)
	}
	return nil
}

func (r *Repository) Unfollow(ctx context.Context, followerID, userID int64) error {
	query := `
	   DELETE FROM followers
	   WHERE user_id = $1 AND follower_id = $2
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := r.db.Exec(ctx, query, userID, followerID)
	return err
}

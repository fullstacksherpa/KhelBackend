package store

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Follower struct {
	UserID     int64  `json:"user_id"`
	FollowerID int64  `json:"follower_id"`
	CreatedAt  string `json:"created_at"`
}

type FollowerStore struct {
	db *pgxpool.Pool
}

func (s *FollowerStore) Follow(ctx context.Context, followerID, userID int64) error {
	query := `
           INSERT INTO followers (user_id, follower_id) VALUES ($1, $2)
   `

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := s.db.Exec(ctx, query, userID, followerID)
	if err != nil {
		log.Printf("Follow query failed for userID=%d, followerID=%d: %v", userID, followerID, err)

		return fmt.Errorf("failed to follow user: %w", err)
	}
	return nil
}

func (s *FollowerStore) Unfollow(ctx context.Context, followerID, userID int64) error {
	query := `
	   DELETE FROM followers
	   WHERE user_id = $1 AND follower_id = $2
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	_, err := s.db.Exec(ctx, query, userID, followerID)
	return err
}

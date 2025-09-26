package pushtokens

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store interface {
	AddOrUpdatePushToken(ctx context.Context, userID int64, token string, deviceInfo json.RawMessage) error
	RemovePushToken(ctx context.Context, userID int64, token string) error
	RemoveTokensByTokenList(ctx context.Context, tokens []string) error
	GetTokensByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]string, error)
	PruneStaleTokens(ctx context.Context, olderThan time.Duration) error
}

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

// AddOrUpdatePushToken upserts token + device info, updates last_updated
func (r *Repository) AddOrUpdatePushToken(ctx context.Context, userID int64, token string, deviceInfo json.RawMessage) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	q := `
	INSERT INTO user_push_tokens (user_id, expo_push_token, device_info, last_updated)
	VALUES ($1, $2, $3, NOW())
	ON CONFLICT (user_id, expo_push_token)
	DO UPDATE SET device_info = EXCLUDED.device_info, last_updated = NOW();
	`

	_, err := r.db.Exec(ctx, q, userID, token, deviceInfo)
	return err
}

// RemovePushToken deletes a token for a user
func (r *Repository) RemovePushToken(ctx context.Context, userID int64, token string) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	q := `DELETE FROM user_push_tokens WHERE user_id = $1 AND expo_push_token = $2`
	_, err := r.db.Exec(ctx, q, userID, token)
	return err
}

// RemoveTokensByTokenList deletes tokens matching any token in the slice
func (r *Repository) RemoveTokensByTokenList(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	// expo tokens are text[] accepted by pg
	q := `DELETE FROM user_push_tokens WHERE expo_push_token = ANY($1)`
	_, err := r.db.Exec(ctx, q, tokens)
	return err
}

// The function retrieves push notification tokens for multiple users at once, returning them as a map where each key is a UserID and the value is a slice of push token associated with that user.
func (r *Repository) GetTokensByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]string, error) {
	result := make(map[int64][]string)
	if len(userIDs) == 0 {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration) //5second
	defer cancel()

	q := `SELECT user_id, expo_push_token FROM user_push_tokens WHERE user_id = ANY($1)`
	rows, err := r.db.Query(ctx, q, userIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var uid int64
	var token string
	for rows.Next() {
		if err := rows.Scan(&uid, &token); err != nil {
			return nil, err
		}
		result[uid] = append(result[uid], token)
	}
	return result, rows.Err()
}

// PruneStaleTokens deletes tokens not updated in olderThan duration
func (r *Repository) PruneStaleTokens(ctx context.Context, olderThan time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	// pass interval string e.g. "90 days" or "3600 seconds"
	interval := fmt.Sprintf("%d seconds", int64(olderThan.Seconds()))
	q := `DELETE FROM user_push_tokens WHERE last_updated < NOW() - $1::interval`
	_, err := r.db.Exec(ctx, q, interval)
	return err
}

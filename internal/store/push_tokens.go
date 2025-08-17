// internal/store/push_tokens.go
package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNoTokens = errors.New("no push tokens")

type PushTokensStore struct {
	db *pgxpool.Pool
}

// AddOrUpdatePushToken upserts token + device info, updates last_updated
func (s *PushTokensStore) AddOrUpdatePushToken(ctx context.Context, userID int64, token string, deviceInfo json.RawMessage) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	q := `
	INSERT INTO user_push_tokens (user_id, expo_push_token, device_info, last_updated)
	VALUES ($1, $2, $3, NOW())
	ON CONFLICT (user_id, expo_push_token)
	DO UPDATE SET device_info = EXCLUDED.device_info, last_updated = NOW();
	`

	_, err := s.db.Exec(ctx, q, userID, token, deviceInfo)
	return err
}

// RemovePushToken deletes a token for a user
func (s *PushTokensStore) RemovePushToken(ctx context.Context, userID int64, token string) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	q := `DELETE FROM user_push_tokens WHERE user_id = $1 AND expo_push_token = $2`
	_, err := s.db.Exec(ctx, q, userID, token)
	return err
}

// RemoveTokensByTokenList deletes tokens matching any token in the slice
func (s *PushTokensStore) RemoveTokensByTokenList(ctx context.Context, tokens []string) error {
	if len(tokens) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	// expo tokens are text[] accepted by pg
	q := `DELETE FROM user_push_tokens WHERE expo_push_token = ANY($1)`
	_, err := s.db.Exec(ctx, q, tokens)
	return err
}

// The function retrieves push notification tokens for multiple users at once, returning them as a map where each key is a UserID and the value is a slice of push token associated with that user.
func (s *PushTokensStore) GetTokensByUserIDs(ctx context.Context, userIDs []int64) (map[int64][]string, error) {
	result := make(map[int64][]string)
	if len(userIDs) == 0 {
		return result, nil
	}

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration) //5second
	defer cancel()

	q := `SELECT user_id, expo_push_token FROM user_push_tokens WHERE user_id = ANY($1)`
	rows, err := s.db.Query(ctx, q, userIDs)
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
func (s *PushTokensStore) PruneStaleTokens(ctx context.Context, olderThan time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	// pass interval string e.g. "90 days" or "3600 seconds"
	interval := fmt.Sprintf("%d seconds", int64(olderThan.Seconds()))
	q := `DELETE FROM user_push_tokens WHERE last_updated < NOW() - $1::interval`
	_, err := s.db.Exec(ctx, q, interval)
	return err
}

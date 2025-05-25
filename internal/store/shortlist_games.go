package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ShortlistedGame represents a record in the shortlisted_games table.
type ShortlistedGame struct {
	UserID    int64     `json:"user_id"`
	GameID    int64     `json:"game_id"`
	CreatedAt time.Time `json:"created_at"`
}

type ShortlistedGameDetail struct {
	Game
	VenueName    string `json:"venue_name"`
	VenueAddress string `json:"venue_address"`
}

// ShortlistGamesStore handles database operations for shortlisted games.
type ShortlistGamesStore struct {
	db *pgxpool.Pool
}

// AddShortlist adds a game to the user's shortlist.
func (s *ShortlistGamesStore) AddShortlist(ctx context.Context, userID, gameID int64) error {
	query := `
		INSERT INTO shortlisted_games (user_id, game_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := s.db.Exec(ctx, query, userID, gameID)
	if err != nil {
		return fmt.Errorf("failed to add shortlisted game: %w", err)
	}
	return nil
}

// RemoveShortlist removes a game from the user's shortlist.
func (s *ShortlistGamesStore) RemoveShortlist(ctx context.Context, userID, gameID int64) error {
	query := `
		DELETE FROM shortlisted_games
		WHERE user_id = $1 AND game_id = $2
	`
	_, err := s.db.Exec(ctx, query, userID, gameID)
	if err != nil {
		return fmt.Errorf("failed to remove shortlisted game: %w", err)
	}
	return nil
}

func (s *ShortlistGamesStore) GetShortlistedGamesByUser(
	ctx context.Context,
	userID int64,
) ([]ShortlistedGameDetail, error) {
	const query = `
    SELECT
      g.id, g.sport_type, g.price, g.format, g.venue_id, g.admin_id, g.max_players,
      g.game_level, g.start_time, g.end_time, g.visibility, g.instruction,
      g.status, g.is_booked, g.match_full, g.created_at, g.updated_at,
      v.name   AS venue_name,
      v.address AS venue_address
    FROM games g
    JOIN shortlisted_games sg ON g.id = sg.game_id
    JOIN venues v           ON g.venue_id = v.id
    WHERE sg.user_id = $1
    ORDER BY sg.created_at DESC
    `

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get shortlisted games: %w", err)
	}
	defer rows.Close()

	var list []ShortlistedGameDetail
	for rows.Next() {
		var d ShortlistedGameDetail
		if err := rows.Scan(
			&d.ID,
			&d.SportType,
			&d.Price,
			&d.Format,
			&d.VenueID,
			&d.AdminID,
			&d.MaxPlayers,
			&d.GameLevel,
			&d.StartTime,
			&d.EndTime,
			&d.Visibility,
			&d.Instruction,
			&d.Status,
			&d.IsBooked,
			&d.MatchFull,
			&d.CreatedAt,
			&d.UpdatedAt,
			&d.VenueName,
			&d.VenueAddress,
		); err != nil {
			return nil, fmt.Errorf("failed to scan shortlisted game row: %w", err)
		}
		list = append(list, d)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return list, nil
}

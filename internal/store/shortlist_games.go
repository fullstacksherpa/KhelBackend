package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ShortlistedGame represents a record in the shortlisted_games table.
type ShortlistedGame struct {
	UserID    int64     `json:"user_id"`
	GameID    int64     `json:"game_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ShortlistGamesStore handles database operations for shortlisted games.
type ShortlistGamesStore struct {
	db *sql.DB
}

// AddShortlist adds a game to the user's shortlist.
func (s *ShortlistGamesStore) AddShortlist(ctx context.Context, userID, gameID int64) error {
	query := `
		INSERT INTO shortlisted_games (user_id, game_id)
		VALUES ($1, $2)
		ON CONFLICT DO NOTHING
	`
	_, err := s.db.ExecContext(ctx, query, userID, gameID)
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
	_, err := s.db.ExecContext(ctx, query, userID, gameID)
	if err != nil {
		return fmt.Errorf("failed to remove shortlisted game: %w", err)
	}
	return nil
}

// GetShortlistedGamesByUser retrieves all games that a user has shortlisted.
// It joins the shortlisted_games table with the games table.
func (s *ShortlistGamesStore) GetShortlistedGamesByUser(ctx context.Context, userID int64) ([]Game, error) {
	query := `
		SELECT g.id, g.sport_type, g.price, g.format, g.venue_id, g.admin_id, g.max_players,
		       g.game_level, g.start_time, g.end_time, g.visibility, g.instruction,
		       g.status, g.is_booked, g.match_full, g.created_at, g.updated_at
		FROM games g
		JOIN shortlisted_games sg ON g.id = sg.game_id
		WHERE sg.user_id = $1
		ORDER BY sg.created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get shortlisted games: %w", err)
	}
	defer rows.Close()

	var games []Game
	for rows.Next() {
		var g Game
		if err := rows.Scan(
			&g.ID,
			&g.SportType,
			&g.Price,
			&g.Format,
			&g.VenueID,
			&g.AdminID,
			&g.MaxPlayers,
			&g.GameLevel,
			&g.StartTime,
			&g.EndTime,
			&g.Visibility,
			&g.Instruction,
			&g.Status,
			&g.IsBooked,
			&g.MatchFull,
			&g.CreatedAt,
			&g.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan game row: %w", err)
		}
		games = append(games, g)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return games, nil
}

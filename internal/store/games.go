package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Game represents a game in the system
type Game struct {
	ID          int64     `json:"id"`                    // Primary key
	SportType   string    `json:"sport_type"`            // Type of sport (e.g., futsal, basketball)
	Price       *int      `json:"price,omitempty"`       // Price of the game (nullable)
	Format      *string   `json:"format,omitempty"`      // Game format (nullable)
	VenueID     int64     `json:"venue_id"`              // Foreign key to venues table
	AdminID     int64     `json:"admin_id"`              // Foreign key to users table (game admin)
	MaxPlayers  int       `json:"max_players"`           // Maximum number of players
	GameLevel   *string   `json:"game_level,omitempty"`  // Skill level (beginner, intermediate, advanced)
	StartTime   time.Time `json:"start_time"`            // Game start time
	EndTime     time.Time `json:"end_time"`              // Game end time
	Visibility  string    `json:"visibility"`            // Visibility (public or private)
	Instruction *string   `json:"instruction,omitempty"` // Game instructions (nullable)
	Status      string    `json:"status"`                // Game status (active, cancelled, completed)
	IsBooked    bool      `json:"is_booked"`             // Whether the game is booked
	MatchFull   bool      `json:"match_full"`            // Whether the game is full
	CreatedAt   time.Time `json:"created_at"`            // Timestamp when the game was created
	UpdatedAt   time.Time `json:"updated_at"`            // Timestamp when the game was last updated
}

// GameRequest represents a request to join a game in the system
type GameRequest struct {
	ID          int64             `json:"id"`
	GameID      int64             `json:"game_id"`
	UserID      int64             `json:"user_id"`
	Status      GameRequestStatus `json:"status"`
	RequestTime time.Time         `json:"request_time"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type GameRequestStatus string

// Enum values for GameRequestStatus
const (
	GameRequestStatusPending  GameRequestStatus = "pending"
	GameRequestStatusAccepted GameRequestStatus = "accepted"
	GameRequestStatusRejected GameRequestStatus = "rejected"
)

type GamePlayer struct {
	ID       int64     `json:"id"`
	GameID   int64     `json:"game_id"`
	UserID   int64     `json:"user_id"`
	Role     string    `json:"role"`
	JoinedAt time.Time `json:"joined_at"`
}

type GameStore struct {
	db *sql.DB
}

// Create creates a new game in the database
func (s *GameStore) Create(ctx context.Context, game *Game) error {

	// Proceed with insertion if no overlaps
	query := `
		INSERT INTO games (
			sport_type, price, format, venue_id, admin_id, max_players, game_level,
			start_time, end_time, visibility, instruction, status, is_booked, match_full
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING id, created_at, updated_at
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	// Execute the query
	err := s.db.QueryRowContext(
		ctx, query,
		game.SportType,
		game.Price,
		game.Format,
		game.VenueID,
		game.AdminID,
		game.MaxPlayers,
		game.GameLevel,
		game.StartTime,
		game.EndTime,
		game.Visibility,
		game.Instruction,
		game.Status,
		game.IsBooked,
		game.MatchFull,
	).Scan(
		&game.ID,
		&game.CreatedAt,
		&game.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("error creating game: %w", err)
	}

	return nil
}

func (s *GameStore) GetGameByID(ctx context.Context, gameID int64) (*Game, error) {
	query := `
		SELECT id, sport_type, price, format, venue_id, admin_id, max_players, 
			   game_level, start_time, end_time, visibility, instruction, status, 
			   is_booked, match_full, created_at, updated_at
		FROM games 
		WHERE id = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	game := &Game{}
	err := s.db.QueryRowContext(ctx, query, gameID).Scan(
		&game.ID,
		&game.SportType,
		&game.Price,
		&game.Format,
		&game.VenueID,
		&game.AdminID,
		&game.MaxPlayers,
		&game.GameLevel,
		&game.StartTime,
		&game.EndTime,
		&game.Visibility,
		&game.Instruction,
		&game.Status,
		&game.IsBooked,
		&game.MatchFull,
		&game.CreatedAt,
		&game.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("game not found")
		}
		return nil, fmt.Errorf("error retrieving game: %w", err)
	}

	return game, nil
}

func (s *GameStore) CheckRequestExist(ctx context.Context, gameID int64, userID int64) (bool, error) {
	query := `
        SELECT 1 FROM game_join_requests 
        WHERE game_id = $1 AND user_id = $2 AND status = 'pending'`

	var exists int
	err := s.db.QueryRowContext(ctx, query, gameID, userID).Scan(&exists)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil // No existing request
		}
		return false, err // Return unexpected errors
	}

	return true, nil // Request exists
}

func (s *GameStore) AddToGameRequest(ctx context.Context, gameID int64, UserID int64) error {
	query := `
        INSERT INTO game_join_requests (game_id, user_id, status)
        VALUES ($1, $2, 'pending')`
	_, err := s.db.ExecContext(ctx, query,
		gameID, UserID)
	if err != nil {
		return err
	}
	return nil
}

func (s *GameStore) IsAdminAssistant(ctx context.Context, gameID int64, userID int64) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM game_players 
			WHERE game_id = $1 AND user_id = $2 AND role IN ('admin', 'assistant')
		)`

	var isAdmin bool
	err := s.db.QueryRowContext(ctx, query, gameID, userID).Scan(&isAdmin)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil // No admin found, but not an error
		}
		return false, err // Unexpected database error
	}

	return isAdmin, nil
}

func (s *GameStore) SetMatchFull(ctx context.Context, gameID int64) error {
	query := `
                UPDATE games 
                SET match_full = true 
                WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query,
		gameID)
	if err != nil {
		return err
	}
	return nil
}

func (s *GameStore) InsertNewPlayer(ctx context.Context, gameID int64, userID int64) error {
	query := `
            INSERT INTO game_players (game_id, user_id, role)
            VALUES ($1, $2, 'player')`
	_, err := s.db.ExecContext(ctx, query,
		gameID, userID)
	if err != nil {
		return err
	}
	return nil
}

func (s *GameStore) UpdateRequestStatus(ctx context.Context, gameID, userID int64, status GameRequestStatus) error {
	query := `
		UPDATE game_join_requests
		SET status = $1
		WHERE game_id = $2 AND user_id = $3
		RETURNING id`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	// Execute the update query
	var requestID int64
	err := s.db.QueryRowContext(ctx, query, status, gameID, userID).Scan(&requestID)
	if err != nil {
		// Return an error if no rows were updated or there's a database issue
		if err == sql.ErrNoRows {
			return fmt.Errorf("no pending request found for the game and user")
		}
		return fmt.Errorf("error updating request status: %w", err)
	}
	return nil
}

func (s *GameStore) GetJoinRequest(ctx context.Context, gameID, userID int64) (*GameRequest, error) {
	query := `
	  SELECT id, game_id, user_id, status, request_time, updated_at
		FROM game_join_requests
		WHERE game_id = $1 AND user_id = $2
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	var req GameRequest
	err := s.db.QueryRowContext(ctx, query, gameID, userID).Scan(
		&req.ID,
		&req.GameID,
		&req.UserID,
		&req.Status,
		&req.RequestTime,
		&req.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("error retrieving join request: %w", err)
	}
	return &req, nil
}

func (s *GameStore) GetPlayerCount(ctx context.Context, gameID int) (int, error) {
	query := `
	 SELECT COUNT(*) 
		FROM game_players 
		WHERE game_id = $1
	`
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	var count int

	err := s.db.QueryRowContext(ctx, query, gameID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("error getting player count: %w", err)
	}
	return count, nil
}

func (s *GameStore) GetGamePlayers(ctx context.Context, gameID int64) ([]*User, error) {
	query := `
		SELECT 
			u.id, 
			u.first_name, 
			u.profile_picture_url, 
			u.skill_level, 
			u.phone
		FROM 
			game_players gp
		JOIN 
			users u ON gp.user_id = u.id
		WHERE 
			gp.game_id = $1
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("error querying game players: %w", err)
	}
	defer rows.Close()

	players := make([]*User, 0)
	for rows.Next() {
		var player User
		err := rows.Scan(
			&player.ID,
			&player.FirstName,
			&player.ProfilePictureURL,
			&player.SkillLevel,
			&player.Phone,
		)
		if err != nil {
			return nil, fmt.Errorf("error scanning player: %w", err)
		}
		players = append(players, &player)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over rows: %w", err)
	}

	if len(players) == 0 {
		return nil, ErrNotFound
	}

	return players, nil
}

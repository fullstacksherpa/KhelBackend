package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/lib/pq"
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

type GameWithVenue struct {
	ID         int64
	SportType  string
	Price      int
	Format     string
	VenueID    int
	AdminID    int
	MaxPlayers int
	GameLevel  string
	StartTime  time.Time
	EndTime    time.Time
	Visibility string
	Status     string
	IsBooked   bool
	MatchFull  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time

	// Venue details for Mapbox
	VenueName string
	Address   string
	Latitude  float64
	Longitude float64
	Amenities []string
	ImageURLs []string
}

type GameStore struct {
	db *sql.DB
}

func (s *GameStore) Create(ctx context.Context, game *Game) (int64, error) {
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
		return 0, fmt.Errorf("error creating game: %w", err)
	}

	return game.ID, nil
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
		return false, err // Unexpected database error
	}

	return isAdmin, nil
}
func (s *GameStore) IsAdmin(ctx context.Context, gameID, userID int64) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1 FROM game_players 
			WHERE game_id = $1 AND user_id = $2 AND role = 'admin'
		)`
	var isAdmin bool
	err := s.db.QueryRowContext(ctx, query, gameID, userID).Scan(&isAdmin)
	if err != nil {
		return false, err // No need to check sql.ErrNoRows
	}
	return isAdmin, nil
}

func (s *GameStore) ToggleMatchFull(ctx context.Context, gameID int64) error {
	// First, check the current value of match_full
	var currentValue bool
	query := `SELECT match_full FROM games WHERE id = $1`
	err := s.db.QueryRowContext(ctx, query, gameID).Scan(&currentValue)
	if err != nil {
		return fmt.Errorf("error checking match_full: %w", err)
	}

	// Toggle the value
	toggledValue := !currentValue

	// Update the match_full field with the toggled value
	updateQuery := `
		UPDATE games 
		SET match_full = $1 
		WHERE id = $2`
	_, err = s.db.ExecContext(ctx, updateQuery, toggledValue, gameID)
	if err != nil {
		return fmt.Errorf("error updating match_full: %w", err)
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

func (s *GameStore) InsertAdminInPlayer(ctx context.Context, gameID int64, userID int64) error {
	query := `
            INSERT INTO game_players (game_id, user_id, role)
            VALUES ($1, $2, 'admin')`
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

func (s *GameStore) AssignAssistant(ctx context.Context, gameID, playerID int64) error {

	// Update player's role to 'assistant'
	query := `
		UPDATE game_players 
		SET role = 'assistant' 
		WHERE game_id = $1 AND user_id = $2 AND role = 'player'
	`
	res, err := s.db.ExecContext(ctx, query, gameID, playerID)
	if err != nil {
		return err
	}

	// Check if the update affected any row
	rowsAffected, _ := res.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("player not found or already an assistant")
	}

	return nil
}

// GetGames queries the database for games that match the provided filters.
func (s *GameStore) GetGames(ctx context.Context, q GameFilterQuery) ([]GameWithVenue, error) {
	query := `
        SELECT 
            g.id,
            g.sport_type,
            g.price,
            g.format,
            g.venue_id,
            g.admin_id,
            g.max_players,
            g.game_level,
            g.start_time,
            g.end_time,
            g.visibility,
            g.status,
            g.is_booked,
            g.match_full,
            g.created_at,
            g.updated_at,
            v.name AS venue_name,
            v.address,
            ST_Y(v.location::geometry) AS latitude,
            ST_X(v.location::geometry) AS longitude,
            v.amenities,
            v.image_urls
        FROM games g
        JOIN venues v ON g.venue_id = v.id
        WHERE 1=1
            AND ($1::varchar IS NULL OR g.sport_type = $1)
            AND ($2::varchar IS NULL OR g.game_level = $2)
            AND ($3::int IS NULL OR g.venue_id = $3)
            AND ($4::bool IS NULL OR g.is_booked = $4)
            AND ($5::timestamp IS NULL OR g.start_time >= $5)
            AND ($6::timestamp IS NULL OR g.end_time <= $6)
            AND ($7::int IS NULL OR g.price >= $7)
            AND ($8::int IS NULL OR g.price <= $8)
            AND ($9::int = 0 OR ST_DWithin(
                v.location, 
                ST_MakePoint($10, $11)::geography, 
                $9 * 1000)  -- Convert km to meters
            )
        ORDER BY g.start_time ` + q.Sort + `
        LIMIT $12 OFFSET $13`

	// Create a timeout context for the query.
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	// We use helper functions to substitute nil values when defaults apply.
	rows, err := s.db.QueryContext(ctx, query,
		nullIfEmpty(q.SportType),
		nullIfEmpty(q.GameLevel),
		nullIfZero(q.VenueID),
		q.IsBooked, // pointer; nil if not filtered
		nullTime(q.StartAfter),
		nullTime(q.EndBefore),
		nullIfZero(q.MinPrice),
		nullIfZero(q.MaxPrice),
		q.Radius,
		q.UserLon, // Note: longitude (X coordinate)
		q.UserLat, // Note: latitude (Y coordinate)
		q.Limit,
		q.Offset,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []GameWithVenue
	for rows.Next() {
		var g GameWithVenue
		err := rows.Scan(
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
			&g.Status,
			&g.IsBooked,
			&g.MatchFull,
			&g.CreatedAt,
			&g.UpdatedAt,
			&g.VenueName,
			&g.Address,
			&g.Latitude,
			&g.Longitude,
			pq.Array(&g.Amenities),
			pq.Array(&g.ImageURLs),
		)
		if err != nil {
			return nil, err
		}
		games = append(games, g)
	}

	return games, nil
}

// Helper functions to return nil if the value is the default.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZero(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}

func nullTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func (s *GameStore) CancelGame(ctx context.Context, gameID int64) error {
	query := `UPDATE games SET status = 'cancelled' WHERE id = $1`
	result, err := s.db.ExecContext(ctx, query, gameID)
	if err != nil {
		return err
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

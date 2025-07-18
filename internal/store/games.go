package store

import (
	"context"

	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
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
type GameSummary struct {
	GameID        int64     `json:"game_id"`
	VenueID       int64     `json:"venue_id"`
	VenueName     string    `json:"venue_name"`
	SportType     string    `json:"sport_type"`
	Price         *int      `json:"price,omitempty"`
	Format        *string   `json:"format,omitempty"`
	GameAdminName string    `json:"game_admin_name"`
	GameLevel     *string   `json:"game_level,omitempty"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	MaxPlayers    int       `json:"max_players"`
	CurrentPlayer int       `json:"current_player"`
	PlayerImages  []string  `json:"player_images"`
	IsBooked      bool      `json:"is_booked"`
	MatchFull     bool      `json:"match_full"`
	VenueLat      float64   `json:"venue_lat"` // Venue latitude
	VenueLon      float64   `json:"venue_lon"` // Venue longitude
	Shortlisted   bool      `json:"shortlisted"`
	Status        string    `json:"status"`
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

// GameDetails holds full info for a single game, including admin, booking and player lists.
type GameDetails struct {
	GameID             int64     `json:"game_id"`
	VenueID            int64     `json:"venue_id"`
	VenueName          string    `json:"venue_name"`
	SportType          string    `json:"sport_type"`
	Price              *int      `json:"price,omitempty"`
	Format             *string   `json:"format,omitempty"`
	GameLevel          *string   `json:"game_level,omitempty"`
	AdminID            int64     `json:"admin_id"`
	GameAdminName      string    `json:"game_admin_name"`
	StartTime          time.Time `json:"start_time"`
	EndTime            time.Time `json:"end_time"`
	MaxPlayers         int       `json:"max_players"`
	CurrentPlayer      int       `json:"current_player"`
	PlayerImages       []string  `json:"player_images"`
	PlayerIDs          []int64   `json:"player_ids"`           // all joined player user IDs
	RequestedPlayerIDs []int64   `json:"requested_player_ids"` // pending request user IDs
	IsBooked           bool      `json:"is_booked"`
	MatchFull          bool      `json:"match_full"`
	Status             string    `json:"status"`
	VenueLat           float64   `json:"venue_lat"`
	VenueLon           float64   `json:"venue_lon"`
}

type GameStore struct {
	db *pgxpool.Pool
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

	err := s.db.QueryRow(
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
	err := s.db.QueryRow(ctx, query, gameID).Scan(
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
		if errors.Is(err, pgx.ErrNoRows) {
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
	err := s.db.QueryRow(ctx, query, gameID, userID).Scan(&exists)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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
	_, err := s.db.Exec(ctx, query,
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
	err := s.db.QueryRow(ctx, query, gameID, userID).Scan(&isAdmin)
	if err != nil {
		return false, err
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
	err := s.db.QueryRow(ctx, query, gameID, userID).Scan(&isAdmin)
	if err != nil {
		return false, err
	}
	return isAdmin, nil
}

func (s *GameStore) ToggleMatchFull(ctx context.Context, gameID int64) error {
	var currentValue bool
	query := `SELECT match_full FROM games WHERE id = $1`
	err := s.db.QueryRow(ctx, query, gameID).Scan(&currentValue)
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
	_, err = s.db.Exec(ctx, updateQuery, toggledValue, gameID)
	if err != nil {
		return fmt.Errorf("error updating match_full: %w", err)
	}

	return nil
}

func (s *GameStore) InsertNewPlayer(ctx context.Context, gameID, userID int64) error {
	var maxPlayers int
	query := `SELECT max_players FROM games WHERE id = $1`
	err := s.db.QueryRow(ctx, query, gameID).Scan(&maxPlayers)
	if err != nil {
		if err == pgx.ErrNoRows {
			return fmt.Errorf("game not found")
		}
		return fmt.Errorf("error fetching game details: %w", err)
	}

	// Step 2: Count current players in the game
	var currentPlayers int
	query = `SELECT COUNT(*) FROM game_players WHERE game_id = $1`
	err = s.db.QueryRow(ctx, query, gameID).Scan(&currentPlayers)
	if err != nil {
		return fmt.Errorf("error counting current players: %w", err)
	}

	// Step 3: Check if max players limit is reached
	if currentPlayers >= maxPlayers {
		return fmt.Errorf("cannot join: game is full")
	}

	// Step 4: Insert player if limit is not reached
	insertQuery := `INSERT INTO game_players (game_id, user_id, role, joined_at) VALUES ($1, $2, 'player', NOW())`
	_, err = s.db.Exec(ctx, insertQuery, gameID, userID)
	if err != nil {
		return fmt.Errorf("error inserting player into game: %w", err)
	}

	return nil
}

func (s *GameStore) InsertAdminInPlayer(ctx context.Context, gameID int64, userID int64) error {
	query := `
		INSERT INTO game_players (game_id, user_id, role)
		VALUES ($1, $2, 'admin')`
	_, err := s.db.Exec(ctx, query, gameID, userID)
	if err != nil {
		return fmt.Errorf("InsertAdminInPlayer error (gameID=%d, userID=%d): %w", gameID, userID, err)
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
	err := s.db.QueryRow(ctx, query, status, gameID, userID).Scan(&requestID)
	if err != nil {
		if err == pgx.ErrNoRows {
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
	err := s.db.QueryRow(ctx, query, gameID, userID).Scan(
		&req.ID,
		&req.GameID,
		&req.UserID,
		&req.Status,
		&req.RequestTime,
		&req.UpdatedAt,
	)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
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

	err := s.db.QueryRow(ctx, query, gameID).Scan(&count)
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

	rows, err := s.db.Query(ctx, query, gameID)
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
	res, err := s.db.Exec(ctx, query, gameID, playerID)
	if err != nil {
		return err
	}

	// Check if the update affected any row
	rowsAffected := res.RowsAffected()
	if rowsAffected == 0 {
		return errors.New("player not found or already an assistant")
	}

	return nil
}

// GetGames queries the database for games that match the provided filters.
func (s *GameStore) GetGames(ctx context.Context, q GameFilterQuery) ([]GameSummary, error) {
	// build the base of your SQL once
	baseQuery := `
SELECT 
    g.id AS game_id,
    g.venue_id,
    v.name AS venue_name,
    g.sport_type,
    g.price,
    g.format,
    u.first_name AS game_admin_name,
    g.game_level,
    g.start_time,
    g.end_time,
    g.max_players,
    (SELECT COUNT(*) FROM game_players gp WHERE gp.game_id = g.id) AS current_player,
    COALESCE((
      SELECT array_agg(t.profile_picture_url)
      FROM (
         SELECT u2.profile_picture_url
         FROM game_players gp2
         JOIN users u2 ON gp2.user_id = u2.id
         WHERE gp2.game_id = g.id
           AND u2.profile_picture_url IS NOT NULL
         ORDER BY gp2.joined_at
         LIMIT 4
      ) AS t
    ), '{}') AS player_images,
    g.is_booked,
    g.match_full,
    g.status,
    ST_Y(v.location::geometry) AS venue_lat,
    ST_X(v.location::geometry) AS venue_lon
FROM games g
JOIN venues v ON g.venue_id = v.id
JOIN users u ON g.admin_id = u.id
WHERE 1 = 1
  AND ($1::varchar IS NULL OR g.sport_type = $1)
  AND ($2::varchar IS NULL OR g.game_level = $2)
  AND ($3::int IS NULL OR g.venue_id = $3)
  AND ($4::bool IS NULL OR g.is_booked = $4)
  AND ($5::varchar IS NULL OR g.status = $5)
  AND ($6::timestamp IS NULL OR g.start_time >= $6)
  AND ($7::timestamp IS NULL OR g.end_time <= $7)
  AND ($8::int IS NULL OR g.price >= $8)
  AND ($9::int IS NULL OR g.price <= $9)
  AND ($10::int = 0 OR ST_DWithin(
           v.location, 
           ST_MakePoint($11, $12)::geography, 
           $10 * 1000
  ))
ORDER BY g.start_time 
`

	// concatenate the sort direction and the LIMIT/OFFSET clause
	query := baseQuery + q.Sort + `
LIMIT $13 OFFSET $14
`

	// apply your timeout
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	rows, err := s.db.Query(ctx, query,
		nullIfEmpty(q.SportType), // $1
		nullIfEmpty(q.GameLevel), // $2
		nullIfZero(q.VenueID),    // $3
		q.IsBooked,               // $4
		q.Status,                 // $5
		nullTime(q.StartAfter),   // $6
		nullTime(q.EndBefore),    // $7
		nullIfZero(q.MinPrice),   // $8
		nullIfZero(q.MaxPrice),   // $9
		q.Radius,                 // $10
		q.UserLon,                // $11
		q.UserLat,                // $12
		q.Limit,                  // $13
		q.Offset,                 // $14
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var games []GameSummary
	for rows.Next() {
		var g GameSummary
		if err := rows.Scan(
			&g.GameID,
			&g.VenueID,
			&g.VenueName,
			&g.SportType,
			&g.Price,
			&g.Format,
			&g.GameAdminName,
			&g.GameLevel,
			&g.StartTime,
			&g.EndTime,
			&g.MaxPlayers,
			&g.CurrentPlayer,
			&g.PlayerImages,
			&g.IsBooked,
			&g.MatchFull,
			&g.Status,
			&g.VenueLat,
			&g.VenueLon,
		); err != nil {
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
	result, err := s.db.Exec(ctx, query, gameID)
	if err != nil {
		return err
	}
	rowsAffected := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("no Game found for cancel: id=%d", gameID)
	}
	return nil
}

type GameRequestWithUser struct {
	ID                int64             `json:"id"`
	GameID            int64             `json:"game_id"`
	UserID            int64             `json:"user_id"`
	Status            GameRequestStatus `json:"status"`
	RequestTime       time.Time         `json:"request_time"`
	UpdatedAt         time.Time         `json:"updated_at"`
	FirstName         string            `json:"first_name"`
	Phone             string            `json:"phone"`
	ProfilePictureURL *string           `json:"profile_picture_url" swaggertype:"string"`
	SkillLevel        *string           `json:"skill_level" swaggertype:"string"`
}

func (s *GameStore) GetAllJoinRequests(ctx context.Context, gameID int64) ([]*GameRequestWithUser, error) {
	query := `
        SELECT 
			gr.id, gr.game_id, gr.user_id, gr.status, gr.request_time, gr.updated_at,
			u.first_name, u.phone, u.profile_picture_url, u.skill_level
		FROM game_join_requests gr
		JOIN users u ON gr.user_id = u.id
		WHERE gr.game_id = $1 AND gr.status = 'pending'
`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()
	rows, err := s.db.Query(ctx, query, gameID)
	if err != nil {
		return nil, fmt.Errorf("error retrieving join requests: %w", err)
	}
	defer rows.Close()
	var requests []*GameRequestWithUser
	for rows.Next() {
		var req GameRequestWithUser
		if err := rows.Scan(
			&req.ID,
			&req.GameID,
			&req.UserID,
			&req.Status,
			&req.RequestTime,
			&req.UpdatedAt,
			&req.FirstName,
			&req.Phone,
			&req.ProfilePictureURL,
			&req.SkillLevel,
		); err != nil {
			return nil, fmt.Errorf("error scanning join request: %w", err)
		}
		requests = append(requests, &req)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over join requests: %w", err)
	}

	return requests, nil
}

// GetGameDetailsWithID returns detailed info for a single game, including booking status,
// the admin ID, all joined players' IDs, and pending join-request IDs.
func (s *GameStore) GetGameDetailsWithID(ctx context.Context, gameID int64) (*GameDetails, error) {
	query := `
SELECT
	g.id               AS game_id,
	g.venue_id,
	v.name             AS venue_name,
	g.sport_type,
	g.price,
	g.format,
	g.game_level,
	g.admin_id,
	u.first_name       AS game_admin_name,
	g.start_time,
	g.end_time,
	g.max_players,
	(
		SELECT COUNT(*)
		FROM game_players gp
		WHERE gp.game_id = g.id
	)                   AS current_player,
	COALESCE(
		(
			SELECT array_agg(t.profile_picture_url)
			FROM (
				SELECT u2.profile_picture_url
				FROM game_players gp2
				JOIN users u2 ON gp2.user_id = u2.id
				WHERE gp2.game_id = g.id
				  AND u2.profile_picture_url IS NOT NULL
				ORDER BY gp2.joined_at
				LIMIT 4
			) AS t
		),
		'{}'
	)                   AS player_images,
	COALESCE(
		(
			SELECT array_agg(gp2.user_id)
			FROM game_players gp2
			WHERE gp2.game_id = g.id
		),
		'{}'
	)                   AS player_ids,
	COALESCE(
		(
			SELECT array_agg(gr.user_id)
			FROM game_join_requests gr
			WHERE gr.game_id = g.id
			  AND gr.status = 'pending'
		),
		'{}'
	)                   AS requested_player_ids,
					    g.is_booked,
    g.match_full,
	g.status,
	ST_Y(v.location::geometry) AS venue_lat,
	ST_X(v.location::geometry) AS venue_lon
FROM games g
JOIN venues v ON g.venue_id = v.id
JOIN users u ON g.admin_id = u.id
WHERE g.id = $1
`

	// enforce timeout
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	var gd GameDetails
	err := s.db.QueryRow(ctx, query, gameID).Scan(
		&gd.GameID,
		&gd.VenueID,
		&gd.VenueName,
		&gd.SportType,
		&gd.Price,
		&gd.Format,
		&gd.GameLevel,
		&gd.AdminID,
		&gd.GameAdminName,
		&gd.StartTime,
		&gd.EndTime,
		&gd.MaxPlayers,
		&gd.CurrentPlayer,
		&gd.PlayerImages,
		&gd.PlayerIDs,
		&gd.RequestedPlayerIDs,
		&gd.IsBooked,
		&gd.MatchFull,
		&gd.Status,
		&gd.VenueLat,
		&gd.VenueLon,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("error retrieving game details: %w", err)
	}

	return &gd, nil
}

// GetUpcomingGamesByVenue queries the database for upcoming active games at a specific venue.
func (s *GameStore) GetUpcomingGamesByVenue(ctx context.Context, venueID int64) ([]GameSummary, error) {
	// Build the base query with filtering for upcoming games and active status.
	query := `
		SELECT 
		    g.id AS game_id,
		    g.venue_id,
		    v.name AS venue_name,
		    g.sport_type,
		    g.price,
		    g.format,
		    u.first_name AS game_admin_name,
		    g.game_level,
		    g.start_time,
		    g.end_time,
		    g.max_players,
		    (SELECT COUNT(*) FROM game_players gp WHERE gp.game_id = g.id) AS current_player,
		    COALESCE((
  SELECT array_agg(t.profile_picture_url)
  FROM (
     SELECT u2.profile_picture_url
     FROM game_players gp2
     JOIN users u2 ON gp2.user_id = u2.id
     WHERE gp2.game_id = g.id
       AND u2.profile_picture_url IS NOT NULL
     ORDER BY gp2.joined_at
     LIMIT 4
  ) AS t
), ARRAY[]::text[]) AS player_images,
		    g.is_booked,
		    g.match_full,
		    g.status,
		    ST_Y(v.location::geometry) AS venue_lat,
		    ST_X(v.location::geometry) AS venue_lon
		FROM games g
		JOIN venues v ON g.venue_id = v.id
		JOIN users u ON g.admin_id = u.id
		WHERE g.venue_id = $1
		  AND g.start_time >= NOW()
		  AND g.status = 'active'
		ORDER BY g.start_time ASC
	`

	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	rows, err := s.db.Query(ctx, query, venueID)
	if err != nil {
		return nil, fmt.Errorf("failed to get upcoming games for venue: %w", err)
	}
	defer rows.Close()

	var games []GameSummary
	for rows.Next() {
		var g GameSummary
		if err := rows.Scan(
			&g.GameID,
			&g.VenueID,
			&g.VenueName,
			&g.SportType,
			&g.Price,
			&g.Format,
			&g.GameAdminName,
			&g.GameLevel,
			&g.StartTime,
			&g.EndTime,
			&g.MaxPlayers,
			&g.CurrentPlayer,
			&g.PlayerImages,
			&g.IsBooked,
			&g.MatchFull,
			&g.Status,
			&g.VenueLat,
			&g.VenueLon,
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

// GetUpcomingGamesByUser returns all active games the user has joined
// whose start_time is in the future, ordered soonest-first.
func (s *GameStore) GetUpcomingGamesByUser(ctx context.Context, userID int64) ([]GameSummary, error) {
	const query = `
SELECT 
    g.id                AS game_id,
    g.venue_id,
    v.name             AS venue_name,
    g.sport_type,
    g.price,
    g.format,
    u.first_name       AS game_admin_name,
    g.game_level,
    g.start_time,
    g.end_time,
    g.max_players,
    (SELECT COUNT(*) FROM game_players gp2 WHERE gp2.game_id = g.id) AS current_player,
    COALESCE((
      SELECT array_agg(t.profile_picture_url)
      FROM (
         SELECT u2.profile_picture_url
         FROM game_players gp3
         JOIN users u2 ON gp3.user_id = u2.id
         WHERE gp3.game_id = g.id
           AND u2.profile_picture_url IS NOT NULL
         ORDER BY gp3.joined_at
         LIMIT 4
      ) AS t
    ), '{}')                   AS player_images,
    g.is_booked,
    g.match_full,
    g.status,
    ST_Y(v.location::geometry) AS venue_lat,
    ST_X(v.location::geometry) AS venue_lon
FROM games g
JOIN game_players gp ON gp.game_id = g.id
JOIN venues v      ON g.venue_id = v.id
JOIN users u       ON g.admin_id = u.id
WHERE gp.user_id = $1
  AND g.start_time >= NOW()
  AND g.status = 'active'
ORDER BY g.start_time ASC
`

	// apply timeout (reuse your QueryTimeoutDuration)
	ctx, cancel := context.WithTimeout(ctx, QueryTimeoutDuration)
	defer cancel()

	rows, err := s.db.Query(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("GetUpcomingGamesByUser query error: %w", err)
	}
	defer rows.Close()

	var games []GameSummary
	for rows.Next() {
		var g GameSummary
		if err := rows.Scan(
			&g.GameID,
			&g.VenueID,
			&g.VenueName,
			&g.SportType,
			&g.Price,
			&g.Format,
			&g.GameAdminName,
			&g.GameLevel,
			&g.StartTime,
			&g.EndTime,
			&g.MaxPlayers,
			&g.CurrentPlayer,
			&g.PlayerImages,
			&g.IsBooked,
			&g.MatchFull,
			&g.Status,
			&g.VenueLat,
			&g.VenueLon,
		); err != nil {
			return nil, fmt.Errorf("GetUpcomingGamesByUser scan error: %w", err)
		}
		games = append(games, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("GetUpcomingGamesByUser rows iteration error: %w", err)
	}
	return games, nil
}

func (s *GameStore) MarkCompletedGames() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	query := `
		UPDATE games
		SET status = 'completed'
		WHERE end_time < NOW()
		  AND status = 'active'
	`

	ct, err := s.db.Exec(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to update games: %w", err)
	}
	fmt.Printf("Marked %d games as completed at %s\n", ct.RowsAffected(), time.Now().Format(time.RFC1123))
	return nil
}

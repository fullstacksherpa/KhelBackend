package games

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

var (
	ErrNotFound             = errors.New("resource not found")
	ErrConflict             = errors.New("resource already exists")
	ErrDuplicateEmail       = errors.New("a user with that email already exists")
	ErrDuplicatePhoneNumber = errors.New("a user with that phone number already exists")
	QueryTimeoutDuration    = time.Second * 5
)

type BookingStatus string

const (
	BookingPending   BookingStatus = "pending"
	BookingRequested BookingStatus = "requested"
	BookingBooked    BookingStatus = "booked"
	BookingRejected  BookingStatus = "rejected"
	BookingCancelled BookingStatus = "cancelled"
)

// Game represents a game in the system
type Game struct {
	ID            int64         `json:"id"`                    // Primary key
	SportType     string        `json:"sport_type"`            // Type of sport (e.g., futsal, basketball)
	Price         *int          `json:"price,omitempty"`       // Price of the game (nullable)
	Format        *string       `json:"format,omitempty"`      // Game format (nullable)
	VenueID       int64         `json:"venue_id"`              // Foreign key to venues table
	AdminID       int64         `json:"admin_id"`              // Foreign key to users table (game admin)
	MaxPlayers    int           `json:"max_players"`           // Maximum number of players
	GameLevel     *string       `json:"game_level,omitempty"`  // Skill level (beginner, intermediate, advanced)
	StartTime     time.Time     `json:"start_time"`            // Game start time
	EndTime       time.Time     `json:"end_time"`              // Game end time
	Visibility    string        `json:"visibility"`            // Visibility (public or private)
	Instruction   *string       `json:"instruction,omitempty"` // Game instructions (nullable)
	Status        string        `json:"status"`                // Game status (active, cancelled, completed)
	BookingStatus BookingStatus `json:"booking_status"`
	MatchFull     bool          `json:"match_full"` // Whether the game is full
	CreatedAt     time.Time     `json:"created_at"` // Timestamp when the game was created
	UpdatedAt     time.Time     `json:"updated_at"` // Timestamp when the game was last updated
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
	GameID        int64         `json:"game_id"`
	VenueID       int64         `json:"venue_id"`
	VenueName     string        `json:"venue_name"`
	SportType     string        `json:"sport_type"`
	Price         *int          `json:"price,omitempty"`
	Format        *string       `json:"format,omitempty"`
	GameAdminName string        `json:"game_admin_name"`
	GameLevel     *string       `json:"game_level,omitempty"`
	StartTime     time.Time     `json:"start_time"`
	EndTime       time.Time     `json:"end_time"`
	MaxPlayers    int           `json:"max_players"`
	CurrentPlayer int           `json:"current_player"`
	PlayerImages  []string      `json:"player_images"`
	BookingStatus BookingStatus `json:"booking_status"`
	MatchFull     bool          `json:"match_full"`
	VenueLat      float64       `json:"venue_lat"` // Venue latitude
	VenueLon      float64       `json:"venue_lon"` // Venue longitude
	Shortlisted   bool          `json:"shortlisted"`
	Status        string        `json:"status"`
}

type GameWithVenue struct {
	ID            int64
	SportType     string
	Price         int
	Format        string
	VenueID       int
	AdminID       int
	MaxPlayers    int
	GameLevel     string
	StartTime     time.Time
	EndTime       time.Time
	Visibility    string
	Status        string
	BookingStatus BookingStatus
	MatchFull     bool
	CreatedAt     time.Time
	UpdatedAt     time.Time

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
	GameID             int64         `json:"game_id"`
	VenueID            int64         `json:"venue_id"`
	VenueName          string        `json:"venue_name"`
	SportType          string        `json:"sport_type"`
	Price              *int          `json:"price,omitempty"`
	Format             *string       `json:"format,omitempty"`
	GameLevel          *string       `json:"game_level,omitempty"`
	AdminID            int64         `json:"admin_id"`
	GameAdminName      string        `json:"game_admin_name"`
	StartTime          time.Time     `json:"start_time"`
	EndTime            time.Time     `json:"end_time"`
	MaxPlayers         int           `json:"max_players"`
	CurrentPlayer      int           `json:"current_player"`
	PlayerImages       []string      `json:"player_images"`
	PlayerIDs          []int64       `json:"player_ids"`           // all joined player user IDs
	RequestedPlayerIDs []int64       `json:"requested_player_ids"` // pending request user IDs
	BookingStatus      BookingStatus `json:"booking_status"`
	MatchFull          bool          `json:"match_full"`
	Status             string        `json:"status"`
	VenueLat           float64       `json:"venue_lat"`
	VenueLon           float64       `json:"venue_lon"`
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

type GameFilterQuery struct {
	Limit         int            `validate:"gte=1"`          // Maximum number of results to return
	Offset        int            `validate:"gte=0"`          // Pagination offset
	Sort          string         `validate:"oneof=asc desc"` // Sorting order for start_time
	SportType     string         // Filter by sport type (e.g., "basketball")
	GameLevel     string         // Filter by game level (e.g., "intermediate")
	VenueID       int            // Filter by a specific venue id
	BookingStatus *BookingStatus // Filter based on booking status (nil = no filter)
	Status        *string        `validate:"omitempty,oneof=active cancelled completed"`

	// Location-based filtering
	UserLat float64 // User's latitude for radius filter
	UserLon float64 // User's longitude for radius filter
	Radius  int     // Radius in kilometers; 0 means no radius filtering

	// Time filtering
	StartAfter time.Time // Return games starting after this time
	EndBefore  time.Time // Return games ending before this time

	// Price filtering
	MinPrice int
	MaxPrice int
}

// Parse extracts query parameters from the request URL and populates the GameFilterQuery.
func (q GameFilterQuery) Parse(r *http.Request) (GameFilterQuery, error) {
	params := r.URL.Query()

	if sportType := params.Get("sport_type"); sportType != "" {
		q.SportType = sportType
	}

	if gameLevel := params.Get("game_level"); gameLevel != "" {
		q.GameLevel = gameLevel
	}

	if venueIDStr := params.Get("venue_id"); venueIDStr != "" {
		venueID, err := strconv.Atoi(venueIDStr)
		if err != nil {
			return q, fmt.Errorf("invalid venue_id: %w", err)
		}
		q.VenueID = venueID
	}

	if status := params.Get("booking_status"); status != "" {
		bs := BookingStatus(status) // convert string -> BookingStatus
		switch bs {
		case BookingPending, BookingRequested, BookingBooked, BookingRejected, BookingCancelled:
			q.BookingStatus = &bs
		default:
			return q, fmt.Errorf("invalid booking_status value: %s", status)
		}
	}

	if status := params.Get("status"); status != "" {
		// you may want to validate it's one of your enum values here
		q.Status = &status
	}

	if latStr := params.Get("lat"); latStr != "" {
		lat, err := strconv.ParseFloat(latStr, 64)
		if err != nil {
			return q, fmt.Errorf("invalid lat value: %w", err)
		}
		q.UserLat = lat
	}

	if lonStr := params.Get("lon"); lonStr != "" {
		lon, err := strconv.ParseFloat(lonStr, 64)
		if err != nil {
			return q, fmt.Errorf("invalid lon value: %w", err)
		}
		q.UserLon = lon
	}

	if radiusStr := params.Get("radius"); radiusStr != "" {
		radius, err := strconv.Atoi(radiusStr)
		if err != nil {
			return q, fmt.Errorf("invalid radius value: %w", err)
		}
		q.Radius = radius
	}

	if startAfterStr := params.Get("start_after"); startAfterStr != "" {
		startAfter, err := time.Parse(time.RFC3339, startAfterStr)
		if err != nil {
			return q, fmt.Errorf("invalid start_after value: %w", err)
		}
		q.StartAfter = startAfter
	}

	if endBeforeStr := params.Get("end_before"); endBeforeStr != "" {
		endBefore, err := time.Parse(time.RFC3339, endBeforeStr)
		if err != nil {
			return q, fmt.Errorf("invalid end_before value: %w", err)
		}
		q.EndBefore = endBefore
	}

	if minPriceStr := params.Get("min_price"); minPriceStr != "" {
		minPrice, err := strconv.Atoi(minPriceStr)
		if err != nil {
			return q, fmt.Errorf("invalid min_price: %w", err)
		}
		q.MinPrice = minPrice
	}

	if maxPriceStr := params.Get("max_price"); maxPriceStr != "" {
		maxPrice, err := strconv.Atoi(maxPriceStr)
		if err != nil {
			return q, fmt.Errorf("invalid max_price: %w", err)
		}
		q.MaxPrice = maxPrice
	}

	// Optional: Allow overriding the default pagination values.
	if limitStr := params.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return q, fmt.Errorf("invalid limit: %w", err)
		}
		q.Limit = limit
	}

	if offsetStr := params.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return q, fmt.Errorf("invalid offset: %w", err)
		}
		q.Offset = offset
	}

	if sort := params.Get("sort"); sort != "" {
		if sort != "asc" && sort != "desc" {
			return q, fmt.Errorf("invalid sort value: must be 'asc' or 'desc'")
		}
		q.Sort = sort
	}

	return q, nil
}

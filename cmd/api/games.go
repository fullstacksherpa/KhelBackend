package main

import (
	"database/sql"
	"errors"
	"fmt"
	"khel/internal/store"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
)

type CreateGamePayload struct {
	SportType   string    `json:"sport_type" validate:"required,oneof=futsal basketball badminton e-sport cricket tennis"`
	Price       *int      `json:"price,omitempty" validate:"omitempty,min=0"`
	Format      *string   `json:"format,omitempty" validate:"omitempty,max=20"`
	VenueID     int64     `json:"venue_id" validate:"required,min=1"`
	MaxPlayers  int       `json:"max_players" validate:"required,min=1"`
	GameLevel   *string   `json:"game_level,omitempty" validate:"omitempty,oneof=beginner intermediate advanced"`
	StartTime   time.Time `json:"start_time" validate:"required"`
	EndTime     time.Time `json:"end_time" validate:"required,gtfield=StartTime"`
	Visibility  string    `json:"visibility" validate:"required,oneof=public private"`
	Instruction *string   `json:"instruction,omitempty" validate:"omitempty,max=500"`
}

// CreateGame godoc
//
//	@Summary		Create a new game
//	@Description	Create a new game with details such as sport type, venue, start time, and end time.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			payload	body		CreateGamePayload	true	"Game details payload"
//	@Success		201		{object}	store.Game			"Game created successfully"
//	@Failure		400		{object}	error				"Invalid request payload"
//	@Failure		401		{object}	error				"Unauthorized"
//	@Failure		409		{object}	error				"Game overlaps with existing game"
//	@Failure		500		{object}	error				"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/create [post]
func (app *application) createGameHandler(w http.ResponseWriter, r *http.Request) {
	var payload CreateGamePayload

	// 1. Parse and validate the request payload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	//TODO: delete later
	fmt.Printf("validated payload is %v:", payload)

	// 2. Get the authenticated user
	user := getUserFromContext(r)
	//TODO: delete later
	fmt.Printf("the user to create game is %v", user.FirstName)

	// 4. Create the game
	game := &store.Game{
		SportType:   payload.SportType,
		Price:       payload.Price,
		Format:      payload.Format,
		VenueID:     payload.VenueID,
		AdminID:     user.ID, // Set the authenticated user as the game admin
		MaxPlayers:  payload.MaxPlayers,
		GameLevel:   payload.GameLevel,
		StartTime:   payload.StartTime,
		EndTime:     payload.EndTime,
		Visibility:  payload.Visibility,
		Instruction: payload.Instruction,
		Status:      "active", // Default status
		IsBooked:    false,    // Default value
		MatchFull:   false,    // Default value
	}

	// 5. Save the game to the database
	gameID, err := app.store.Games.Create(r.Context(), game)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	//TODO: delete later
	fmt.Printf("Games Created at database..........✅")

	// Put the admin in the game player
	err = app.store.Games.InsertAdminInPlayer(r.Context(), gameID, user.ID)
	if err != nil {
		log.Println("🚨🚨🚨InsertAdminInPlayer failed:", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to add player")
		return
	}
	// 6. Return the created game as the response
	if err := app.jsonResponse(w, http.StatusCreated, game); err != nil {

		app.internalServerError(w, r, err)
		return
	}
}

// CreateJoinRequest godoc
//
//	@Summary		Send a request to join a game
//	@Description	Allows a user to send a request to join a specific game. The game ID is provided in the URL path.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			gameID	path		int					true	"Game ID"
//	@Success		201		{object}	map[string]string	"Join request submitted for approval"
//	@Failure		400		{object}	error				"Invalid game ID"
//	@Failure		404		{object}	error				"Game not found or inactive"
//	@Failure		409		{object}	error				"Join request already sent"
//	@Failure		500		{object}	error				"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID}/request [post]
func (app *application) CreateJoinRequest(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	// Parse gameID from URL
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	// Check if game exists and is active
	_, err = app.store.Games.GetGameByID(r.Context(), gameID)
	if err != nil {
		app.notFoundResponse(w, r, errors.New("game not found or is inactive"))
		return
	}

	// Check if a join request already exists
	exists, err := app.store.Games.CheckRequestExist(r.Context(), gameID, user.ID)
	if err != nil {
		app.logger.Errorf("Error checking join request: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	if exists {
		writeJSONError(w, http.StatusConflict, "Already sent request to this game")
		return // ✅ Fix: Stop execution after sending conflict response
	}

	// Create the join request
	err = app.store.Games.AddToGameRequest(r.Context(), gameID, user.ID)
	if err != nil {
		app.logger.Errorf("Error inserting join request: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create request")
		return
	}

	// Success response
	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Join request submitted for approval",
	})
}

// AcceptJoinRequest godoc
//
//	@Summary		Accept a join request for a game
//	@Description	Accepts a pending join request for a game by updating the request status to accepted and inserting the player into the game. The game ID is provided in the URL path and the user ID is provided in the request body.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			gameID	path		int						true	"Game ID"
//	@Param			payload	body		object{user_id=int}		true	"Payload containing the user ID to accept"
//	@Success		200		{object}	map[string]interface{}	"Message confirming the join request acceptance and player addition"
//	@Failure		400		{object}	error					"Invalid game ID, payload error, or request is not in pending state"
//	@Failure		404		{object}	error					"Join request not found"
//	@Failure		500		{object}	error					"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID}/accept [post]
func (app *application) AcceptJoinRequest(w http.ResponseWriter, r *http.Request) {

	// Parse gameID from URL
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	var payload struct {
		UserID int64 `json:"user_id"`
	}
	if err := readJSON(w, r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "cannot read the payload")
		return
	}

	// Get the join request
	req, err := app.store.Games.GetJoinRequest(r.Context(), gameID, payload.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "Invalid request")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}
	if req.Status != store.GameRequestStatusPending {
		app.badRequestResponse(w, r, errors.New("request is not in pending state"))
		return
	}

	// Update request status
	err = app.store.Games.UpdateRequestStatus(r.Context(), gameID, payload.UserID, store.GameRequestStatusAccepted)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to update request")
		return
	}

	err = app.store.Games.InsertNewPlayer(r.Context(), gameID, payload.UserID)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to add player")
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Successfully added userID: %d to the gameID: %d ✅", req.UserID, req.GameID),
	})
}

// Prepare response
type PlayerResponse struct {
	ID              int64          `json:"id"`
	FirstName       string         `json:"first_name"`
	ProfileImageURL sql.NullString `json:"profile_picture_url" swaggertype:"string"`
	SkillLevel      sql.NullString `json:"skill_level" swaggertype:"string"`
	Phone           string         `json:"phone"`
}

// GetGamePlayersHandler godoc
//
//	@Summary		Retrieve players for a game
//	@Description	Fetches the list of players participating in a specific game. The game ID is provided in the URL path.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			gameID	path		int				true	"Game ID"
//	@Success		200		{array}		PlayerResponse	"List of game players"
//	@Failure		400		{object}	error			"Invalid game ID"
//	@Failure		404		{object}	error			"Game players not found"
//	@Failure		500		{object}	error			"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID}/players [get]
func (app *application) getGamePlayersHandler(w http.ResponseWriter, r *http.Request) {
	// Parse gameID from URL
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid game ID"))
		return
	}

	// Fetch players for the game
	players, err := app.store.Games.GetGamePlayers(r.Context(), gameID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFoundResponse(w, r, store.ErrNotFound)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	response := make([]PlayerResponse, 0, len(players))
	for _, player := range players {
		response = append(response, PlayerResponse{
			ID:              player.ID,
			FirstName:       player.FirstName,
			ProfileImageURL: player.ProfilePictureURL,
			SkillLevel:      player.SkillLevel,
			Phone:           player.Phone,
		})
	}

	// Return JSON response
	if err := app.jsonResponse(w, http.StatusOK, response); err != nil {
		app.internalServerError(w, r, err)
	}
}

// RejectJoinRequest godoc
//
//	@Summary		Reject a join request for a game
//	@Description	Rejects a pending join request for a game. The game ID is specified in the URL path and the user ID is provided in the request body.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			gameID	path		int						true	"Game ID"
//	@Param			payload	body		object{user_id=int}		true	"Payload containing the user ID of the join request to reject"
//	@Success		200		{object}	map[string]interface{}	"Message confirming the join request was rejected"
//	@Failure		400		{object}	error					"Invalid game ID, payload error, or request is not pending"
//	@Failure		404		{object}	error					"Join request not found"
//	@Failure		500		{object}	error					"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID}/reject [post]
func (app *application) RejectJoinRequest(w http.ResponseWriter, r *http.Request) {
	// Parse gameID from URL
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid game ID", http.StatusBadRequest)
		return
	}

	// Parse request body
	var payload struct {
		UserID int64 `json:"user_id"`
	}
	if err := readJSON(w, r, &payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Cannot read the payload")
		return
	}

	// Get the join request
	req, err := app.store.Games.GetJoinRequest(r.Context(), gameID, payload.UserID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "Invalid request")
			return
		}
		writeJSONError(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	// Check if the request is still pending
	if req.Status != store.GameRequestStatusPending {
		app.badRequestResponse(w, r, errors.New("request is not in pending state"))
		return
	}

	// Update request status to rejected
	err = app.store.Games.UpdateRequestStatus(r.Context(), gameID, payload.UserID, store.GameRequestStatusRejected)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "Failed to update request status")
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]interface{}{
		"message": fmt.Sprintf("Successfully rejected userID: %d from gameID: %d ❌", payload.UserID, gameID),
	})
}

// AssignAssistantHandler godoc
//
//	@Summary		Assign an assistant role to a player
//	@Description	Allows a game admin to assign the assistant role to a player for a specified game.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			gameID		path		int					true	"Game ID"
//	@Param			playerID	path		int					true	"Player ID to be assigned as assistant"
//	@Success		200			{object}	map[string]string	"Assistant role assigned successfully"
//	@Failure		400			{object}	error				"Invalid game ID, invalid player ID, or player not found/already an assistant"
//	@Failure		403			{object}	error				"Only game admins can assign assistants"
//	@Failure		500			{object}	error				"Database error"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID}/assign-assistant/{playerID} [post]
func (app *application) AssignAssistantHandler(w http.ResponseWriter, r *http.Request) {
	//      /games/{gameID}/assign-assistant/{playerID}
	// Extract gameID and playerID from URL
	gameIDStr := chi.URLParam(r, "gameID")
	playerIDStr := chi.URLParam(r, "playerID")

	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid game ID")
		return
	}

	playerID, err := strconv.ParseInt(playerIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid player ID")
		return
	}

	// Assign assistant role
	err = app.store.Games.AssignAssistant(r.Context(), gameID, playerID)
	if err != nil {
		if err.Error() == "only game admins can assign assistants" {
			writeJSONError(w, http.StatusForbidden, err.Error())
			return
		}
		if err.Error() == "player not found or already an assistant" {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		app.logger.Errorf("Error assigning assistant: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Database error")
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "Assistant role assigned successfully"})
}

// GetGames godoc
//
//	@Summary		Retrieve a list of games
//	@Description	Returns a list of games based on filters such as sport type, game level, venue, booking status, location, time range, and status.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			sport_type	query		string				false	"Sport type to filter games (e.g., basketball)"
//	@Param			game_level	query		string				false	"Game level (e.g., intermediate)"
//	@Param			venue_id	query		int					false	"Venue ID to filter games"
//	@Param			is_booked	query		boolean				false	"Filter games based on booking status"
//	@Param			status		query		string				false	"Game status: active, cancelled, completed"
//	@Param			lat			query		number				false	"User latitude for location filtering"
//	@Param			lon			query		number				false	"User longitude for location filtering"
//	@Param			radius		query		int					false	"Radius in kilometers for location-based filtering (0 for no filter)"
//	@Param			start_after	query		string				false	"Filter games starting after this time (RFC3339 format)"
//	@Param			end_before	query		string				false	"Filter games ending before this time (RFC3339 format)"
//	@Param			min_price	query		int					false	"Minimum price"
//	@Param			max_price	query		int					false	"Maximum price"
//	@Param			limit		query		int					false	"Maximum number of results to return"
//	@Param			offset		query		int					false	"Pagination offset"
//	@Param			sort		query		string				false	"Sort order, either 'asc' or 'desc'"
//	@Success		200			{object}	[]store.GameSummary	"List of games and GeoJSON features"
//	@Failure		400			{object}	error				"Invalid request parameters"
//	@Failure		500			{object}	error				"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/get-games [get]
func (app *application) getGamesHandler(w http.ResponseWriter, r *http.Request) {
	// Set default filter values.
	fq := store.GameFilterQuery{
		Limit:      10,
		Offset:     0,
		Sort:       "asc",
		Radius:     0, // 0 means no location-based filtering.
		StartAfter: time.Now(),
	}

	// Parse query parameters from the request (overriding defaults if provided).
	fq, err := fq.Parse(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Validate the filter query.
	if err := Validate.Struct(fq); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Get the currently logged in user.
	user := getUserFromContext(r)

	// Query the database for matching games.
	games, err := app.store.Games.GetGames(r.Context(), fq)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Get the shortlisted games for the user.
	shortlistedGames, err := app.store.ShortlistedGames.GetShortlistedGamesByUser(r.Context(), user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Build a set of shortlisted game IDs for fast lookup.
	shortlistedIDs := make(map[int64]struct{})
	for _, sg := range shortlistedGames {
		shortlistedIDs[sg.ID] = struct{}{}
	}

	// Mark games as shortlisted if they appear in the user's shortlist.
	for i, game := range games {
		if _, found := shortlistedIDs[game.GameID]; found {
			games[i].Shortlisted = true
		}
	}

	response := make([]store.GameSummary, len(games))
	copy(response, games)

	if err := app.jsonResponse(w, http.StatusOK, response); err != nil {
		app.internalServerError(w, r, err)
	}
}

// ToggleMatchFull godoc
//
//	@Summary		Toggle match full status
//	@Description	Allows an admin to toggle match full status
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int					true	"Game ID"
//	@Success		200	{object}	map[string]string	"Match full status updated"
//	@Failure		400	{object}	error				"Invalid request payload"
//	@Failure		401	{object}	error				"Unauthorized"
//	@Failure		404	{object}	error				"Game not found"
//	@Failure		500	{object}	error				"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/{id}/toggle-match-full [patch]
func (app *application) toggleMatchFullHandler(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid game ID")
		return
	}
	// Toggle match full status
	err = app.store.Games.ToggleMatchFull(r.Context(), gameID)
	if err == sql.ErrNoRows {
		app.notFoundResponse(w, r, err)
		return
	} else if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "Match full status updated"})
}

// CancelGame godoc
//
//	@Summary		Cancel a game
//	@Description	Allows an admin to cancel a game
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			id	path		int					true	"Game ID"
//	@Success		200	{object}	map[string]string	"Game cancelled successfully"
//	@Failure		400	{object}	error				"Invalid request payload"
//	@Failure		401	{object}	error				"Unauthorized"
//	@Failure		404	{object}	error				"Game not found"
//	@Failure		500	{object}	error				"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID}/cancel-game [patch]
func (app *application) cancelGameHandler(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid game ID")
		return
	}

	// Cancel the game
	err = app.store.Games.CancelGame(r.Context(), gameID)
	if err == sql.ErrNoRows {
		app.notFoundResponse(w, r, err)
		return
	} else if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "Game cancelled successfully"})
}

// GetAllGameJoinRequests godoc
//
//	@Summary		Get all join requests for a game
//	@Description	Retrieve all join requests for a specific game by game ID, including user details.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			gameID	path		int							true	"Game ID"
//	@Success		200		{array}		store.GameRequestWithUser	"List of join requests with user details"
//	@Failure		400		{object}	error						"Invalid game ID"
//	@Failure		500		{object}	error						"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID}/requests [get]
func (app *application) getAllGameJoinRequestsHandler(w http.ResponseWriter, r *http.Request) {
	// Extract game ID from URL params
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid game ID")
		return
	}

	// Fetch join requests from the store
	requests, err := app.store.Games.GetAllJoinRequests(r.Context(), gameID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Respond with the join requests
	if err := app.jsonResponse(w, http.StatusOK, requests); err != nil {
		app.internalServerError(w, r, err)
		return
	}
}

// GetGameDetails godoc
//
//	@Summary		Get detailed game information
//	@Description	Returns detailed information for a specific game including venue details and player images
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			gameID	path		int	true	"Game ID"
//	@Success		200		{object}	store.GameDetails
//	@Failure		400		{object}	error	"Invalid game ID"
//	@Failure		404		{object}	error	"Game not found"
//	@Failure		500		{object}	error	"Internal server error"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID} [get]
func (app *application) getGameDetailsHandler(w http.ResponseWriter, r *http.Request) {
	// Parse gameID from URL
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid game ID"))
		return
	}

	// Fetch game details
	game, err := app.store.Games.GetGameDetailsWithID(r.Context(), gameID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			app.notFoundResponse(w, r, errors.New("game not found"))
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	// Return JSON response
	if err := app.jsonResponse(w, http.StatusOK, game); err != nil {
		app.internalServerError(w, r, err)
	}
}

// GetUpcomingGamesByVenue godoc
//
//	@Summary		Retrieve upcoming active games at a venue
//	@Description	Returns a list of upcoming active games for the specified venue.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Success		200		{array}		store.GameSummary	"List of upcoming active games"
//	@Failure		400		{object}	error				"Bad Request: Missing or invalid venueID"
//	@Failure		500		{object}	error				"Internal Server Error: Could not retrieve upcoming games"
//	@Security		ApiKeyAuth
//	@Router			/games/{venueID}/upcoming [get]
func (app *application) getUpcomingGamesByVenueHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve the venueID from the URL path parameter using Chi.
	venueIDStr := chi.URLParam(r, "venueID")
	if venueIDStr == "" {
		app.badRequestResponse(w, r, fmt.Errorf("missing venueID parameter"))
		return
	}

	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID parameter: %w", err))
		return
	}

	games, err := app.store.Games.GetUpcomingGamesByVenue(r.Context(), venueID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Return the games as a JSON response.
	if err := app.jsonResponse(w, http.StatusOK, games); err != nil {
		app.internalServerError(w, r, err)
	}
}

// GetUpcomingGamesForUser godoc
//
//	@Summary		List upcoming active games for the current user
//	@Description	Returns a list of upcoming active games that the authenticated user has joined.
//	@Tags			Games
//	@Accept			json
//	@Produce		json
//	@Success		200	{array}		store.GameSummary	"List of upcoming active games for user"
//	@Failure		401	{object}	error				"Unauthorized: missing or invalid API key"
//	@Failure		500	{object}	error				"Internal Server Error: could not retrieve upcoming games"
//	@Security		ApiKeyAuth
//	@Router			/games/get-upcoming [get]
func (app *application) getUpcomingGamesForUser(w http.ResponseWriter, r *http.Request) {

	user := getUserFromContext(r)
	if user == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("Unauthorized"))
		return
	}

	games, err := app.store.Games.GetUpcomingGamesByUser(r.Context(), user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Return the games as a JSON response.
	if err := app.jsonResponse(w, http.StatusOK, games); err != nil {
		app.internalServerError(w, r, err)
	}
}

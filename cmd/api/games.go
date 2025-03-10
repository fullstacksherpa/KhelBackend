package main

import (
	"database/sql"
	"errors"
	"fmt"
	"khel/internal/store"
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
//	@Router			/games [post]
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

	// 2. Get the authenticated user
	user := getUserFromContext(r)

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
	if err := app.store.Games.Create(r.Context(), game); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 6. Return the created game as the response
	if err := app.jsonResponse(w, http.StatusCreated, game); err != nil {
		app.internalServerError(w, r, err)
		return
	}
}

// POST /games/{gameID}/request
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

// POST /games/{gameID}/accept
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

	// Prepare response
	type PlayerResponse struct {
		ID              int64          `json:"id"`
		FirstName       string         `json:"first_name"`
		ProfileImageURL sql.NullString `json:"profile_picture_url"`
		SkillLevel      sql.NullString `json:"skill_level"`
		Phone           string         `json:"phone"`
	}

	response := make([]PlayerResponse, 0, len(players))
	for _, player := range players {
		response = append(response, PlayerResponse{
			ID:              player.ID,
			FirstName:       player.FirstName,
			ProfileImageURL: sql.NullString{String: player.ProfilePictureURL.String, Valid: player.ProfilePictureURL.Valid},
			SkillLevel:      sql.NullString{String: player.SkillLevel.String, Valid: player.SkillLevel.Valid},
			Phone:           player.Phone,
		})
	}

	// Return JSON response
	if err := app.jsonResponse(w, http.StatusOK, response); err != nil {
		app.internalServerError(w, r, err)
	}
}

// POST /games/{gameID}/reject
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

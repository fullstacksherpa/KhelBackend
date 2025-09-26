package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// AddShortlistedGame godoc
//
//	@Summary		Add a game to shortlist
//	@Description	Allows authenticated users to add a game to their shortlist.
//	@Tags			Shortlist_Games
//	@Accept			json
//	@Produce		json
//	@Param			gameID	path		int					true	"Game ID"
//	@Success		201		{object}	map[string]string	"Game added to shortlist"
//	@Failure		400		{object}	error				"Bad Request: Invalid game ID or unauthenticated request"
//	@Failure		500		{object}	error				"Internal Server Error: Could not add shortlist"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID}/shortlist [post]
func (app *application) addShortlistedGameHandler(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil || gameID == 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid gameID"))
		return
	}

	user := getUserFromContext(r)
	if user == nil {
		app.badRequestResponse(w, r, fmt.Errorf("unauthenticated request"))
		return
	}

	if err := app.store.Games.AddShortlist(r.Context(), user.ID, gameID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusCreated, map[string]string{"message": "game added to shortlist"})
}

// RemoveShortlistedGame godoc
//
//	@Summary		Remove a game from shortlist
//	@Description	Allows authenticated users to remove a game from their shortlist.
//	@Tags			Shortlist_Games
//	@Accept			json
//	@Produce		json
//	@Param			gameID	path		int					true	"Game ID"
//	@Success		200		{object}	map[string]string	"Game removed from shortlist"
//	@Failure		400		{object}	error				"Bad Request: Invalid game ID or unauthenticated request"
//	@Failure		500		{object}	error				"Internal Server Error: Could not remove shortlist"
//	@Security		ApiKeyAuth
//	@Router			/games/{gameID}/shortlist [delete]
func (app *application) removeShortlistedGameHandler(w http.ResponseWriter, r *http.Request) {
	gameIDStr := chi.URLParam(r, "gameID")
	gameID, err := strconv.ParseInt(gameIDStr, 10, 64)
	if err != nil || gameID == 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid gameID"))
		return
	}

	user := getUserFromContext(r)
	if user == nil {
		app.badRequestResponse(w, r, fmt.Errorf("unauthenticated request"))
		return
	}

	if err := app.store.Games.RemoveShortlist(r.Context(), user.ID, gameID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "game removed from shortlist"})
}

// ListShortlistedGames godoc
//
//	@Summary		Retrieve shortlisted games for the authenticated user
//	@Description	Returns a list of games that the authenticated user has shortlisted.
//	@Tags			Shortlist_Games
//	@Accept			json
//	@Produce		json
//	@Success		200	{array}		[]store.ShortlistedGameDetail	"List of shortlisted games"
//	@Failure		400	{object}	error							"Bad Request: Unauthenticated request"
//	@Failure		500	{object}	error							"Internal Server Error: Could not retrieve shortlist"
//	@Security		ApiKeyAuth
//	@Router			/games/shortlist [get]
func (app *application) listShortlistedGamesHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	if user == nil {
		app.badRequestResponse(w, r, fmt.Errorf("unauthenticated request"))
		return
	}

	games, err := app.store.Games.GetShortlistedGamesByUser(r.Context(), user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, games)
}

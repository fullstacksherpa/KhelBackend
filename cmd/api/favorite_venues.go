package main

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// AddFavoriteVenue godoc
//
//	@Summary		Add a venue to favorites
//	@Description	Allows authenticated users to add a venue to their favorites list.
//	@Tags			Favorite_Venues
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Success		201		{object}	map[string]string	"Venue added to favorites"
//	@Failure		400		{object}	error				"Bad Request: Invalid venue ID or unauthenticated request"
//	@Failure		500		{object}	error				"Internal Server Error: Could not add favorite"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/favorite [post]
func (app *application) addFavoriteHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the venue ID from URL parameters.
	venueIDStr := chi.URLParam(r, "venueID")
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil || venueID == 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID"))
		return
	}

	// Get the current user from the context (assumed to be added by your middleware).
	user := getUserFromContext(r)
	if user == nil {
		app.badRequestResponse(w, r, fmt.Errorf("unauthenticated request"))
		return
	}

	// Insert the favorite.
	if err := app.store.Venues.AddFavorite(r.Context(), user.ID, venueID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Return a success response.
	app.jsonResponse(w, http.StatusCreated, map[string]string{"message": "venue added to favorites"})
}

// RemoveFavoriteVenue godoc
//
//	@Summary		Remove a venue from favorites
//	@Description	Allows authenticated users to remove a venue from their favorites list.
//	@Tags			Favorite_Venues
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Success		200		{object}	map[string]string	"Venue removed from favorites"
//	@Failure		400		{object}	error				"Bad Request: Invalid venue ID or unauthenticated request"
//	@Failure		500		{object}	error				"Internal Server Error: Could not remove favorite"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/favorite [delete]
func (app *application) removeFavoriteHandler(w http.ResponseWriter, r *http.Request) {
	// Extract the venue ID from URL parameters.
	venueIDStr := chi.URLParam(r, "venueID")
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil || venueID == 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID"))
		return
	}

	// Get the current user from the context.
	user := getUserFromContext(r)
	if user == nil {
		app.badRequestResponse(w, r, fmt.Errorf("unauthenticated request"))
		return
	}

	// Delete the favorite record.
	if err := app.store.Venues.RemoveFavorite(r.Context(), user.ID, venueID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Return a success response.
	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "venue removed from favorites"})
}

// ListFavoriteVenues godoc
//
//	@Summary		Retrieve user's favorite venues
//	@Description	Returns a list of venues that the authenticated user has marked as favorites.
//	@Tags			Favorite_Venues
//	@Accept			json
//	@Produce		json
//	@Success		200	{array}		store.Venue	"List of favorite venues"
//	@Failure		400	{object}	error		"Bad Request: Unauthenticated request"
//	@Failure		500	{object}	error		"Internal Server Error: Could not retrieve favorites"
//	@Security		ApiKeyAuth
//	@Router			/venues/favorites [get]
func (app *application) listFavoritesHandler(w http.ResponseWriter, r *http.Request) {
	// Get the current user.
	user := getUserFromContext(r)
	if user == nil {
		app.badRequestResponse(w, r, fmt.Errorf("unauthenticated request"))
		return
	}

	// Get the list of favorite venues.
	favorites, err := app.store.Venues.GetFavoritesByUser(r.Context(), user.ID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Return the list as JSON.
	app.jsonResponse(w, http.StatusOK, favorites)
}

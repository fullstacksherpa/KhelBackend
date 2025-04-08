package main

import (
	"database/sql"
	"errors"
	"fmt"
	"khel/internal/store"
	"math"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

// Create Review Handler
type createReviewPayload struct {
	Rating  int    `json:"rating" validate:"required,min=1,max=5"`
	Comment string `json:"comment" validate:"required,max=500"`
}

// CreateVenueReview godoc
//
//	@Summary		Create a review for a venue
//	@Description	Creates a new review for a specific venue. The review includes a rating and comment.
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Param			payload	body		createReviewPayload	true	"Review payload"
//	@Success		201		{object}	store.Review		"Review created successfully"
//	@Failure		400		{object}	error				"Bad Request: Invalid input"
//	@Failure		500		{object}	error				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/reviews [post]
func (app *application) createVenueReviewHandler(w http.ResponseWriter, r *http.Request) {
	venueID := chi.URLParam(r, "venueID")
	vID, err := strconv.ParseInt(venueID, 10, 64)
	fmt.Printf("I got this venue id on review %v", vID)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid venue ID"))
		return
	}

	var payload createReviewPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := getUserFromContext(r)
	userID := user.ID

	review := &store.Review{
		VenueID: vID,
		UserID:  userID,
		Rating:  payload.Rating,
		Comment: payload.Comment,
	}

	if err := app.store.Reviews.CreateReview(r.Context(), review); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusCreated, review)
}

// GetVenueReviews godoc
//
//	@Summary		Retrieve reviews for a venue
//	@Description	Retrieves all reviews for a specific venue along with the total count and average rating.
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int						true	"Venue ID"
//	@Success		200		{object}	map[string]interface{}	"Reviews, total review count, and average rating"
//	@Failure		400		{object}	error					"Bad Request: Invalid venue ID"
//	@Failure		500		{object}	error					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/reviews [get]
func (app *application) getVenueReviewsHandler(w http.ResponseWriter, r *http.Request) {
	venueID := chi.URLParam(r, "venueID")
	vID, err := strconv.ParseInt(venueID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid venue ID"))
		return
	}

	reviews, err := app.store.Reviews.GetReviews(r.Context(), vID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Get review stats
	total, average, err := app.store.Reviews.GetReviewStats(r.Context(), vID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	response := map[string]interface{}{
		"reviews":       reviews,
		"total_reviews": total,
		"average":       math.Round(average*10) / 10,
	}

	app.jsonResponse(w, http.StatusOK, response)
}

// DeleteVenueReview godoc
//
//	@Summary		Delete a venue review
//	@Description	Deletes a review for a venue. This operation is allowed only if the requester is the review owner.
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int					true	"Venue ID"
//	@Param			reviewID	path		int					true	"Review ID"
//	@Success		200			{object}	map[string]string	"Review deleted successfully"
//	@Failure		400			{object}	error				"Bad Request: Invalid review ID"
//	@Failure		404			{object}	error				"Not Found: Review not found"
//	@Failure		500			{object}	error				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/reviews/{reviewID} [delete]
func (app *application) deleteVenueReviewHandler(w http.ResponseWriter, r *http.Request) {
	reviewID := chi.URLParam(r, "reviewID")
	rID, err := strconv.ParseInt(reviewID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid review ID"))
		return
	}

	user := getUserFromContext(r)
	userID := user.ID

	if err := app.store.Reviews.DeleteReview(r.Context(), rID, userID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "review deleted"})
}

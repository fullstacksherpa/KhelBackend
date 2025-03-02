package main

import (
	"database/sql"
	"errors"
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

func (app *application) createVenueReviewHandler(w http.ResponseWriter, r *http.Request) {
	venueID := chi.URLParam(r, "venueID")
	vID, err := strconv.ParseInt(venueID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid venue ID"))
		return
	}

	var payload createReviewPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	//TODO: update later
	var userID = int64(1)

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

// Get Reviews Handler
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

// Delete Review Handler
func (app *application) deleteVenueReviewHandler(w http.ResponseWriter, r *http.Request) {
	reviewID := chi.URLParam(r, "reviewID")
	rID, err := strconv.ParseInt(reviewID, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, errors.New("invalid review ID"))
		return
	}

	//TODO: update later
	var userID = int64(1)

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

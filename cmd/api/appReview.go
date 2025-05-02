package main

import (
	"errors"
	"fmt"
	"net/http"
)

// for swagger only
type AppReviewPayload struct {
	Rating   int    `json:"rating"`
	Feedback string `json:"feedback"`
}

// submitReviewHandler godoc
//
//	@Summary		Submit App Review
//	@Description	Allows authenticated users to submit a rating and feedback for the app.
//	@Tags			App_Reviews
//	@Accept			json
//	@Produce		json
//	@Param			review	body		AppReviewPayload	true	"Review payload"
//	@Success		201		{object}	string				"Review submitted"
//	@Failure		400		{object}	error				"Bad Request"
//	@Failure		500		{object}	error				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/app-reviews [post]
func (app *application) submitReviewHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Rating   int    `json:"rating"`
		Feedback string `json:"feedback"`
	}
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if payload.Rating < 1 || payload.Rating > 5 {
		app.badRequestResponse(w, r, fmt.Errorf("rating must be between 1 and 5"))
		return
	}

	user := getUserFromContext(r)
	if user == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("please logout and login again"))
		return
	}

	if err := app.store.AppReviews.AddReview(r.Context(), user.ID, payload.Rating, payload.Feedback); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusCreated, map[string]string{"message": "Review submitted"})
}

// getAllAppReviewsHandler godoc
//
//	@Summary		List All App Reviews
//	@Description	Returns all app reviews from all users.
//	@Tags			App_Reviews
//	@Accept			json
//	@Produce		json
//	@Success		200	{array}		store.AppReview	"List of all reviews"
//	@Failure		401	{object}	error			"unauthorized route"
//	@Failure		500	{object}	error			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/app-reviews [get]
func (app *application) getAllAppReviewsHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)

	if user == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("please logout and login again"))
		return
	}

	reviews, err := app.store.AppReviews.GetAllReviews(r.Context())
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, reviews)
}

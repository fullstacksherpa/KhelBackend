package main

import (
	"context"
	"errors"
	"fmt"
	"khel/internal/domain/facilities"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

func parseInt64PathParam(r *http.Request, name string) (int64, error) {
	value := chi.URLParam(r, name)
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("invalid %s", name)
	}
	return id, nil
}

func (app *application) parseVenueAndFacilityID(r *http.Request) (int64, int64, error) {
	venueID, err := parseInt64PathParam(r, "venueID")
	if err != nil {
		return 0, 0, err
	}

	facilityID, err := parseInt64PathParam(r, "facilityID")
	if err != nil {
		return 0, 0, err
	}

	return venueID, facilityID, nil
}

// Ensures users cannot access another venue's facility by guessing facilityID.
func (app *application) requireFacilityBelongsToVenue(ctx context.Context, venueID, facilityID int64) error {
	ok, err := app.store.Facilities.BelongsToVenue(ctx, venueID, facilityID)
	if err != nil {
		return err
	}

	if !ok {
		return facilities.ErrFacilityNotFound
	}

	return nil
}

type CreateFacilityPayload struct {
	Name        string   `json:"name" validate:"required,max=120"`
	Description *string  `json:"description,omitempty" validate:"omitempty,max=500"`
	Sport       *string  `json:"sport,omitempty" validate:"omitempty,max=50"`
	SurfaceType *string  `json:"surface_type,omitempty" validate:"omitempty,max=80"`
	Capacity    *int     `json:"capacity,omitempty" validate:"omitempty,gt=0"`
	ImageURLs   []string `json:"image_urls,omitempty"`
	IsDefault   bool     `json:"is_default"`
}

type UpdateFacilityPayload struct {
	Name        *string  `json:"name,omitempty" validate:"omitempty,max=120"`
	Description *string  `json:"description,omitempty" validate:"omitempty,max=500"`
	Sport       *string  `json:"sport,omitempty" validate:"omitempty,max=50"`
	SurfaceType *string  `json:"surface_type,omitempty" validate:"omitempty,max=80"`
	Capacity    *int     `json:"capacity,omitempty" validate:"omitempty,gt=0"`
	ImageURLs   []string `json:"image_urls,omitempty"`
	IsActive    *bool    `json:"is_active,omitempty"`
	IsDefault   *bool    `json:"is_default,omitempty"`
}

// createFacilityHandler godoc
//
//	@Summary		Create facility under venue
//	@Description	Creates a bookable facility/ground/court under a venue. Facilities are the new booking and pricing scope.
//	@Tags			Venue Facilities
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int						true	"Venue ID"
//	@Param			payload	body		CreateFacilityPayload	true	"Facility payload"
//	@Success		201		{object}	facilities.Facility		"Facility created"
//	@Failure		400		{object}	ErrorResponse			"Bad Request"
//	@Failure		401		{object}	ErrorResponse			"Unauthorized"
//	@Failure		403		{object}	ErrorResponse			"Forbidden"
//	@Failure		500		{object}	ErrorResponse			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities [post]
func (app *application) createFacilityHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := parseInt64PathParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var payload CreateFacilityPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	payload.Name = strings.TrimSpace(payload.Name)

	facility, err := app.store.Facilities.Create(r.Context(), facilities.CreateFacilityInput{
		VenueID:     venueID,
		Name:        payload.Name,
		Description: payload.Description,
		Sport:       payload.Sport,
		SurfaceType: payload.SurfaceType,
		Capacity:    payload.Capacity,
		ImageURLs:   payload.ImageURLs,
		IsDefault:   payload.IsDefault,
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusCreated, facility)
}

// listFacilitiesHandler godoc
//
//	@Summary		List facilities under venue
//	@Description	Returns all facilities/grounds/courts under a venue.
//	@Tags			Venue Facilities
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Success		200		{array}		facilities.Facility	"Facilities"
//	@Failure		400		{object}	ErrorResponse		"Bad Request"
//	@Failure		401		{object}	ErrorResponse		"Unauthorized"
//	@Failure		500		{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities [get]
func (app *application) listFacilitiesHandler(w http.ResponseWriter, r *http.Request) {
	venueID, err := parseInt64PathParam(r, "venueID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	items, err := app.store.Facilities.ListByVenueID(r.Context(), venueID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, items)
}

// getFacilityHandler godoc
//
//	@Summary		Get facility detail
//	@Description	Returns one facility by venue ID and facility ID.
//	@Tags			Venue Facilities
//	@Produce		json
//	@Param			venueID		path		int					true	"Venue ID"
//	@Param			facilityID	path		int					true	"Facility ID"
//	@Success		200			{object}	facilities.Facility	"Facility"
//	@Failure		400			{object}	ErrorResponse		"Bad Request"
//	@Failure		404			{object}	ErrorResponse		"Facility not found"
//	@Failure		500			{object}	ErrorResponse		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID} [get]
func (app *application) getFacilityHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	facility, err := app.store.Facilities.GetByID(r.Context(), venueID, facilityID)
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, facility)
}

// updateFacilityHandler godoc
//
//	@Summary		Update facility
//	@Description	Updates a facility under a venue.
//	@Tags			Venue Facilities
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int						true	"Venue ID"
//	@Param			facilityID	path		int						true	"Facility ID"
//	@Param			payload		body		UpdateFacilityPayload	true	"Facility update payload"
//	@Success		200			{object}	facilities.Facility		"Facility updated"
//	@Failure		400			{object}	ErrorResponse			"Bad Request"
//	@Failure		404			{object}	ErrorResponse			"Facility not found"
//	@Failure		500			{object}	ErrorResponse			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID} [patch]
func (app *application) updateFacilityHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var payload UpdateFacilityPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if payload.Name != nil {
		trimmed := strings.TrimSpace(*payload.Name)
		if trimmed == "" {
			app.badRequestResponse(w, r, fmt.Errorf("name cannot be empty"))
			return
		}
		payload.Name = &trimmed
	}

	facility, err := app.store.Facilities.Update(r.Context(), venueID, facilityID, facilities.UpdateFacilityInput{
		Name:        payload.Name,
		Description: payload.Description,
		Sport:       payload.Sport,
		SurfaceType: payload.SurfaceType,
		Capacity:    payload.Capacity,
		ImageURLs:   payload.ImageURLs,
		IsActive:    payload.IsActive,
		IsDefault:   payload.IsDefault,
	})
	if err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, facility)
}

// deleteFacilityHandler godoc
//
//	@Summary		Delete facility
//	@Description	Deletes a non-default facility. Default facility cannot be deleted because old venue-level APIs depend on it.
//	@Tags			Venue Facilities
//	@Produce		json
//	@Param			venueID		path	int	true	"Venue ID"
//	@Param			facilityID	path	int	true	"Facility ID"
//	@Success		204			"No Content"
//	@Failure		400			{object}	ErrorResponse	"Bad Request"
//	@Failure		404			{object}	ErrorResponse	"Facility not found or default facility cannot be deleted"
//	@Failure		500			{object}	ErrorResponse	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID} [delete]
func (app *application) deleteFacilityHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.store.Facilities.Delete(r.Context(), venueID, facilityID); err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

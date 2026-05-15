package main

import (
	"fmt"
	"khel/internal/domain/bookings"
	"net/http"
	"strings"
	"time"
)

// getFacilityPricingHandler godoc
//
//	@Summary		Retrieve pricing slots for a facility
//	@Description	Returns pricing slots for a facility under a venue. Optional `day` query filters by day of week.
//	@Tags			Facility Pricing
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int						true	"Venue ID"
//	@Param			facilityID	path		int						true	"Facility ID"
//	@Param			day			query		string					false	"Day of week"
//	@Success		200			{array}		bookings.PricingSlot	"Pricing slots"
//	@Failure		400			{object}	ErrorResponse			"Bad Request"
//	@Failure		404			{object}	ErrorResponse			"Facility not found"
//	@Failure		500			{object}	ErrorResponse			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/pricing [get]
func (app *application) getFacilityPricingHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.requireFacilityBelongsToVenue(r.Context(), venueID, facilityID); err != nil {
		app.notFoundResponse(w, r, err)
		return
	}

	dayOfWeek := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("day")))

	pricingSlots, err := app.store.Bookings.GetPricingSlots(r.Context(), venueID, facilityID, dayOfWeek)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, pricingSlots)
}

// createFacilityPricingHandler godoc
//
//	@Summary		Create pricing slots for a facility
//	@Description	Adds new pricing slots to a facility under a venue.
//	@Description	start_time and end_time must use 24-hour HH:mm:ss format. Example: 09:00:00, 18:30:00.
//	@Description	start_time must be before end_time.
//	@Tags			Facility Pricing
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int							true	"Venue ID"
//	@Param			facilityID	path		int							true	"Facility ID"
//	@Param			payload		body		BulkCreatePricingPayload	true	"Pricing slots"
//	@Success		201			{array}		bookings.PricingSlot		"Pricing slots created"
//	@Failure		400			{object}	ErrorResponse				"Bad Request"
//	@Failure		404			{object}	ErrorResponse				"Facility not found"
//	@Failure		500			{object}	ErrorResponse				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/pricing [post]
func (app *application) createFacilityPricingHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.requireFacilityBelongsToVenue(r.Context(), venueID, facilityID); err != nil {
		app.notFoundResponse(w, r, err)
		return
	}

	var payload BulkCreatePricingPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	slots := make([]*bookings.PricingSlot, 0, len(payload.Slots))

	for i, in := range payload.Slots {
		st, err := time.Parse("15:04:05", in.StartTime)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("slot %d: invalid start_time", i))
			return
		}

		et, err := time.Parse("15:04:05", in.EndTime)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("slot %d: invalid end_time", i))
			return
		}

		if !st.Before(et) {
			app.badRequestResponse(w, r, fmt.Errorf("slot %d: start_time must be before end_time", i))
			return
		}

		slots = append(slots, &bookings.PricingSlot{
			VenueID:    venueID,
			FacilityID: facilityID,
			DayOfWeek:  strings.ToLower(strings.TrimSpace(in.DayOfWeek)),
			StartTime:  st,
			EndTime:    et,
			Price:      in.Price,
		})
	}

	if err := app.store.Bookings.CreatePricingSlotsBatch(r.Context(), slots); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusCreated, slots)
}

// updateFacilityPricingHandler godoc
//
//	@Summary		Update facility pricing slot
//	@Description	Updates a pricing slot for a specific facility under a venue.
//	@Tags			Facility Pricing
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int						true	"Venue ID"
//	@Param			facilityID	path		int						true	"Facility ID"
//	@Param			pricingID	path		int						true	"Pricing Slot ID"
//	@Param			payload		body		UpdatePricingPayload	true	"Pricing update payload"
//	@Success		200			{object}	bookings.PricingSlot	"Pricing updated"
//	@Failure		400			{object}	ErrorResponse			"Bad Request"
//	@Failure		404			{object}	ErrorResponse			"Facility or pricing not found"
//	@Failure		500			{object}	ErrorResponse			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/pricing/{pricingID} [put]
func (app *application) updateFacilityPricingHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	pricingID, err := parseInt64PathParam(r, "pricingID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.requireFacilityBelongsToVenue(r.Context(), venueID, facilityID); err != nil {
		app.notFoundResponse(w, r, err)
		return
	}

	var payload UpdatePricingPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	startTime, err := time.Parse("15:04:05", payload.StartTime)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	endTime, err := time.Parse("15:04:05", payload.EndTime)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if !startTime.Before(endTime) {
		app.badRequestResponse(w, r, fmt.Errorf("start_time must be before end_time"))
		return
	}

	pricing := &bookings.PricingSlot{
		ID:         pricingID,
		VenueID:    venueID,
		FacilityID: facilityID,
		DayOfWeek:  strings.ToLower(strings.TrimSpace(payload.DayOfWeek)),
		StartTime:  startTime,
		EndTime:    endTime,
		Price:      payload.Price,
	}

	if err := app.store.Bookings.UpdatePricing(r.Context(), pricing); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, pricing)
}

// deleteFacilityPricingHandler godoc
//
//	@Summary		Delete facility pricing slot
//	@Description	Deletes a pricing slot for a specific facility under a venue.
//	@Tags			Facility Pricing
//	@Produce		json
//	@Param			venueID		path	int	true	"Venue ID"
//	@Param			facilityID	path	int	true	"Facility ID"
//	@Param			pricingID	path	int	true	"Pricing Slot ID"
//	@Success		204			"No Content"
//	@Failure		400			{object}	ErrorResponse	"Bad Request"
//	@Failure		404			{object}	ErrorResponse	"Facility or pricing not found"
//	@Failure		500			{object}	ErrorResponse	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/pricing/{pricingID} [delete]
func (app *application) deleteFacilityPricingHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	pricingID, err := parseInt64PathParam(r, "pricingID")
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.store.Bookings.DeletePricingSlot(r.Context(), venueID, facilityID, pricingID); err != nil {
		if strings.Contains(err.Error(), "no pricing slot found") {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	writeJSON(w, http.StatusNoContent, nil)
}

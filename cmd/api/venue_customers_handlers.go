package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"khel/internal/domain/venuecustomers"
	"khel/internal/params"

	"github.com/go-chi/chi/v5"
)

type venueCustomerListResponse struct {
	Customers  []venuecustomers.VenueCustomer `json:"customers"`
	Pagination params.Pagination              `json:"pagination"`
	Segment    string                         `json:"segment"`
}

// listVenueCustomersHandler godoc
//
//	@Summary		List venue customers
//	@Description	Lists users who booked this specific venue only. Supports customer segments such as regular, high_value, risky, cancel_often, and spend_more.
//	@Tags			Venue-Owner-Customers
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int		true	"Venue ID"
//	@Param			segment	query		string	false	"Customer segment"	Enums(all,regular,high_value,risky,cancel_often,spend_more)
//	@Param			page	query		int		false	"Page number (default: 1)"
//	@Param			limit	query		int		false	"Items per page (default: 15, max: 30)"
//	@Success		200		{object}	envelope{data=venueCustomerListResponse}
//	@Failure		400		{object}	error	"Bad Request: invalid venue ID or segment"
//	@Failure		401		{object}	error	"Unauthorized"
//	@Failure		403		{object}	error	"Forbidden: venue does not belong to owner"
//	@Failure		500		{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/customers [get]
func (app *application) listVenueCustomersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	venueID, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil || venueID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID"))
		return
	}

	segment := strings.TrimSpace(r.URL.Query().Get("segment"))
	if segment == "" {
		segment = string(venuecustomers.SegmentAll)
	}

	if !venuecustomers.IsValidSegment(segment) {
		app.badRequestResponse(w, r, fmt.Errorf("invalid segment %q", segment))
		return
	}

	p := params.ParsePagination(r.URL.Query())

	customers, total, err := app.store.VenueCustomers.ListVenueCustomers(ctx, venueID, venuecustomers.ListCustomersFilter{
		Segment: venuecustomers.Segment(segment),
		Limit:   p.Limit,
		Offset:  p.Offset,
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	p.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, venueCustomerListResponse{
		Customers:  customers,
		Pagination: p,
		Segment:    segment,
	})
}

// getVenueCustomerDetailHandler godoc
//
//	@Summary		Get venue customer detail
//	@Description	Retrieves customer detail for a specific venue only, including reliability score, spend summary, last booking, consumed inventory items, and recent bookings.
//	@Tags			Venue-Owner-Customers
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int	true	"Venue ID"
//	@Param			userID	path		int	true	"User ID"
//	@Success		200		{object}	envelope{data=venuecustomers.VenueCustomerDetail}
//	@Failure		400		{object}	error	"Bad Request: invalid venue ID or user ID"
//	@Failure		401		{object}	error	"Unauthorized"
//	@Failure		403		{object}	error	"Forbidden: venue does not belong to owner"
//	@Failure		404		{object}	error	"Not Found: customer has not booked this venue"
//	@Failure		500		{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/customers/{userID} [get]
func (app *application) getVenueCustomerDetailHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	venueID, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil || venueID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID"))
		return
	}

	userID, err := strconv.ParseInt(chi.URLParam(r, "userID"), 10, 64)
	if err != nil || userID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid userID"))
		return
	}

	detail, err := app.store.VenueCustomers.GetVenueCustomerDetail(ctx, venueID, userID)
	if err != nil {
		if errors.Is(err, venuecustomers.ErrCustomerNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, detail)
}

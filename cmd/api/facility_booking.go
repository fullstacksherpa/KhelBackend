package main

import (
	"errors"
	"fmt"
	"khel/internal/domain/bookings"
	"khel/internal/domain/facilities"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// FacilityBookingDateQuery is used only for Swagger documentation.
//
// The handlers below read the date from query string:
//
//	?date=2026-05-12
//
// If date is not provided, we default to today's date in Nepal timezone.
// This is useful for owner dashboard screens where owner usually wants
// today's pending/scheduled/canceled bookings.
type FacilityBookingDateQuery struct {
	Date string `json:"date" example:"2026-05-12"`
}

// FacilityPendingBookingsResponse is the response wrapper for pending bookings.
type FacilityPendingBookingsResponse struct {
	Bookings []bookings.PendingBooking `json:"bookings"`
}

// FacilityScheduledBookingsResponse is the response wrapper for scheduled bookings.
type FacilityScheduledBookingsResponse struct {
	Bookings []bookings.ScheduledBooking `json:"bookings"`
}

// FacilityCanceledBookingsResponse is the response wrapper for canceled bookings.
type FacilityCanceledBookingsResponse struct {
	Bookings []bookings.CanceledBooking `json:"bookings"`
}

// parseBookingDateFromQuery parses ?date=YYYY-MM-DD from request.
//
// Important:
// We use Asia/Kathmandu because our product is Nepal-based.
// This prevents bugs where the server timezone or UTC shifts the booking date.
//
// Example:
//
//	GET /venues/5/facilities/2/pending-bookings?date=2026-05-12
//
// If date is missing, this returns today's Nepal date.
func parseBookingDateFromQuery(r *http.Request) (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load Nepal timezone: %w", err)
	}

	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		now := time.Now().In(loc)

		// Normalize to start of local day.
		// Repository method will usually build full-day boundaries again,
		// but passing a normalized date keeps handler behavior predictable.
		return time.Date(
			now.Year(),
			now.Month(),
			now.Day(),
			0, 0, 0, 0,
			loc,
		), nil
	}

	parsedDate, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format, use YYYY-MM-DD")
	}

	return parsedDate, nil
}

// getPendingFacilityBookingsHandler godoc
//
//	@Summary		Get pending bookings for a facility
//	@Description	Returns pending booking requests for one facility under a venue. This is the facility-level replacement for venue-level pending bookings.
//	@Tags			Facility Bookings
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int								true	"Venue ID"
//	@Param			facilityID	path		int								true	"Facility ID"
//	@Param			date		query		string							false	"Booking date in YYYY-MM-DD format. Defaults to today's date in Nepal timezone."
//	@Success		200			{object}	FacilityPendingBookingsResponse	"Pending bookings"
//	@Failure		400			{object}	ErrorResponse					"Bad request"
//	@Failure		401			{object}	ErrorResponse					"Unauthorized"
//	@Failure		403			{object}	ErrorResponse					"Forbidden"
//	@Failure		404			{object}	ErrorResponse					"Facility not found"
//	@Failure		500			{object}	ErrorResponse					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/pending-bookings [get]
func (app *application) getPendingFacilityBookingsHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Security/safety check:
	// Never trust facilityID alone from the URL.
	// A user may guess another facility ID, so verify it belongs to this venue.
	if err := app.requireFacilityBelongsToVenue(r.Context(), venueID, facilityID); err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	date, err := parseBookingDateFromQuery(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	pendingBookings, err := app.store.Bookings.GetPendingBookingsForVenueDate(
		r.Context(),
		venueID,
		facilityID,
		date,
	)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, PendingBookingsResponse{
		Bookings: app.pendingBookingsToResponse(pendingBookings),
	})
}

// getScheduledFacilityBookingsHandler godoc
//
//	@Summary		Get scheduled bookings for a facility
//	@Description	Returns confirmed/scheduled bookings for one facility under a venue. This lets owners manage each ground/court separately.
//	@Tags			Facility Bookings
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int									true	"Venue ID"
//	@Param			facilityID	path		int									true	"Facility ID"
//	@Param			date		query		string								false	"Booking date in YYYY-MM-DD format. Defaults to today's date in Nepal timezone."
//	@Success		200			{object}	FacilityScheduledBookingsResponse	"Scheduled bookings"
//	@Failure		400			{object}	ErrorResponse						"Bad request"
//	@Failure		401			{object}	ErrorResponse						"Unauthorized"
//	@Failure		403			{object}	ErrorResponse						"Forbidden"
//	@Failure		404			{object}	ErrorResponse						"Facility not found"
//	@Failure		500			{object}	ErrorResponse						"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/scheduled-bookings [get]
func (app *application) getScheduledFacilityBookingsHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Make sure facility belongs to this venue before querying bookings.
	// This protects against cross-venue access.
	if err := app.requireFacilityBelongsToVenue(r.Context(), venueID, facilityID); err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	date, err := parseBookingDateFromQuery(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	scheduledBookings, err := app.store.Bookings.GetScheduledBookingsForVenueDate(
		r.Context(),
		venueID,
		facilityID,
		date,
	)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, ScheduledBookingsResponse{
		Bookings: app.scheduledBookingsToResponse(scheduledBookings),
	})
}

// getCanceledFacilityBookingsHandler godoc
//
//	@Summary		Get canceled bookings for a facility
//	@Description	Returns canceled bookings for one facility under a venue. This keeps canceled bookings separated per facility.
//	@Tags			Facility Bookings
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int									true	"Venue ID"
//	@Param			facilityID	path		int									true	"Facility ID"
//	@Param			date		query		string								false	"Booking date in YYYY-MM-DD format. Defaults to today's date in Nepal timezone."
//	@Success		200			{object}	FacilityCanceledBookingsResponse	"Canceled bookings"
//	@Failure		400			{object}	ErrorResponse						"Bad request"
//	@Failure		401			{object}	ErrorResponse						"Unauthorized"
//	@Failure		403			{object}	ErrorResponse						"Forbidden"
//	@Failure		404			{object}	ErrorResponse						"Facility not found"
//	@Failure		500			{object}	ErrorResponse						"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/canceled-bookings [get]
func (app *application) getCanceledFacilityBookingsHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Facility belongs-to-venue check is important because facilityID is global.
	// Example: /venues/1/facilities/99 must fail if facility 99 belongs to venue 2.
	if err := app.requireFacilityBelongsToVenue(r.Context(), venueID, facilityID); err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	date, err := parseBookingDateFromQuery(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	canceledBookings, err := app.store.Bookings.GetCanceledBookingsForVenueDate(
		r.Context(),
		venueID,
		facilityID,
		date,
	)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, CanceledBookingsResponse{
		Bookings: app.canceledBookingsToResponse(canceledBookings),
	})
}

// -----------------------------------------------------------------------------
// Payloads / Responses
// -----------------------------------------------------------------------------

// FacilityAvailableTimesResponse is the response wrapper for available facility slots.
type FacilityAvailableTimesResponse struct {
	AvailableTimes []FacilityAvailableTimeSlotResponse `json:"available_times"`
}

// FacilityBookingResponse wraps one booking response.
// If your codebase already has BookingResponse and bookingToResponse(),
// you can keep using app.bookingToResponse(booking) instead.
type FacilityBookingResponse struct {
	Booking any `json:"booking"`
}

// CreateFacilityBookingPayload is used by normal users to request a booking.
//
// Important:
// start_time and end_time should be sent as RFC3339 timestamps.
//
// Example:
//
//	{
//	  "start_time": "2026-05-12T18:00:00+05:45",
//	  "end_time": "2026-05-12T19:00:00+05:45"
//	}
type CreateFacilityBookingPayload struct {
	StartTime time.Time `json:"start_time" validate:"required"`
	EndTime   time.Time `json:"end_time" validate:"required"`
}

// CreateManualFacilityBookingPayload is used by the venue owner/admin to create
// an offline/manual booking.
//
// Manual bookings are useful when:
//   - customer calls the venue directly
//   - customer pays cash
//   - owner wants to block a time slot from the dashboard
//
// We still store it in the same bookings table so availability logic remains simple.
type CreateManualFacilityBookingPayload struct {
	StartTime time.Time `json:"start_time" validate:"required"`
	EndTime   time.Time `json:"end_time" validate:"required"`

	CustomerName  *string `json:"customer_name,omitempty" validate:"omitempty,max=120"`
	CustomerPhone *string `json:"customer_phone,omitempty" validate:"omitempty,max=30"`
	Note          *string `json:"note,omitempty" validate:"omitempty,max=500"`
}

// FacilityAvailableTimeSlotResponse is the API response model for facility availability.
//
// We keep this separate from bookings.AvailableTimeSlot because API response needs
// the `available` field for frontend/mobile compatibility.
//
// Example response:
//
//	{
//	  "start_time": "2026-05-14T07:00:00+05:45",
//	  "end_time": "2026-05-14T08:00:00+05:45",
//	  "price_per_hour": 1800,
//	  "available": true
//	}
type FacilityAvailableTimeSlotResponse struct {
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	PricePerHour int       `json:"price_per_hour"`
	Available    bool      `json:"available"`
}

// availableFacilityTimesHandler godoc
//
//	@Summary		Get hourly available times for a facility
//	@Description	Returns hourly available booking slots for one facility under a venue. Response shape matches the old venue-level available-times API, but availability is calculated at facility level.
//	@Tags			Facility Bookings
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int									true	"Venue ID"
//	@Param			facilityID	path		int									true	"Facility ID"
//	@Param			date		query		string								true	"Booking date in YYYY-MM-DD format, for example 2026-05-14"
//	@Success		200			{array}		FacilityAvailableTimeSlotResponse	"Hourly available time slots"
//	@Failure		400			{object}	ErrorResponse						"Bad Request"
//	@Failure		401			{object}	ErrorResponse						"Unauthorized"
//	@Failure		403			{object}	ErrorResponse						"Forbidden"
//	@Failure		404			{object}	ErrorResponse						"Facility not found"
//	@Failure		500			{object}	ErrorResponse						"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/available-times [get]
func (app *application) availableFacilityTimesHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// facility_id is globally unique, so always confirm it belongs to this venue.
	// This prevents cross-venue data access by guessing facility IDs.
	if err := app.requireFacilityBelongsToVenue(r.Context(), venueID, facilityID); err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	date, err := parseRequiredFacilityBookingDate(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	availableTimes, err := app.buildHourlyAvailableTimesForFacility(r, venueID, facilityID, date)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, availableTimes)
}

// bookFacilityHandler godoc
//
//	@Summary		Request booking for a facility
//	@Description	Creates a pending booking request for a specific facility under a venue. Availability and pricing are checked at facility level.
//	@Tags			Facility Bookings
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int								true	"Venue ID"
//	@Param			facilityID	path		int								true	"Facility ID"
//	@Param			payload		body		CreateFacilityBookingPayload	true	"Booking payload"
//	@Success		201			{object}	FacilityBookingResponse			"Booking request created"
//	@Failure		400			{object}	ErrorResponse					"Bad Request"
//	@Failure		401			{object}	ErrorResponse					"Unauthorized"
//	@Failure		403			{object}	ErrorResponse					"Forbidden"
//	@Failure		404			{object}	ErrorResponse					"Facility not found"
//	@Failure		409			{object}	ErrorResponse					"Time slot already booked"
//	@Failure		500			{object}	ErrorResponse					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/bookings [post]
func (app *application) bookFacilityHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.requireFacilityBelongsToVenue(r.Context(), venueID, facilityID); err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	var payload CreateFacilityBookingPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := validateBookingTimeRange(payload.StartTime, payload.EndTime); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := getUserFromContext(r)
	if user == nil {
		app.unauthorizedErrorResponse(w, r, fmt.Errorf("unauthorized"))
		return
	}

	totalPrice, err := app.calculateFacilityBookingPrice(
		r,
		venueID,
		facilityID,
		payload.StartTime,
		payload.EndTime,
	)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.ensureFacilityTimeIsAvailable(
		r,
		venueID,
		facilityID,
		payload.StartTime,
		payload.EndTime,
	); err != nil {
		app.conflictResponse(w, r, err)
		return
	}

	booking := &bookings.Booking{
		VenueID:    venueID,
		FacilityID: facilityID,
		UserID:     user.ID,

		StartTime:  payload.StartTime,
		EndTime:    payload.EndTime,
		TotalPrice: totalPrice,

		// Normal user booking starts as pending.
		// Venue owner can later accept/reject it.
		Status: "pending",
	}

	if _, err := app.store.Bookings.CreateBooking(r.Context(), booking); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Use your existing response mapper if you already have it.
	app.jsonResponse(w, http.StatusCreated, FacilityBookingResponse{
		Booking: app.bookingToResponse(booking),
	})
}

// createManualFacilityBookingHandler godoc
//
//	@Summary		Create manual booking for a facility
//	@Description	Creates a confirmed manual/offline booking for one facility. This is useful when an owner receives a booking by phone, walk-in, or cash payment.
//	@Tags			Facility Bookings
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int									true	"Venue ID"
//	@Param			facilityID	path		int									true	"Facility ID"
//	@Param			payload		body		CreateManualFacilityBookingPayload	true	"Manual booking payload"
//	@Success		201			{object}	FacilityBookingResponse				"Manual booking created"
//	@Failure		400			{object}	ErrorResponse						"Bad Request"
//	@Failure		401			{object}	ErrorResponse						"Unauthorized"
//	@Failure		403			{object}	ErrorResponse						"Forbidden"
//	@Failure		404			{object}	ErrorResponse						"Facility not found"
//	@Failure		409			{object}	ErrorResponse						"Time slot already booked"
//	@Failure		500			{object}	ErrorResponse						"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/facilities/{facilityID}/bookings/manual [post]
func (app *application) createManualFacilityBookingHandler(w http.ResponseWriter, r *http.Request) {
	venueID, facilityID, err := app.parseVenueAndFacilityID(r)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.requireFacilityBelongsToVenue(r.Context(), venueID, facilityID); err != nil {
		if errors.Is(err, facilities.ErrFacilityNotFound) {
			app.notFoundResponse(w, r, err)
			return
		}

		app.internalServerError(w, r, err)
		return
	}

	var payload CreateManualFacilityBookingPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := validateBookingTimeRange(payload.StartTime, payload.EndTime); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	user := getUserFromContext(r)
	if user == nil {
		app.unauthorizedErrorResponse(w, r, fmt.Errorf("unauthorized"))
		return
	}

	totalPrice, err := app.calculateFacilityBookingPrice(
		r,
		venueID,
		facilityID,
		payload.StartTime,
		payload.EndTime,
	)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	if err := app.ensureFacilityTimeIsAvailable(
		r,
		venueID,
		facilityID,
		payload.StartTime,
		payload.EndTime,
	); err != nil {
		app.conflictResponse(w, r, err)
		return
	}

	booking := &bookings.Booking{
		VenueID:    venueID,
		FacilityID: facilityID,

		// Manual bookings still need a user_id if your DB has user_id NOT NULL.
		// We use the current owner/admin user as the creator.
		// The actual walk-in customer details are stored in customer_name/customer_phone.
		UserID: user.ID,

		StartTime:  payload.StartTime,
		EndTime:    payload.EndTime,
		TotalPrice: totalPrice,

		// Manual bookings are confirmed immediately because the owner is creating them.
		Status: "confirmed",

		CustomerName:  cleanOptionalString(payload.CustomerName),
		CustomerPhone: cleanOptionalString(payload.CustomerPhone),
		Note:          cleanOptionalString(payload.Note),
	}

	if _, err := app.store.Bookings.CreateBooking(r.Context(), booking); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusCreated, FacilityBookingResponse{
		Booking: app.bookingToResponse(booking),
	})
}

// -----------------------------------------------------------------------------
// Helper functions
// -----------------------------------------------------------------------------

// parseRequiredFacilityBookingDate parses ?date=YYYY-MM-DD.
//
// For available-times, date should be required because availability is date-specific.
// Example:
//
//	GET /venues/5/facilities/2/available-times?date=2026-05-12
func parseRequiredFacilityBookingDate(r *http.Request) (time.Time, error) {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to load Nepal timezone: %w", err)
	}

	dateStr := strings.TrimSpace(r.URL.Query().Get("date"))
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("date query param is required, use YYYY-MM-DD")
	}

	date, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		return time.Time{}, fmt.Errorf("invalid date format, use YYYY-MM-DD")
	}

	return date, nil
}

// validateBookingTimeRange applies simple booking time rules.
//
// Keep this small and boring.
// Business rules like "minimum 1 hour" or "only 30-minute blocks" should live here
// so every booking handler follows the same behavior.
func validateBookingTimeRange(startTime, endTime time.Time) error {
	if startTime.IsZero() {
		return fmt.Errorf("start_time is required")
	}

	if endTime.IsZero() {
		return fmt.Errorf("end_time is required")
	}

	if !startTime.Before(endTime) {
		return fmt.Errorf("start_time must be before end_time")
	}

	duration := endTime.Sub(startTime)
	if duration < time.Hour {
		return fmt.Errorf("booking duration must be at least 1 hour")
	}

	// Optional but useful for sports venue bookings:
	// force booking to clean 30-minute intervals.
	if startTime.Minute()%30 != 0 || endTime.Minute()%30 != 0 {
		return fmt.Errorf("booking time must be in 30-minute intervals")
	}

	return nil
}

// buildHourlyAvailableTimesForFacility returns hourly bookable slots for one facility.
//
// This keeps the new facility-level API response compatible with the old venue-level API.
//
// Example pricing:
//
//	07:00 - 12:00, Rs. 1800/hr
//
// Output:
//
//	07:00 - 08:00
//	08:00 - 09:00
//	09:00 - 10:00
//	10:00 - 11:00
//	11:00 - 12:00
//
// Important:
// Pricing is still read from facility-level pricing.
// Existing bookings are still checked at facility level.
// Only the response shape is made similar to your old API.
func (app *application) buildHourlyAvailableTimesForFacility(
	r *http.Request,
	venueID int64,
	facilityID int64,
	date time.Time,
) ([]FacilityAvailableTimeSlotResponse, error) {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return nil, fmt.Errorf("failed to load Nepal timezone: %w", err)
	}

	localDate := date.In(loc)
	dayOfWeek := strings.ToLower(localDate.Weekday().String())

	pricingSlots, err := app.store.Bookings.GetPricingSlots(
		r.Context(),
		venueID,
		facilityID,
		dayOfWeek,
	)
	if err != nil {
		return nil, fmt.Errorf("get pricing slots: %w", err)
	}

	if len(pricingSlots) == 0 {
		return []FacilityAvailableTimeSlotResponse{}, nil
	}

	bookedIntervals, err := app.store.Bookings.GetBookingsForDate(
		r.Context(),
		venueID,
		facilityID,
		localDate,
	)
	if err != nil {
		return nil, fmt.Errorf("get booked intervals: %w", err)
	}

	sort.Slice(bookedIntervals, func(i, j int) bool {
		return bookedIntervals[i].Start.Before(bookedIntervals[j].Start)
	})

	var availableSlots []FacilityAvailableTimeSlotResponse

	for _, pricingSlot := range pricingSlots {
		pricingStart := combineDateWithClockTime(localDate, pricingSlot.StartTime, loc)
		pricingEnd := combineDateWithClockTime(localDate, pricingSlot.EndTime, loc)

		// Bad pricing data should not crash the endpoint.
		// Example: start_time >= end_time.
		if !pricingStart.Before(pricingEnd) {
			continue
		}

		hourlySlots := splitPricingSlotIntoHourlySlots(
			pricingStart,
			pricingEnd,
			pricingSlot.Price,
			bookedIntervals,
		)

		availableSlots = append(availableSlots, hourlySlots...)
	}

	sort.Slice(availableSlots, func(i, j int) bool {
		return availableSlots[i].StartTime.Before(availableSlots[j].StartTime)
	})

	return availableSlots, nil
}

// splitPricingSlotIntoHourlySlots converts one pricing interval into hourly slots.
//
// Example:
//
//	pricingStart = 07:00
//	pricingEnd   = 12:00
//
// Output:
//
//	07:00-08:00
//	08:00-09:00
//	09:00-10:00
//	10:00-11:00
//	11:00-12:00
//
// If a booking overlaps with an hourly slot, that slot is skipped.
// That means this endpoint returns only actually available slots,
// matching your old response behavior where available=true is always returned.
func splitPricingSlotIntoHourlySlots(
	pricingStart time.Time,
	pricingEnd time.Time,
	pricePerHour int,
	bookedIntervals []bookings.Interval,
) []FacilityAvailableTimeSlotResponse {
	const slotDuration = time.Hour

	var slots []FacilityAvailableTimeSlotResponse

	for slotStart := pricingStart; slotStart.Add(slotDuration).Equal(pricingEnd) || slotStart.Add(slotDuration).Before(pricingEnd); slotStart = slotStart.Add(slotDuration) {
		slotEnd := slotStart.Add(slotDuration)

		currentSlot := bookings.Interval{
			Start: slotStart,
			End:   slotEnd,
		}

		isBooked := isIntervalBooked(currentSlot, bookedIntervals)

		slots = append(slots, FacilityAvailableTimeSlotResponse{
			StartTime:    slotStart,
			EndTime:      slotEnd,
			PricePerHour: pricePerHour,

			// If the slot overlaps with an existing booking, it is not available.
			Available: !isBooked,
		})
	}

	return slots
}

// isIntervalBooked checks whether one slot overlaps any existing booking.
//
// We use overlap instead of exact match because bookings can be longer than 1 hour.
//
// Example:
// Existing booking:
//
//	08:30 - 10:30
//
// This should block:
//
//	08:00 - 09:00
//	09:00 - 10:00
//	10:00 - 11:00
func isIntervalBooked(slot bookings.Interval, bookedIntervals []bookings.Interval) bool {
	for _, booked := range bookedIntervals {
		if intervalsOverlap(slot, booked) {
			return true
		}
	}

	return false
}

// calculateFacilityBookingPrice calculates total price from facility pricing slots.
//
// This supports bookings that are fully inside one pricing slot.
// It also supports bookings that cross multiple pricing slots,
// as long as pricing covers the whole requested time.
//
// Example:
//
//	6pm-7pm = Rs. 1000/hr
//	7pm-8pm = Rs. 1200/hr
//
// Booking 6pm-8pm => 2200
func (app *application) calculateFacilityBookingPrice(
	r *http.Request,
	venueID int64,
	facilityID int64,
	startTime time.Time,
	endTime time.Time,
) (int, error) {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return 0, fmt.Errorf("failed to load Nepal timezone: %w", err)
	}

	localStart := startTime.In(loc)
	dayOfWeek := strings.ToLower(localStart.Weekday().String())

	pricingSlots, err := app.store.Bookings.GetPricingSlots(
		r.Context(),
		venueID,
		facilityID,
		dayOfWeek,
	)
	if err != nil {
		return 0, fmt.Errorf("get pricing slots: %w", err)
	}

	if len(pricingSlots) == 0 {
		return 0, fmt.Errorf("no pricing available for this facility on %s", dayOfWeek)
	}

	total := 0
	cursor := startTime

	for cursor.Before(endTime) {
		var matchedSlot *bookings.PricingSlot
		var matchedSlotEnd time.Time

		for i := range pricingSlots {
			slot := &pricingSlots[i]

			slotStart := combineDateWithClockTime(localStart, slot.StartTime, loc)
			slotEnd := combineDateWithClockTime(localStart, slot.EndTime, loc)

			// The cursor must be inside one pricing slot.
			if !cursor.Before(slotStart) && cursor.Before(slotEnd) {
				matchedSlot = slot
				matchedSlotEnd = slotEnd
				break
			}
		}

		if matchedSlot == nil {
			return 0, fmt.Errorf("requested time is outside facility pricing hours")
		}

		priceUntil := matchedSlotEnd
		if endTime.Before(priceUntil) {
			priceUntil = endTime
		}

		minutes := priceUntil.Sub(cursor).Minutes()
		if minutes <= 0 {
			return 0, fmt.Errorf("invalid pricing interval")
		}

		// Price is stored per hour.
		// We calculate proportionally and round up to avoid undercharging.
		partialPrice := math.Ceil((float64(matchedSlot.Price) / 60.0) * minutes)
		total += int(partialPrice)

		cursor = priceUntil
	}

	return total, nil
}

// ensureFacilityTimeIsAvailable checks whether requested time overlaps with
// existing pending or confirmed bookings.
//
// Repository should already filter by:
//
//	status IN ('pending', 'confirmed')
//
// because pending bookings should temporarily hold the slot until accepted/rejected.
func (app *application) ensureFacilityTimeIsAvailable(
	r *http.Request,
	venueID int64,
	facilityID int64,
	startTime time.Time,
	endTime time.Time,
) error {
	existingBookings, err := app.store.Bookings.GetBookingsForDate(
		r.Context(),
		venueID,
		facilityID,
		startTime,
	)
	if err != nil {
		return fmt.Errorf("get existing bookings: %w", err)
	}

	requested := bookings.Interval{
		Start: startTime,
		End:   endTime,
	}

	for _, existing := range existingBookings {
		if intervalsOverlap(requested, existing) {
			return fmt.Errorf("time slot is already booked")
		}
	}

	return nil
}

// combineDateWithClockTime takes a real date and a TIME-only value from Postgres,
// then combines them into one full timestamp.
//
// pricingSlot.StartTime stores only the clock part from Postgres TIME.
// Example:
//
//	date = 2026-05-12
//	clock = 18:00:00
//	result = 2026-05-12 18:00:00 Asia/Kathmandu
func combineDateWithClockTime(date time.Time, clock time.Time, loc *time.Location) time.Time {
	localDate := date.In(loc)

	return time.Date(
		localDate.Year(),
		localDate.Month(),
		localDate.Day(),
		clock.Hour(),
		clock.Minute(),
		clock.Second(),
		0,
		loc,
	)
}

// intervalsOverlap returns true when two intervals overlap.
//
// Touching boundaries are not overlapping.
// Example:
//
//	07:00-08:00 and 08:00-09:00 = no overlap
//	07:00-08:30 and 08:00-09:00 = overlap
func intervalsOverlap(a, b bookings.Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

// cleanOptionalString trims optional strings and converts empty strings to nil.
//
// This keeps your JSON and DB clean.
// Example:
//
//	"customer_name": ""
//
// becomes nil instead of storing an empty string.
func cleanOptionalString(value *string) *string {
	if value == nil {
		return nil
	}

	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}

	return &trimmed
}

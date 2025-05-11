package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"khel/internal/store"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// AvailableTimeSlot represents a free time interval for booking.
type AvailableTimeSlot struct {
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	PricePerHour int       `json:"price_per_hour"`
}

// HourlySlot represents one 1-hour booking bucket.
type HourlySlot struct {
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"`
	PricePerHour int       `json:"price_per_hour"`
	Available    bool      `json:"available"`
}

// AvailableTimes godoc
//
//	@Summary		List available time slots for a venue
//	@Description	Returns one-hour buckets (with availability) for a given venue/day.
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int			true	"Venue ID"
//	@Param			date	query		string		true	"Date in YYYY-MM-DD format"
//	@Success		200		{array}		HourlySlot	"Hourly availability"
//	@Failure		400		{object}	error		"Bad Request"
//	@Failure		500		{object}	error		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/available-times [get]
func (app *application) availableTimesHandler(w http.ResponseWriter, r *http.Request) {
	// Step 1: Parse venueID from URL path
	venueID, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Step 2: Parse `date` from query param in format YYYY-MM-DD
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		app.badRequestResponse(w, r, fmt.Errorf("missing date"))
		return
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	dayOfWeek := strings.ToLower(date.Weekday().String())

	// Step 3: Load pricing slots and bookings for the venue and the selected date
	pricingSlots, err := app.store.Bookings.GetPricingSlots(r.Context(), venueID, dayOfWeek)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	bookings, err := app.store.Bookings.GetBookingsForDate(r.Context(), venueID, date)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Step 4: Set the target timezone to Asia/Kathmandu
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Step 5: Convert bookings to intervals in Kathmandu time
	type TimeInterval struct {
		Start time.Time
		End   time.Time
	}
	var bookedIntervals []TimeInterval
	for _, b := range bookings {
		bookedIntervals = append(bookedIntervals, TimeInterval{
			Start: b.Start.In(loc),
			End:   b.End.In(loc),
		})
	}

	// Step 6: Define a helper function to check for overlaps
	overlaps := func(start, end time.Time) bool {
		for _, bi := range bookedIntervals {
			if start.Before(bi.End) && bi.Start.Before(end) {
				return true
			}
		}
		return false
	}

	// Step 7: Generate hourly time slots and mark availability
	var out []HourlySlot
	for _, ps := range pricingSlots {
		// Convert pricing slot times into the selected date in Nepal timezone
		slotStart := time.Date(date.Year(), date.Month(), date.Day(),
			ps.StartTime.Hour(), ps.StartTime.Minute(), 0, 0, loc)
		slotEnd := time.Date(date.Year(), date.Month(), date.Day(),
			ps.EndTime.Hour(), ps.EndTime.Minute(), 0, 0, loc)

		// Break the slot into 1-hour buckets
		for t := slotStart; !t.Add(time.Hour).After(slotEnd); t = t.Add(time.Hour) {
			tEnd := t.Add(time.Hour)

			out = append(out, HourlySlot{
				StartTime:    t,
				EndTime:      tEnd,
				PricePerHour: ps.Price,
				Available:    !overlaps(t, tEnd),
			})
		}
	}

	// Step 8: Encode the result as JSON and send response
	app.jsonResponse(w, http.StatusOK, out)
}

// BookVenuePayload represents the JSON payload to book a venue.
type BookVenuePayload struct {
	StartTime time.Time `json:"start_time" validate:"required"`
	EndTime   time.Time `json:"end_time" validate:"required,gtfield=StartTime"`
}

// BookVenue godoc
//
//	@Summary		Book a venue time slot
//	@Description	Books a venue for the specified time slot if available and calculates the total price based on the applicable pricing slot.
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Param			payload	body		BookVenuePayload	true	"Booking details payload"
//	@Success		201		{object}	store.Booking		"Booking created successfully"
//	@Failure		400		{object}	error				"Bad Request: Invalid input"
//	@Failure		409		{object}	error				"Conflict: Time slot is already booked"
//	@Failure		500		{object}	error				"Internal Server Error: Could not create booking"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/bookings [post]
func (app *application) bookVenueHandler(w http.ResponseWriter, r *http.Request) {
	venueIDStr := chi.URLParam(r, "venueID")
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid venue ID", http.StatusBadRequest)
		return
	}
	var payload BookVenuePayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// (Validation logic may be added here.)
	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
	}

	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Determine the day and fetch pricing slots for that day.
	localStart := payload.StartTime.In(loc)
	dayOfWeek := strings.ToLower(localStart.Weekday().String())
	pricingSlots, err := app.store.Bookings.GetPricingSlots(r.Context(), venueID, dayOfWeek)
	if err != nil || len(pricingSlots) == 0 {
		http.Error(w, "No pricing available for this day", http.StatusBadRequest)
		return
	}

	// Ensure the requested booking falls within one of the pricing slots.
	validSlot := false
	var applicablePrice int
	for _, ps := range pricingSlots {
		slotStart := time.Date(payload.StartTime.Year(), payload.StartTime.Month(), payload.StartTime.Day(),
			ps.StartTime.Hour(), ps.StartTime.Minute(), ps.StartTime.Second(), 0, loc)
		slotEnd := time.Date(payload.StartTime.Year(), payload.StartTime.Month(), payload.StartTime.Day(),
			ps.EndTime.Hour(), ps.EndTime.Minute(), ps.EndTime.Second(), 0, loc)
		// Check if the requested interval is within this pricing slot.
		if (payload.StartTime.Equal(slotStart) || payload.StartTime.After(slotStart)) &&
			(payload.EndTime.Equal(slotEnd) || payload.EndTime.Before(slotEnd)) {
			validSlot = true
			applicablePrice = ps.Price
			break
		}
	}
	if !validSlot {
		http.Error(w, "Requested time slot is not within available pricing intervals", http.StatusBadRequest)
		return
	}

	// Check for overlapping bookings.
	bookings, err := app.store.Bookings.GetBookingsForDate(r.Context(), venueID, payload.StartTime)
	if err != nil {
		http.Error(w, "Error checking bookings", http.StatusInternalServerError)
		return
	}
	requestedInterval := store.Interval{Start: payload.StartTime, End: payload.EndTime}
	for _, b := range bookings {
		if intervalsOverlap(requestedInterval, b) {
			http.Error(w, "Time slot is already booked", http.StatusConflict)
			return
		}
	}

	// Calculate total price.
	duration := payload.EndTime.Sub(payload.StartTime).Hours()
	totalPrice := int(duration * float64(applicablePrice))

	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Create the booking.
	booking := &store.Booking{
		VenueID:    venueID,
		UserID:     user.ID,
		StartTime:  payload.StartTime,
		EndTime:    payload.EndTime,
		TotalPrice: totalPrice,
		Status:     "pending",
	}
	if err := app.store.Bookings.CreateBooking(r.Context(), booking); err != nil {
		log.Printf("CreateBooking failed: %v", err)
		http.Error(w, "Error creating booking", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(booking)
}

// intervalsOverlap checks whether two intervals overlap.
func intervalsOverlap(a, b store.Interval) bool {
	return a.Start.Before(b.End) && b.Start.Before(a.End)
}

// UpdatePricingPayload represents the JSON payload to update pricing info.
type UpdatePricingPayload struct {
	DayOfWeek string `json:"day_of_week"`
	StartTime string `json:"start_time"` // Format "15:04:05"
	EndTime   string `json:"end_time"`   // Format "15:04:05"
	Price     int    `json:"price"`
}

// UpdateVenuePricing godoc
//
//	@Summary		Update a pricing slot for a venue
//	@Description	Allows venue owners to update the pricing information (day, time range, and price) for a specific pricing slot.
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int						true	"Venue ID"
//	@Param			pricingID	path		int						true	"Pricing Slot ID"
//	@Param			payload		body		UpdatePricingPayload	true	"Pricing update payload"
//	@Success		200			{object}	store.PricingSlot		"Pricing updated successfully"
//	@Failure		400			{object}	error					"Bad Request: Invalid input"
//	@Failure		500			{object}	error					"Internal Server Error: Could not update pricing"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/pricing/{pricingID} [put]
func (app *application) updateVenuePricingHandler(w http.ResponseWriter, r *http.Request) {
	venueIDStr := chi.URLParam(r, "venueID")
	pricingIDStr := chi.URLParam(r, "pricingID")

	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	pricingID, err := strconv.ParseInt(pricingIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	var payload UpdatePricingPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// Parse start_time and end_time.
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

	pricing := &store.PricingSlot{
		ID:        pricingID,
		VenueID:   venueID,
		DayOfWeek: strings.ToLower(payload.DayOfWeek),
		StartTime: startTime,
		EndTime:   endTime,
		Price:     payload.Price,
	}

	if err := app.store.Bookings.UpdatePricing(r.Context(), pricing); err != nil {
		http.Error(w, "Error updating pricing", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(pricing)
}

type CreatePricingPayload struct {
	DayOfWeek string `json:"day_of_week" validate:"required,oneof=sunday monday tuesday wednesday thursday friday saturday"`
	StartTime string `json:"start_time" validate:"required"` // format "15:04:05"
	EndTime   string `json:"end_time"   validate:"required"` // format "15:04:05"
	Price     int    `json:"price"      validate:"required,gt=0"`
}

// CreateVenuePricing godoc
//
//	@Summary		Create a new pricing slot for a venue
//	@Description	Adds a new day/time price rule to venue_pricing
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int						true	"Venue ID"
//	@Param			payload	body		CreatePricingPayload	true	"New pricing slot"
//	@Success		201		{object}	store.PricingSlot		"Pricing slot created"
//	@Failure		400		{object}	error					"Bad Request"
//	@Failure		500		{object}	error					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/pricing [post]
func (app *application) createVenuePricingHandler(w http.ResponseWriter, r *http.Request) {
	// 1) Parse venueID
	venueIDStr := chi.URLParam(r, "venueID")
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// 2) Decode + validate payload
	var payload CreatePricingPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// 3) Parse times
	startTime, err := time.Parse("15:04:05", payload.StartTime)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid start_time: %w", err))
		return
	}
	endTime, err := time.Parse("15:04:05", payload.EndTime)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid end_time: %w", err))
		return
	}

	// 4) Build model and call store
	ps := &store.PricingSlot{
		VenueID:   venueID,
		DayOfWeek: strings.ToLower(payload.DayOfWeek),
		StartTime: startTime,
		EndTime:   endTime,
		Price:     payload.Price,
	}
	if err := app.store.Bookings.CreatePricingSlot(r.Context(), ps); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 5) Return
	app.jsonResponse(w, http.StatusOK, ps)
}

// getPendingBookingsHandler godoc
//
//	@Summary		List pending booking requests for a venue
//	@Description	Returns all bookings with status="pending" for a given venue and date.
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int						true	"Venue ID"
//	@Param			date	query		string					true	"Date in YYYY-MM-DD format"
//	@Success		200		{array}		store.PendingBooking	"Pending bookings"
//	@Failure		400		{object}	error					"Bad Request"
//	@Failure		500		{object}	error					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/pending-bookings [get]
func (app *application) getPendingBookingsHandler(w http.ResponseWriter, r *http.Request) {
	// 1) parse venueID
	vid, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %w", err))
		return
	}

	// 2) parse date query
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		app.badRequestResponse(w, r, errors.New("missing date"))
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid date format: %w", err))
		return
	}

	// 3) fetch from store
	bookings, err := app.store.Bookings.GetPendingBookingsForVenueDate(r.Context(), vid, date)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 4) respond JSON
	app.jsonResponse(w, http.StatusOK, bookings)
}

// getScheduledBookingsHandler godoc
//
//	@Summary		List Scheduled booking requests for a venue
//	@Description	Returns all bookings with status="confirmed" for a given venue and date.
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int						true	"Venue ID"
//	@Param			date	query		string					true	"Date in YYYY-MM-DD format"
//	@Success		200		{array}		store.ScheduledBooking	"Scheduled bookings"
//	@Failure		400		{object}	error					"Bad Request"
//	@Failure		500		{object}	error					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/scheduled-bookings [get]
func (app *application) getScheduledBookingsHandler(w http.ResponseWriter, r *http.Request) {
	// 1) parse venueID
	vid, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %w", err))
		return
	}

	// 2) parse date query
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		app.badRequestResponse(w, r, errors.New("missing date"))
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid date format: %w", err))
		return
	}

	// 3) fetch from store
	bookings, err := app.store.Bookings.GetScheduledBookingsForVenueDate(r.Context(), vid, date)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 4) respond JSON
	app.jsonResponse(w, http.StatusOK, bookings)
}

// acceptBookingHandler godoc
//
//	@Summary		Accept a pending booking request
//	@Description	Marks the booking with status="pending" as "confirmed".
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path	int	true	"Venue ID"
//	@Param			bookingID	path	int	true	"Booking ID"
//	@Success		204
//	@Failure		400	{object}	error	"Bad Request"
//	@Failure		404	{object}	error	"Not Found"
//	@Failure		500	{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/pending-bookings/{bookingID}/accept [post]
func (app *application) acceptBookingHandler(w http.ResponseWriter, r *http.Request) {
	vid, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %w", err))
		return
	}
	bid, err := strconv.ParseInt(chi.URLParam(r, "bookingID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid bookingID: %w", err))
		return
	}

	if err := app.store.Bookings.AcceptBooking(r.Context(), vid, bid); err != nil {
		if err == sql.ErrNoRows {
			app.notFoundResponse(w, r, errors.New("not found"))
		} else {
			app.internalServerError(w, r, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// rejectBookingHandler godoc
//
//	@Summary		Reject a pending booking request
//	@Description	Marks the booking with status="pending" as "rejected".
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path	int	true	"Venue ID"
//	@Param			bookingID	path	int	true	"Booking ID"
//	@Success		204
//	@Failure		400	{object}	error	"Bad Request"
//	@Failure		404	{object}	error	"Not Found"
//	@Failure		500	{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/pending-bookings/{bookingID}/reject [post]
func (app *application) rejectBookingHandler(w http.ResponseWriter, r *http.Request) {
	vid, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID: %w", err))
		return
	}
	bid, err := strconv.ParseInt(chi.URLParam(r, "bookingID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid bookingID: %w", err))
		return
	}

	if err := app.store.Bookings.RejectBooking(r.Context(), vid, bid); err != nil {
		if err == sql.ErrNoRows {
			app.notFoundResponse(w, r, errors.New("not found"))
		} else {
			app.internalServerError(w, r, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

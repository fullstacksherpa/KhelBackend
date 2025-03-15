package main

import (
	"encoding/json"
	"khel/internal/store"
	"net/http"
	"sort"
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

// AvailableTimes godoc
//
//	@Summary		List available time slots for a venue
//	@Description	Returns the available booking time slots for a given venue and date by subtracting booked intervals from the venue’s pricing slots.
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Param			date	query		string				true	"Date in YYYY-MM-DD format"
//	@Success		200		{array}		AvailableTimeSlot	"List of available time slots"
//	@Failure		400		{object}	error				"Bad Request: Invalid input"
//	@Failure		500		{object}	error				"Internal Server Error: Could not retrieve available times"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/available-times [get]
func (app *application) availableTimesHandler(w http.ResponseWriter, r *http.Request) {
	venueIDStr := chi.URLParam(r, "venueID")
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		app.badRequestResponse(w, r, err)
		return
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	dayOfWeek := strings.ToLower(date.Weekday().String()) // e.g. "monday", "tuesday", etc.

	// Fetch pricing slots for this venue on the specified day.
	pricingSlots, err := app.store.Bookings.GetPricingSlots(r.Context(), venueID, dayOfWeek)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Get existing bookings for the venue on the given day.
	bookings, err := app.store.Bookings.GetBookingsForDate(r.Context(), venueID, date)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// For each pricing slot, subtract booked intervals.
	var availableSlots []AvailableTimeSlot
	for _, ps := range pricingSlots {
		// Create a full timestamp interval for the pricing slot on the specified date.
		slotStart := time.Date(date.Year(), date.Month(), date.Day(), ps.StartTime.Hour(), ps.StartTime.Minute(), ps.StartTime.Second(), 0, time.UTC)
		slotEnd := time.Date(date.Year(), date.Month(), date.Day(), ps.EndTime.Hour(), ps.EndTime.Minute(), ps.EndTime.Second(), 0, time.UTC)

		// Get available intervals within this pricing slot.
		availIntervals := subtractIntervals(store.Interval{Start: slotStart, End: slotEnd}, bookings)
		for _, interval := range availIntervals {
			availableSlots = append(availableSlots, AvailableTimeSlot{
				StartTime:    interval.Start,
				EndTime:      interval.End,
				PricePerHour: ps.Price,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(availableSlots)
}

// subtractIntervals subtracts booked intervals from a pricing slot.
// It returns the list of intervals that remain available.
func subtractIntervals(slot store.Interval, bookings []store.Interval) []store.Interval {
	// Sort bookings by start time.
	sort.Slice(bookings, func(i, j int) bool {
		return bookings[i].Start.Before(bookings[j].Start)
	})
	available := []store.Interval{}
	currentStart := slot.Start

	for _, b := range bookings {
		// Skip bookings that end before the current start.
		if !b.End.After(currentStart) {
			continue
		}
		// If booking starts after currentStart, then there's an available segment.
		if b.Start.After(currentStart) {
			avail := store.Interval{Start: currentStart, End: b.Start}
			if avail.End.After(avail.Start) {
				available = append(available, avail)
			}
		}
		// Move currentStart forward.
		if b.End.After(currentStart) {
			currentStart = b.End
		}
		// If currentStart has passed the slot's end, break.
		if !currentStart.Before(slot.End) {
			break
		}
	}
	// Add any remaining time.
	if currentStart.Before(slot.End) {
		available = append(available, store.Interval{Start: currentStart, End: slot.End})
	}
	return available
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

	// Determine the day and fetch pricing slots for that day.
	dayOfWeek := strings.ToLower(payload.StartTime.Weekday().String())
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
			ps.StartTime.Hour(), ps.StartTime.Minute(), ps.StartTime.Second(), 0, time.UTC)
		slotEnd := time.Date(payload.StartTime.Year(), payload.StartTime.Month(), payload.StartTime.Day(),
			ps.EndTime.Hour(), ps.EndTime.Minute(), ps.EndTime.Second(), 0, time.UTC)
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

	// Create the booking.
	booking := &store.Booking{
		VenueID:    venueID,
		UserID:     1, // Replace with the actual authenticated user ID.
		StartTime:  payload.StartTime,
		EndTime:    payload.EndTime,
		TotalPrice: totalPrice,
		Status:     "confirmed",
	}
	if err := app.store.Bookings.CreateBooking(r.Context(), booking); err != nil {
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
//	@Tags			Venue
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

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
	"github.com/jackc/pgx/v5/pgconn"
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
//	@Param			date	query		string		true	"Date in 2025-06-28T00:00:00+05:45 format"
//	@Success		200		{array}		HourlySlot	"Hourly availability"
//	@Failure		400		{object}	error		"Bad Request"
//	@Failure		500		{object}	error		"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/available-times [get]
func (app *application) availableTimesHandler(w http.ResponseWriter, r *http.Request) {
	// Parse venueID from URL path to int64
	venueID, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	//  Parse `date` from query param in format 2025-06-28T00:00:00+05:45
	dateStr := r.URL.Query().Get("date")
	if dateStr == "" {
		app.badRequestResponse(w, r, fmt.Errorf("missing date"))
		return
	}

	//  Set the target timezone to Asia/Kathmandu
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Parse into named location so date.Location()==Asia/Kathmandu date will be
	// 2025-06-30 00:00:00 +0545 +0545. first +0545 is for offset and second is for location
	date, err := time.ParseInLocation(time.RFC3339, dateStr, loc)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid date format: %w", err))
		return
	}

	dateInKtm := date.In(loc)

	dayOfWeek := strings.ToLower(dateInKtm.Weekday().String())

	// Step 3: Load pricing slots and bookings for the venue and the selected date
	pricingSlots, err := app.store.Bookings.GetPricingSlots(r.Context(), venueID, dayOfWeek)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if len(pricingSlots) == 0 {
		app.jsonResponse(w, http.StatusOK, []HourlySlot{}) // no slots for this day
		return
	}

	//Prevents generating duplicate time buckets in output.
	// !unique[key]
	//This line checks:
	//unique[key] looks up the value for that key in the map.
	//If it does not exist, Go returns the zero value for the type bool, which is false.
	//!false → true
	//So !unique[key] becomes true the first time you encounter that key.
	unique := make(map[string]bool)
	var filtered []store.PricingSlot

	for _, ps := range pricingSlots {
		key := fmt.Sprintf("%s-%s-%d", ps.StartTime.Format("15:04"), ps.EndTime.Format("15:04"), ps.Price)
		if !unique[key] {
			unique[key] = true
			filtered = append(filtered, ps)
		}
	}

	pricingSlots = filtered

	bookings, err := app.store.Bookings.GetBookingsForDate(r.Context(), venueID, date)
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
	// Round current time to the next full hour in Kathmandu timezone
	now := time.Now().In(loc)
	if now.Minute() > 0 || now.Second() > 0 || now.Nanosecond() > 0 {
		now = now.Truncate(time.Hour).Add(time.Hour)
	} else {
		now = now.Truncate(time.Hour)
	}

	for _, ps := range pricingSlots {
		// Convert pricing slot times into the selected date in Nepal timezone
		slotStart := time.Date(date.Year(), date.Month(), date.Day(),
			ps.StartTime.Hour(), ps.StartTime.Minute(), 0, 0, loc)
		slotEnd := time.Date(date.Year(), date.Month(), date.Day(),
			ps.EndTime.Hour(), ps.EndTime.Minute(), 0, 0, loc)

		// Break the slot into 1-hour buckets
		for t := slotStart; !t.Add(time.Hour).After(slotEnd); t = t.Add(time.Hour) {
			tEnd := t.Add(time.Hour)

			if t.Before(now) {
				continue
			}

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

	//sample data
	//start_time 🎯: 2025-07-02 08:00:00 +0545 +0545
	//end_time 🎯: 2025-07-02 09:00:00 +0545 +0545
	//localStart 🎯: 2025-07-02 08:00:00 +0545 +0545
	//dayOfWeek 🎯: wednesday
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

type ManualBookingPayload struct {
	StartTime     time.Time `json:"start_time" validate:"required"`
	EndTime       time.Time `json:"end_time" validate:"required"`
	Price         int       `json:"price" validate:"required,gt=0"`
	Email         string    `json:"customer_email" validate:"omitempty,email"`
	CustomerName  string    `json:"customer_name" validate:"omitempty,max=100"`
	CustomerPhone string    `json:"customer_number" validate:"omitempty,nepaliphone"`
	Note          string    `json:"note" validate:"omitempty,max=255"`
}

// CreateManualBooking godoc
//
//	@Summary		Manually create a confirmed booking
//	@Description	Venue owners can create a confirmed booking manually by specifying start/end time, price, and optional customer details.
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int						true	"Venue ID"
//	@Param			payload	body		ManualBookingPayload	true	"Manual booking payload"
//	@Success		201		{object}	store.Booking			"Booking created successfully"
//	@Failure		400		{object}	error					"Bad Request: Invalid input or validation failed"
//	@Failure		401		{object}	error					"Unauthorized: Missing or invalid credentials"
//	@Failure		409		{object}	error					"Conflict: Time slot is already booked"
//	@Failure		500		{object}	error					"Internal Server Error: Could not create booking"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/bookings/manual [post]
func (app *application) createManualBookingHandler(w http.ResponseWriter, r *http.Request) {
	venueIDStr := chi.URLParam(r, "venueID")
	venueID, err := strconv.ParseInt(venueIDStr, 10, 64)
	if err != nil {
		http.Error(w, "Invalid venue ID", http.StatusBadRequest)
		return
	}
	var payload ManualBookingPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	fmt.Printf("⏰Start_time manualBooking: %s\n", payload.StartTime)
	fmt.Printf("⏰End_time manualBooking: %s\n", payload.EndTime)

	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
	}

	//sample data
	//frontend send like start_time 🎯 : 2025-06-29T11:00:00+05:45
	//end_time 🎯 : 2025-06-29T12:00:00+05:45
	//serverlog start_time 🎯: 2025-07-02 08:00:00 +0545 +0545
	//serverlog end_time 🎯: 2025-07-02 09:00:00 +0545 +0545

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

	user := getUserFromContext(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Determine userID
	var bookingUserID int64 = user.ID
	if payload.Email != "" {
		targetUser, err := app.store.Users.GetByEmail(r.Context(), payload.Email)
		if err == nil && targetUser != nil {
			bookingUserID = targetUser.ID
			fmt.Printf("Target user name is: %s targetUserID 🎯: %d\n ", targetUser.FirstName, targetUser.ID)
		} else {
			log.Printf("Email %s not found, using owner ID", payload.Email)
		}
	}
	//Trim empty strings before setting pointer fields (to avoid storing "" instead of NULL)
	var namePtr, phonePtr, notePtr *string
	if strings.TrimSpace(payload.CustomerName) != "" {
		namePtr = &payload.CustomerName
	}
	if strings.TrimSpace(payload.CustomerPhone) != "" {
		phonePtr = &payload.CustomerPhone
	}
	if strings.TrimSpace(payload.Note) != "" {
		notePtr = &payload.Note
	}

	booking := &store.Booking{
		VenueID:       venueID,
		UserID:        bookingUserID,
		StartTime:     payload.StartTime,
		EndTime:       payload.EndTime,
		TotalPrice:    payload.Price,
		Status:        "confirmed",
		CustomerName:  namePtr,
		CustomerPhone: phonePtr,
		Note:          notePtr,
	}

	if err := app.store.Bookings.CreateBooking(r.Context(), booking); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusCreated, booking)
}

// GetVenuePricing godoc
//
//	@Summary		Retrieve pricing slots for a venue (optionally filtered by day)
//	@Description	Returns all pricing slots for the specified venue. If the optional `day` query parameter is provided (e.g., `?day=monday`), only slots for that day will be returned.
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int					true	"Venue ID"
//	@Param			day		query		string				false	"Day of week (sunday, monday, tuesday, wednesday, thursday, friday, saturday)"
//	@Success		200		{array}		store.PricingSlot	"List of pricing slots"
//	@Failure		400		{object}	error				"Bad Request"
//	@Failure		500		{object}	error				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/pricing [get]
func (app *application) getVenuePricing(w http.ResponseWriter, r *http.Request) {
	// Step 1: Parse venueID from URL path
	venueID, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	dayOfWeek := r.URL.Query().Get("day")

	pricingSlots, err := app.store.Bookings.GetPricingSlots(r.Context(), venueID, dayOfWeek)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Step 8: Encode the result as JSON and send response
	app.jsonResponse(w, http.StatusOK, pricingSlots)
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

// DeleteVenuePricing godoc
//
//	@Summary		Delete a pricing slot for a venue
//	@Description	Allows venue owners to delete a specific pricing slot.
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path	int	true	"Venue ID"
//	@Param			pricingID	path	int	true	"Pricing Slot ID"
//	@Success		204			"No Content"
//	@Failure		400			{object}	error	"Bad Request: Invalid input"
//	@Failure		404			{object}	error	"Not Found: No such pricing slot"
//	@Failure		500			{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/pricing/{pricingID} [delete]
func (app *application) deleteVenuePricingHandler(w http.ResponseWriter, r *http.Request) {
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

	err = app.store.Bookings.DeletePricingSlot(r.Context(), venueID, pricingID)
	if err != nil {
		if strings.Contains(err.Error(), "no pricing slot found") {
			http.Error(w, err.Error(), http.StatusNotFound)
		} else {
			app.internalServerError(w, r, err)
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

type BulkCreatePricingPayload struct {
	//we use dive, required to ensure each item inside the array is individually validated.
	Slots []CreatePricingPayload `json:"slots" validate:"required,dive,required"`
}

type CreatePricingPayload struct {
	DayOfWeek string `json:"day_of_week" validate:"required,oneof=sunday monday tuesday wednesday thursday friday saturday"`
	StartTime string `json:"start_time" validate:"required"` // format "15:04:05"
	EndTime   string `json:"end_time"   validate:"required"` // format "15:04:05"
	Price     int    `json:"price"      validate:"required,gt=0"`
}

// CreateVenuePricing godoc
//
//	@Summary		Create one or more pricing slots for a venue
//	@Description	Adds new day/time price rules to venue_pricing in bulk
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int							true	"Venue ID"
//	@Param			payload	body		BulkCreatePricingPayload	true	"Pricing slots"
//	@Success		201		{array}		store.PricingSlot			"Pricing slots created"
//	@Failure		400		{object}	error						"Bad Request"
//	@Failure		500		{object}	error						"Internal Server Error"
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

	// 2) Decode + validate bulk payload
	var payload BulkCreatePricingPayload
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if err := Validate.Struct(payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	// 3) Build slice of store.PricingSlot
	slots := make([]*store.PricingSlot, 0, len(payload.Slots))
	for i, in := range payload.Slots {
		// parse times from string to time.time which is type for our model
		st, err := time.Parse("15:04:05", in.StartTime)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("slot %d: invalid start_time: %w", i, err))
			return
		}
		et, err := time.Parse("15:04:05", in.EndTime)
		if err != nil {
			app.badRequestResponse(w, r, fmt.Errorf("slot %d: invalid end_time: %w", i, err))
			return
		}
		if !st.Before(et) {
			app.badRequestResponse(w, r, fmt.Errorf("slot %d: start_time must be before end_time", i))
			return
		}

		slots = append(slots, &store.PricingSlot{
			VenueID:   venueID,
			DayOfWeek: strings.ToLower(strings.TrimSpace(in.DayOfWeek)),
			StartTime: st,
			EndTime:   et,
			Price:     in.Price,
		})
	}

	// 4) Bulk insert in one transaction using batch
	if err := app.store.Bookings.CreatePricingSlotsBatch(r.Context(), slots); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 5) Return 201 + full slice (with IDs)
	app.jsonResponse(w, http.StatusCreated, slots)
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

	loc, _ := time.LoadLocation("Asia/Kathmandu")

	// Parse date string in Kathmandu local time
	date, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid date format: %w", err))
		return
	}

	//date will be Parsed time is: 2025-06-29 00:00:00 +0545 +0545

	// 3) fetch from store
	bookings, err := app.store.Bookings.GetScheduledBookingsForVenueDate(r.Context(), vid, date)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 4) respond JSON
	app.jsonResponse(w, http.StatusOK, bookings)
}

// getCanceledBookingsHandler godoc
//
//	@Summary		List Canceled booking requests for a venue
//	@Description	Returns all bookings with status="canceled" for a given venue and date.
//	@Tags			Venue-Owner
//	@Accept			json
//	@Produce		json
//	@Param			venueID	path		int						true	"Venue ID"
//	@Param			date	query		string					true	"Date in YYYY-MM-DD format"
//	@Success		200		{array}		store.ScheduledBooking	"Scheduled bookings"
//	@Failure		400		{object}	error					"Bad Request"
//	@Failure		500		{object}	error					"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/canceled-bookings [get]
func (app *application) getCanceledBookingsHandler(w http.ResponseWriter, r *http.Request) {
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

	loc, _ := time.LoadLocation("Asia/Kathmandu")

	// Parse date string in Kathmandu local time
	date, err := time.ParseInLocation("2006-01-02", dateStr, loc)
	if err != nil {
		app.badRequestResponse(w, r, fmt.Errorf("invalid date format: %w", err))
		return
	}

	//date will be Parsed time is: 2025-06-29 00:00:00 +0545 +0545

	// 3) fetch from store
	bookings, err := app.store.Bookings.GetCanceledBookingsForVenueDate(r.Context(), vid, date)
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
			return
		}

		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "unique_confirmed_bookings_per_venue_time" {
			app.conflictResponse(w, r, errors.New("booking with this time already exists"))
			return
		} else {
			app.internalServerError(w, r, err)
			return
		}
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

// cancelBookingHandler godoc
//
//	@Summary		Cancel a pending booking request or confirmed booking
//	@Description	Marks the booking with status="pending or confirmed" as "canceled".
//	@Tags			Venue
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path	int	true	"Venue ID"
//	@Param			bookingID	path	int	true	"Booking ID"
//	@Success		204
//	@Failure		400	{object}	error	"Bad Request"
//	@Failure		404	{object}	error	"Not Found"
//	@Failure		500	{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/cancel-bookings/{bookingID} [post]
func (app *application) cancelBookingHandler(w http.ResponseWriter, r *http.Request) {
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

	// 🔐 Step 1: Extract userID from context (set by your auth middleware)
	authUser := getUserFromContext(r) // assuming this returns a struct with ID
	if authUser == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("check Bearer token"))
		return
	}

	// 🔍 Step 2: Check ownership
	ownerID, err := app.store.Bookings.GetBookingOwner(r.Context(), vid, bid)
	if err != nil {
		if err == sql.ErrNoRows {
			app.notFoundResponse(w, r, errors.New("booking not found"))
		} else {
			app.internalServerError(w, r, err)
		}
		return
	}

	if ownerID != authUser.ID {
		app.forbiddenResponse(w, r)
		return
	}

	// ✅ Step 3: Cancel booking
	if err := app.store.Bookings.CancelBooking(r.Context(), vid, bid); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getBookingsByUserHandler godoc
//
//	@Summary		List all bookings for a user
//	@Description	Returns every booking made by the specified user, including venue details.
//	@Tags			users
//	@Accept			json
//	@Produce		json
//	@Param			page	query		int		false	"Page number (1-based)"		default(1)	minimum(1)
//	@Param			limit	query		int		false	"Items per page (max 50)"	default(7)	minimum(1)	maximum(50)
//	@Param			status	query		string	false	"Filter by booking status"	Enums(confirmed, pending, rejected, done)
//	@Success		200		{array}		store.UserBooking
//	@Failure		400		{object}	error	"Bad Request"
//	@Failure		401		{object}	error	"Unauthorized"
//	@Failure		500		{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/users/bookings [get]
func (app *application) getBookingsByUserHandler(w http.ResponseWriter, r *http.Request) {
	user := getUserFromContext(r)
	if user == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("unauthorized request"))
		return
	}

	// Parse query params
	q := r.URL.Query()
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 50 {
		limit = 7
	}

	var status *string
	if s := q.Get("status"); s != "" {
		status = &s
	}

	filter := store.BookingFilter{
		Status: status,
		Page:   page,
		Limit:  limit,
	}

	bookings, err := app.store.Bookings.GetBookingsByUser(r.Context(), user.ID, filter)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, bookings)
}

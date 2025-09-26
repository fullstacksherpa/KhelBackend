package store

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

type GameFilterQuery struct {
	Limit         int            `validate:"gte=1"`          // Maximum number of results to return
	Offset        int            `validate:"gte=0"`          // Pagination offset
	Sort          string         `validate:"oneof=asc desc"` // Sorting order for start_time
	SportType     string         // Filter by sport type (e.g., "basketball")
	GameLevel     string         // Filter by game level (e.g., "intermediate")
	VenueID       int            // Filter by a specific venue id
	BookingStatus *BookingStatus // Filter based on booking status (nil = no filter)
	Status        *string        `validate:"omitempty,oneof=active cancelled completed"`

	// Location-based filtering
	UserLat float64 // User's latitude for radius filter
	UserLon float64 // User's longitude for radius filter
	Radius  int     // Radius in kilometers; 0 means no radius filtering

	// Time filtering
	StartAfter time.Time // Return games starting after this time
	EndBefore  time.Time // Return games ending before this time

	// Price filtering
	MinPrice int
	MaxPrice int
}

// Parse extracts query parameters from the request URL and populates the GameFilterQuery.
func (q GameFilterQuery) Parse(r *http.Request) (GameFilterQuery, error) {
	params := r.URL.Query()

	if sportType := params.Get("sport_type"); sportType != "" {
		q.SportType = sportType
	}

	if gameLevel := params.Get("game_level"); gameLevel != "" {
		q.GameLevel = gameLevel
	}

	if venueIDStr := params.Get("venue_id"); venueIDStr != "" {
		venueID, err := strconv.Atoi(venueIDStr)
		if err != nil {
			return q, fmt.Errorf("invalid venue_id: %w", err)
		}
		q.VenueID = venueID
	}

	if status := params.Get("booking_status"); status != "" {
		bs := BookingStatus(status) // convert string -> BookingStatus
		switch bs {
		case BookingPending, BookingRequested, BookingBooked, BookingRejected, BookingCancelled:
			q.BookingStatus = &bs
		default:
			return q, fmt.Errorf("invalid booking_status value: %s", status)
		}
	}

	if status := params.Get("status"); status != "" {
		// you may want to validate it's one of your enum values here
		q.Status = &status
	}

	if latStr := params.Get("lat"); latStr != "" {
		lat, err := strconv.ParseFloat(latStr, 64)
		if err != nil {
			return q, fmt.Errorf("invalid lat value: %w", err)
		}
		q.UserLat = lat
	}

	if lonStr := params.Get("lon"); lonStr != "" {
		lon, err := strconv.ParseFloat(lonStr, 64)
		if err != nil {
			return q, fmt.Errorf("invalid lon value: %w", err)
		}
		q.UserLon = lon
	}

	if radiusStr := params.Get("radius"); radiusStr != "" {
		radius, err := strconv.Atoi(radiusStr)
		if err != nil {
			return q, fmt.Errorf("invalid radius value: %w", err)
		}
		q.Radius = radius
	}

	if startAfterStr := params.Get("start_after"); startAfterStr != "" {
		startAfter, err := time.Parse(time.RFC3339, startAfterStr)
		if err != nil {
			return q, fmt.Errorf("invalid start_after value: %w", err)
		}
		q.StartAfter = startAfter
	}

	if endBeforeStr := params.Get("end_before"); endBeforeStr != "" {
		endBefore, err := time.Parse(time.RFC3339, endBeforeStr)
		if err != nil {
			return q, fmt.Errorf("invalid end_before value: %w", err)
		}
		q.EndBefore = endBefore
	}

	if minPriceStr := params.Get("min_price"); minPriceStr != "" {
		minPrice, err := strconv.Atoi(minPriceStr)
		if err != nil {
			return q, fmt.Errorf("invalid min_price: %w", err)
		}
		q.MinPrice = minPrice
	}

	if maxPriceStr := params.Get("max_price"); maxPriceStr != "" {
		maxPrice, err := strconv.Atoi(maxPriceStr)
		if err != nil {
			return q, fmt.Errorf("invalid max_price: %w", err)
		}
		q.MaxPrice = maxPrice
	}

	// Optional: Allow overriding the default pagination values.
	if limitStr := params.Get("limit"); limitStr != "" {
		limit, err := strconv.Atoi(limitStr)
		if err != nil {
			return q, fmt.Errorf("invalid limit: %w", err)
		}
		q.Limit = limit
	}

	if offsetStr := params.Get("offset"); offsetStr != "" {
		offset, err := strconv.Atoi(offsetStr)
		if err != nil {
			return q, fmt.Errorf("invalid offset: %w", err)
		}
		q.Offset = offset
	}

	if sort := params.Get("sort"); sort != "" {
		if sort != "asc" && sort != "desc" {
			return q, fmt.Errorf("invalid sort value: must be 'asc' or 'desc'")
		}
		q.Sort = sort
	}

	return q, nil
}

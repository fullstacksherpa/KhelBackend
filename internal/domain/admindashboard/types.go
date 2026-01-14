package admindashboard

import "context"

type Overview struct {
	// Users
	TotalUsers         int64 `json:"total_users"`
	TotalActiveUsers   int64 `json:"total_active_users"`
	TotalInactiveUsers int64 `json:"total_inactive_users"`

	// Games
	TotalFutsalGames     int64 `json:"total_futsal_games"`
	TotalBasketballGames int64 `json:"total_basketball_games"`
	TotalBadmintonGames  int64 `json:"total_badminton_games"`

	// Venue Requests
	TotalVenueRequests        int64 `json:"total_venue_requests"`
	TotalPendingVenueRequests int64 `json:"total_pending_venue_requests"`

	// Venues
	TotalVenues        int64 `json:"total_venues"`
	TotalActiveVenues  int64 `json:"total_active_venues"`
	TotalPendingVenues int64 `json:"total_pending_venues"`

	// Bookings
	TotalBookings          int64 `json:"total_bookings"`
	TotalConfirmedBookings int64 `json:"total_confirmed_bookings"`
	TotalPendingBookings   int64 `json:"total_pending_bookings"`
	TotalRejectedBookings  int64 `json:"total_rejected_bookings"`
	TotalCompletedBookings int64 `json:"total_completed_bookings"`
}

type Store interface {
	GetOverview(ctx context.Context) (*Overview, error)
}

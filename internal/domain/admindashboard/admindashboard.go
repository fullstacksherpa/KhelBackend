package admindashboard

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

func (r *Repository) GetOverview(ctx context.Context) (*Overview, error) {
	const q = `
		SELECT
			(SELECT COUNT(*) FROM users),
			(SELECT COUNT(*) FROM users WHERE is_active = true),
			(SELECT COUNT(*) FROM users WHERE is_active = false),

			(SELECT COUNT(*) FROM games WHERE sport_type = 'futsal'),
			(SELECT COUNT(*) FROM games WHERE sport_type = 'basketball'),
			(SELECT COUNT(*) FROM games WHERE sport_type = 'badminton'),

			(SELECT COUNT(*) FROM venue_requests),
			(SELECT COUNT(*) FROM venue_requests WHERE status = 'requested'),

			(SELECT COUNT(*) FROM venues),
			(SELECT COUNT(*) FROM venues WHERE status = 'active'),
			(SELECT COUNT(*) FROM venues WHERE status = 'requested'),

			(SELECT COUNT(*) FROM bookings),
			(SELECT COUNT(*) FROM bookings WHERE status = 'confirmed'),
			(SELECT COUNT(*) FROM bookings WHERE status = 'pending'),
			(SELECT COUNT(*) FROM bookings WHERE status = 'rejected'),
			(SELECT COUNT(*) FROM bookings WHERE status = 'done')
	`

	var o Overview
	err := r.db.QueryRow(ctx, q).Scan(
		&o.TotalUsers,
		&o.TotalActiveUsers,
		&o.TotalInactiveUsers,

		&o.TotalFutsalGames,
		&o.TotalBasketballGames,
		&o.TotalBadmintonGames,

		&o.TotalVenueRequests,
		&o.TotalPendingVenueRequests,

		&o.TotalVenues,
		&o.TotalActiveVenues,
		&o.TotalPendingVenues,

		&o.TotalBookings,
		&o.TotalConfirmedBookings,
		&o.TotalPendingBookings,
		&o.TotalRejectedBookings,
		&o.TotalCompletedBookings,
	)
	if err != nil {
		return nil, fmt.Errorf("get admin overview: %w", err)
	}

	return &o, nil
}

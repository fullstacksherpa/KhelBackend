package venueearnings

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

// GetVenueEarnings returns:
//   - one summary for selected date range
//   - paginated daily earning rows
//   - total count of daily rows for pagination
//
// Important:
// We filter by paid_at, not start_time.
// That means "today earning" means money collected/closed today.
// If you want "bookings played today", then use start_time instead.
func (r *Repository) GetVenueEarnings(ctx context.Context, venueID int64, filter GetVenueEarningsFilter) (*VenueEarningsResult, int, error) {
	var result VenueEarningsResult

	summaryQuery := `
		WITH filtered_bookings AS (
			SELECT
				b.id,
				b.payment_method,

				-- total_price is venue slot price only
				COALESCE(b.total_price, 0) AS slot_earning,

				-- final_amount is booking + inventory total
				COALESCE(b.final_amount, b.paid_amount, b.total_price, 0) AS total_earning,

				-- inventory earning is the difference between final amount and slot price
				GREATEST(
					COALESCE(b.final_amount, b.paid_amount, b.total_price, 0) - COALESCE(b.total_price, 0),
					0
				) AS inventory_earning

			FROM bookings b
			WHERE b.venue_id = $1
			  AND b.status = 'done'
			  AND b.paid_at IS NOT NULL
			  AND b.paid_at >= $2
			  AND b.paid_at < $3
		)
		SELECT
			COUNT(id)::INT AS total_bookings,

			COALESCE(SUM(slot_earning), 0)::INT AS slot_earning,
			COALESCE(SUM(inventory_earning), 0)::INT AS inventory_earning,
			COALESCE(SUM(total_earning), 0)::INT AS total_earning,

			COALESCE(SUM(total_earning) FILTER (
				WHERE LOWER(COALESCE(payment_method, '')) = 'cash'
			), 0)::INT AS cash_earning,

			COALESCE(SUM(total_earning) FILTER (
				WHERE LOWER(COALESCE(payment_method, '')) IN ('online', 'card', 'stripe', 'esewa', 'khalti')
			), 0)::INT AS online_earning,

			COALESCE(SUM(total_earning) FILTER (
				WHERE payment_method IS NULL
				   OR LOWER(COALESCE(payment_method, '')) NOT IN ('cash', 'online', 'card', 'stripe', 'esewa', 'khalti')
			), 0)::INT AS other_earning

		FROM filtered_bookings;
	`

	err := r.db.QueryRow(ctx, summaryQuery, venueID, filter.StartDate, filter.EndDate).Scan(
		&result.Summary.TotalBookings,
		&result.Summary.SlotEarning,
		&result.Summary.InventoryEarning,
		&result.Summary.TotalEarning,
		&result.Summary.CashEarning,
		&result.Summary.OnlineEarning,
		&result.Summary.OtherEarning,
	)
	if err != nil {
		return nil, 0, err
	}

	result.Summary.Period = string(filter.Period)
	result.Summary.StartDate = filter.StartDate
	result.Summary.EndDate = filter.EndDate

	var total int

	countQuery := `
		WITH daily_rows AS (
			SELECT
				-- Group by Nepal calendar date, not UTC date.
				(b.paid_at AT TIME ZONE 'Asia/Kathmandu')::DATE AS earning_date
			FROM bookings b
			WHERE b.venue_id = $1
			  AND b.status = 'done'
			  AND b.paid_at IS NOT NULL
			  AND b.paid_at >= $2
			  AND b.paid_at < $3
			GROUP BY earning_date
		)
		SELECT COUNT(*)::INT FROM daily_rows;
	`

	if err := r.db.QueryRow(ctx, countQuery, venueID, filter.StartDate, filter.EndDate).Scan(&total); err != nil {
		return nil, 0, err
	}

	dailyQuery := `
		WITH filtered_bookings AS (
			SELECT
				-- This makes frontend chart show Nepal date like 2026-05-11.
				(b.paid_at AT TIME ZONE 'Asia/Kathmandu')::DATE AS earning_date,

				COALESCE(b.total_price, 0) AS slot_earning,

				COALESCE(b.final_amount, b.paid_amount, b.total_price, 0) AS total_earning,

				GREATEST(
					COALESCE(b.final_amount, b.paid_amount, b.total_price, 0) - COALESCE(b.total_price, 0),
					0
				) AS inventory_earning

			FROM bookings b
			WHERE b.venue_id = $1
			  AND b.status = 'done'
			  AND b.paid_at IS NOT NULL
			  AND b.paid_at >= $2
			  AND b.paid_at < $3
		)
		SELECT
			earning_date::TEXT AS earning_date,
			COUNT(*)::INT AS total_bookings,
			COALESCE(SUM(slot_earning), 0)::INT AS slot_earning,
			COALESCE(SUM(inventory_earning), 0)::INT AS inventory_earning,
			COALESCE(SUM(total_earning), 0)::INT AS total_earning
		FROM filtered_bookings
		GROUP BY earning_date
		ORDER BY earning_date DESC
		LIMIT $4 OFFSET $5;
	`

	rows, err := r.db.Query(ctx, dailyQuery, venueID, filter.StartDate, filter.EndDate, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	daily := make([]DailyEarning, 0)

	for rows.Next() {
		var item DailyEarning

		err := rows.Scan(
			&item.Date,
			&item.TotalBookings,
			&item.SlotEarning,
			&item.InventoryEarning,
			&item.TotalEarning,
		)
		if err != nil {
			return nil, 0, err
		}

		daily = append(daily, item)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	result.Daily = daily

	return &result, total, nil
}

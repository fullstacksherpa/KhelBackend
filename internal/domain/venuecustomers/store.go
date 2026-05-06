package venuecustomers

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

// ListVenueCustomers returns users who booked only this venue.
// Important security rule:
// Every query is filtered by b.venue_id = $1, so venue owners cannot see customers
// from other venues.
func (r *Repository) ListVenueCustomers(ctx context.Context, venueID int64, filter ListCustomersFilter) ([]VenueCustomer, int, error) {
	if filter.Limit <= 0 {
		filter.Limit = 15
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Segment == "" {
		filter.Segment = SegmentAll
	}

	whereSegment, orderBy := customerSegmentSQL(filter.Segment)

	query := fmt.Sprintf(`
		WITH booking_inventory AS (
			SELECT
				bii.booking_id,
				SUM(bii.quantity * bii.unit_price_snapshot)::INT AS inventory_total
			FROM booking_inventory_items bii
			WHERE bii.venue_id = $1
			GROUP BY bii.booking_id
		),
		customer_stats AS (
			SELECT
				u.id AS user_id,
				u.first_name,
				u.last_name,
				(u.first_name || ' ' || u.last_name) AS full_name,
				u.email::TEXT AS email,
				u.phone,
				u.profile_picture_url,

				COUNT(b.id)::INT AS total_bookings,
				COUNT(*) FILTER (WHERE b.status = 'pending')::INT AS pending_bookings,
				COUNT(*) FILTER (WHERE b.status = 'confirmed')::INT AS confirmed_bookings,
				COUNT(*) FILTER (WHERE b.status = 'done')::INT AS done_bookings,
				COUNT(*) FILTER (WHERE b.status = 'canceled')::INT AS canceled_bookings,
				COUNT(*) FILTER (WHERE b.status = 'rejected')::INT AS rejected_bookings,

				COALESCE(SUM(b.total_price), 0)::INT AS total_booking_spend,
				COALESCE(SUM(bi.inventory_total), 0)::INT AS total_inventory_spend,

				COALESCE(
					SUM(
						COALESCE(
							b.final_amount,
							b.total_price + COALESCE(bi.inventory_total, 0)
						)
					),
					0
				)::INT AS total_spend,

				MAX(b.created_at) AS last_booked_at,
				MAX(b.end_time) FILTER (WHERE b.status = 'done') AS last_played_at,

				CASE
					WHEN COUNT(b.id) = 0 THEN 0
					ELSE ROUND(
						(COUNT(*) FILTER (WHERE b.status = 'canceled')::NUMERIC / COUNT(b.id)::NUMERIC) * 100,
						2
					)
				END::FLOAT AS cancellation_rate,

				LEAST(
					100,
					GREATEST(
						0,
						(
							50
							+ (COUNT(*) FILTER (WHERE b.status = 'done') * 8)
							+ (COUNT(*) FILTER (WHERE b.status = 'confirmed') * 5)
							+ LEAST(
								COALESCE(
									SUM(
										COALESCE(
											b.final_amount,
											b.total_price + COALESCE(bi.inventory_total, 0)
										)
									),
									0
								) / 1000,
								15
							)
							- (COUNT(*) FILTER (WHERE b.status = 'canceled') * 15)
							- (COUNT(*) FILTER (WHERE b.status = 'rejected') * 8)
						)
					)
				)::INT AS reliability_score
			FROM bookings b
			INNER JOIN users u ON u.id = b.user_id
			LEFT JOIN booking_inventory bi ON bi.booking_id = b.id
			WHERE b.venue_id = $1
			GROUP BY
				u.id,
				u.first_name,
				u.last_name,
				u.email,
				u.phone,
				u.profile_picture_url
		),
		filtered AS (
			SELECT *
			FROM customer_stats
			%s
		)
		SELECT
			user_id,
			first_name,
			last_name,
			full_name,
			email,
			phone,
			profile_picture_url,

			total_bookings,
			pending_bookings,
			confirmed_bookings,
			done_bookings,
			canceled_bookings,
			rejected_bookings,

			total_booking_spend,
			total_inventory_spend,
			total_spend,

			last_booked_at,
			last_played_at,
			cancellation_rate,
			reliability_score,

			COUNT(*) OVER()::INT AS total_count
		FROM filtered
		%s
		LIMIT $2 OFFSET $3
	`, whereSegment, orderBy)

	rows, err := r.db.Query(ctx, query, venueID, filter.Limit, filter.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list venue customers: %w", err)
	}
	defer rows.Close()

	customers := []VenueCustomer{}
	total := 0

	for rows.Next() {
		var c VenueCustomer

		err := rows.Scan(
			&c.UserID,
			&c.FirstName,
			&c.LastName,
			&c.FullName,
			&c.Email,
			&c.Phone,
			&c.ProfilePictureURL,

			&c.TotalBookings,
			&c.PendingBookings,
			&c.ConfirmedBookings,
			&c.DoneBookings,
			&c.CanceledBookings,
			&c.RejectedBookings,

			&c.TotalBookingSpend,
			&c.TotalInventorySpend,
			&c.TotalSpend,

			&c.LastBookedAt,
			&c.LastPlayedAt,
			&c.CancellationRate,
			&c.ReliabilityScore,

			&total,
		)
		if err != nil {
			return nil, 0, fmt.Errorf("scan venue customer: %w", err)
		}

		c.Tags = buildCustomerTags(c)
		customers = append(customers, c)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate venue customers: %w", err)
	}

	return customers, total, nil
}

// customerSegmentSQL keeps customer grouping logic in one place.
// These thresholds are intentionally simple.
// Later, after real data, you can move thresholds to config.
func customerSegmentSQL(segment Segment) (where string, orderBy string) {
	switch segment {
	case SegmentRegular:
		return `
			WHERE total_bookings >= 3
			  AND cancellation_rate < 35
		`, `
			ORDER BY total_bookings DESC, last_booked_at DESC NULLS LAST
		`

	case SegmentHighValue:
		return `
			WHERE total_spend >= 5000
		`, `
			ORDER BY total_spend DESC, last_booked_at DESC NULLS LAST
		`

	case SegmentRisky:
		return `
			WHERE reliability_score < 50
			   OR cancellation_rate >= 40
		`, `
			ORDER BY reliability_score ASC, cancellation_rate DESC
		`

	case SegmentCancelOften:
		return `
			WHERE canceled_bookings >= 2
			   OR cancellation_rate >= 30
		`, `
			ORDER BY cancellation_rate DESC, canceled_bookings DESC
		`

	case SegmentSpendMore:
		return ``, `
			ORDER BY total_spend DESC, total_inventory_spend DESC
		`

	default:
		return ``, `
			ORDER BY last_booked_at DESC NULLS LAST
		`
	}
}

func buildCustomerTags(c VenueCustomer) []string {
	tags := []string{}

	if c.TotalBookings >= 3 && c.CancellationRate < 35 {
		tags = append(tags, "regular_customer")
	}

	if c.TotalSpend >= 5000 {
		tags = append(tags, "high_value_customer")
	}

	if c.ReliabilityScore < 50 || c.CancellationRate >= 40 {
		tags = append(tags, "risky_customer")
	}

	if c.CanceledBookings >= 2 || c.CancellationRate >= 30 {
		tags = append(tags, "cancels_often")
	}

	if c.TotalInventorySpend > 0 {
		tags = append(tags, "buys_inventory")
	}

	return tags
}

func (r *Repository) GetVenueCustomerDetail(ctx context.Context, venueID, userID int64) (*VenueCustomerDetail, error) {
	customer, err := r.getVenueCustomer(ctx, venueID, userID)
	if err != nil {
		return nil, err
	}

	consumedItems, err := r.listConsumedItems(ctx, venueID, userID)
	if err != nil {
		return nil, err
	}

	recentBookings, err := r.listRecentBookings(ctx, venueID, userID)
	if err != nil {
		return nil, err
	}

	return &VenueCustomerDetail{
		Customer:       *customer,
		ConsumedItems:  consumedItems,
		RecentBookings: recentBookings,
	}, nil
}

func (r *Repository) getVenueCustomer(ctx context.Context, venueID, userID int64) (*VenueCustomer, error) {
	query := `
		WITH booking_inventory AS (
			SELECT
				bii.booking_id,
				SUM(bii.quantity * bii.unit_price_snapshot)::INT AS inventory_total
			FROM booking_inventory_items bii
			WHERE bii.venue_id = $1
			GROUP BY bii.booking_id
		)
		SELECT
			u.id AS user_id,
			u.first_name,
			u.last_name,
			(u.first_name || ' ' || u.last_name) AS full_name,
			u.email::TEXT AS email,
			u.phone,
			u.profile_picture_url,

			COUNT(b.id)::INT AS total_bookings,
			COUNT(*) FILTER (WHERE b.status = 'pending')::INT AS pending_bookings,
			COUNT(*) FILTER (WHERE b.status = 'confirmed')::INT AS confirmed_bookings,
			COUNT(*) FILTER (WHERE b.status = 'done')::INT AS done_bookings,
			COUNT(*) FILTER (WHERE b.status = 'canceled')::INT AS canceled_bookings,
			COUNT(*) FILTER (WHERE b.status = 'rejected')::INT AS rejected_bookings,

			COALESCE(SUM(b.total_price), 0)::INT AS total_booking_spend,
			COALESCE(SUM(bi.inventory_total), 0)::INT AS total_inventory_spend,

			COALESCE(
				SUM(
					COALESCE(
						b.final_amount,
						b.total_price + COALESCE(bi.inventory_total, 0)
					)
				),
				0
			)::INT AS total_spend,

			MAX(b.created_at) AS last_booked_at,
			MAX(b.end_time) FILTER (WHERE b.status = 'done') AS last_played_at,

			CASE
				WHEN COUNT(b.id) = 0 THEN 0
				ELSE ROUND(
					(COUNT(*) FILTER (WHERE b.status = 'canceled')::NUMERIC / COUNT(b.id)::NUMERIC) * 100,
					2
				)
			END::FLOAT AS cancellation_rate,

			LEAST(
				100,
				GREATEST(
					0,
					(
						50
						+ (COUNT(*) FILTER (WHERE b.status = 'done') * 8)
						+ (COUNT(*) FILTER (WHERE b.status = 'confirmed') * 5)
						+ LEAST(
							COALESCE(
								SUM(
									COALESCE(
										b.final_amount,
										b.total_price + COALESCE(bi.inventory_total, 0)
									)
								),
								0
							) / 1000,
							15
						)
						- (COUNT(*) FILTER (WHERE b.status = 'canceled') * 15)
						- (COUNT(*) FILTER (WHERE b.status = 'rejected') * 8)
					)
				)
			)::INT AS reliability_score
		FROM bookings b
		INNER JOIN users u ON u.id = b.user_id
		LEFT JOIN booking_inventory bi ON bi.booking_id = b.id
		WHERE b.venue_id = $1
		  AND b.user_id = $2
		GROUP BY
			u.id,
			u.first_name,
			u.last_name,
			u.email,
			u.phone,
			u.profile_picture_url
	`

	var c VenueCustomer

	err := r.db.QueryRow(ctx, query, venueID, userID).Scan(
		&c.UserID,
		&c.FirstName,
		&c.LastName,
		&c.FullName,
		&c.Email,
		&c.Phone,
		&c.ProfilePictureURL,

		&c.TotalBookings,
		&c.PendingBookings,
		&c.ConfirmedBookings,
		&c.DoneBookings,
		&c.CanceledBookings,
		&c.RejectedBookings,

		&c.TotalBookingSpend,
		&c.TotalInventorySpend,
		&c.TotalSpend,

		&c.LastBookedAt,
		&c.LastPlayedAt,
		&c.CancellationRate,
		&c.ReliabilityScore,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCustomerNotFound
		}
		return nil, fmt.Errorf("get venue customer: %w", err)
	}

	c.Tags = buildCustomerTags(c)

	return &c, nil
}

func (r *Repository) listConsumedItems(ctx context.Context, venueID, userID int64) ([]ConsumedItem, error) {
	query := `
		SELECT
			bii.inventory_item_id,
			bii.item_name_snapshot,
			SUM(bii.quantity)::INT AS quantity,
			SUM(bii.quantity * bii.unit_price_snapshot)::INT AS total_spend,
			MAX(bii.created_at) AS last_consumed_at
		FROM booking_inventory_items bii
		INNER JOIN bookings b ON b.id = bii.booking_id
		WHERE bii.venue_id = $1
		  AND b.venue_id = $1
		  AND b.user_id = $2
		GROUP BY
			bii.inventory_item_id,
			bii.item_name_snapshot
		ORDER BY quantity DESC, total_spend DESC
	`

	rows, err := r.db.Query(ctx, query, venueID, userID)
	if err != nil {
		return nil, fmt.Errorf("list consumed items: %w", err)
	}
	defer rows.Close()

	items := []ConsumedItem{}

	for rows.Next() {
		var item ConsumedItem

		err := rows.Scan(
			&item.InventoryItemID,
			&item.ItemName,
			&item.Quantity,
			&item.TotalSpend,
			&item.LastConsumedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan consumed item: %w", err)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate consumed items: %w", err)
	}

	return items, nil
}

func (r *Repository) listRecentBookings(ctx context.Context, venueID, userID int64) ([]CustomerBooking, error) {
	query := `
		WITH booking_inventory AS (
			SELECT
				bii.booking_id,
				SUM(bii.quantity * bii.unit_price_snapshot)::INT AS inventory_total
			FROM booking_inventory_items bii
			WHERE bii.venue_id = $1
			GROUP BY bii.booking_id
		)
		SELECT
			b.id,
			b.start_time,
			b.end_time,
			b.status::TEXT,
			b.total_price,
			COALESCE(bi.inventory_total, 0)::INT AS inventory_spend,
			COALESCE(b.final_amount, b.total_price + COALESCE(bi.inventory_total, 0))::INT AS final_amount,
			b.created_at
		FROM bookings b
		LEFT JOIN booking_inventory bi ON bi.booking_id = b.id
		WHERE b.venue_id = $1
		  AND b.user_id = $2
		ORDER BY b.created_at DESC
		LIMIT 10
	`

	rows, err := r.db.Query(ctx, query, venueID, userID)
	if err != nil {
		return nil, fmt.Errorf("list recent bookings: %w", err)
	}
	defer rows.Close()

	bookings := []CustomerBooking{}

	for rows.Next() {
		var b CustomerBooking

		err := rows.Scan(
			&b.BookingID,
			&b.StartTime,
			&b.EndTime,
			&b.Status,
			&b.BookingPrice,
			&b.InventorySpend,
			&b.FinalAmount,
			&b.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan recent booking: %w", err)
		}

		bookings = append(bookings, b)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent bookings: %w", err)
	}

	return bookings, nil
}

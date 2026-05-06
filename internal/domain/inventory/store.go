package inventory

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Store {
	return &Repository{db: db}
}

func (r *Repository) CreateInventoryItem(ctx context.Context, item *InventoryItem) error {
	count, err := r.CountInventoryItems(ctx, item.VenueID)
	if err != nil {
		return err
	}

	if count >= 10 {
		return ErrInventoryLimitReached
	}

	query := `
		INSERT INTO venue_inventory_items (
			venue_id,
			name,
			description,
			unit_price,
			image_url,
			stock_quantity,
			track_stock
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, is_active, created_at, updated_at
	`

	err = r.db.QueryRow(
		ctx,
		query,
		item.VenueID,
		item.Name,
		item.Description,
		item.UnitPrice,
		item.ImageURL,
		item.StockQuantity,
		item.TrackStock,
	).Scan(
		&item.ID,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("create inventory item: %w", err)
	}

	return nil
}

func (r *Repository) CountInventoryItems(ctx context.Context, venueID int64) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM venue_inventory_items
		WHERE venue_id = $1
	`

	var count int

	err := r.db.QueryRow(ctx, query, venueID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count inventory items: %w", err)
	}

	return count, nil
}

func (r *Repository) ListInventoryItems(ctx context.Context, venueID int64) ([]InventoryItem, error) {
	query := `
		SELECT
			id,
			venue_id,
			name,
			description,
			unit_price,
			image_url,
			stock_quantity,
			track_stock,
			is_active,
			created_at,
			updated_at
		FROM venue_inventory_items
		WHERE venue_id = $1
		ORDER BY created_at ASC
	`

	rows, err := r.db.Query(ctx, query, venueID)
	if err != nil {
		return nil, fmt.Errorf("list inventory items: %w", err)
	}
	defer rows.Close()

	items := []InventoryItem{}

	for rows.Next() {
		var item InventoryItem

		err := rows.Scan(
			&item.ID,
			&item.VenueID,
			&item.Name,
			&item.Description,
			&item.UnitPrice,
			&item.ImageURL,
			&item.StockQuantity,
			&item.TrackStock,
			&item.IsActive,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan inventory item: %w", err)
		}

		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *Repository) GetInventoryItemByID(ctx context.Context, venueID, itemID int64) (*InventoryItem, error) {
	query := `
		SELECT
			id,
			venue_id,
			name,
			description,
			unit_price,
			image_url,
			stock_quantity,
			track_stock,
			is_active,
			created_at,
			updated_at
		FROM venue_inventory_items
		WHERE venue_id = $1 AND id = $2
	`

	var item InventoryItem

	err := r.db.QueryRow(ctx, query, venueID, itemID).Scan(
		&item.ID,
		&item.VenueID,
		&item.Name,
		&item.Description,
		&item.UnitPrice,
		&item.ImageURL,
		&item.StockQuantity,
		&item.TrackStock,
		&item.IsActive,
		&item.CreatedAt,
		&item.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInventoryItemNotFound
		}
		return nil, fmt.Errorf("get inventory item: %w", err)
	}

	return &item, nil
}

func (r *Repository) UpdateInventoryItem(ctx context.Context, venueID, itemID int64, payload UpdateInventoryItemPayload) error {
	query := `UPDATE venue_inventory_items SET `
	args := []interface{}{}
	argCounter := 1

	if payload.Name != nil {
		query += fmt.Sprintf("name = $%d, ", argCounter)
		args = append(args, *payload.Name)
		argCounter++
	}

	if payload.Description != nil {
		query += fmt.Sprintf("description = $%d, ", argCounter)
		args = append(args, *payload.Description)
		argCounter++
	}

	if payload.UnitPrice != nil {
		query += fmt.Sprintf("unit_price = $%d, ", argCounter)
		args = append(args, *payload.UnitPrice)
		argCounter++
	}

	if payload.ImageURL != nil {
		query += fmt.Sprintf("image_url = $%d, ", argCounter)
		args = append(args, *payload.ImageURL)
		argCounter++
	}

	if payload.StockQuantity != nil {
		query += fmt.Sprintf("stock_quantity = $%d, ", argCounter)
		args = append(args, *payload.StockQuantity)
		argCounter++
	}

	if payload.TrackStock != nil {
		query += fmt.Sprintf("track_stock = $%d, ", argCounter)
		args = append(args, *payload.TrackStock)
		argCounter++
	}

	if payload.IsActive != nil {
		query += fmt.Sprintf("is_active = $%d, ", argCounter)
		args = append(args, *payload.IsActive)
		argCounter++
	}

	if len(args) == 0 {
		return nil
	}

	query += "updated_at = NOW(), "
	query = strings.TrimSuffix(query, ", ")

	query += fmt.Sprintf(" WHERE venue_id = $%d AND id = $%d", argCounter, argCounter+1)
	args = append(args, venueID, itemID)

	result, err := r.db.Exec(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("update inventory item: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrInventoryItemNotFound
	}

	return nil
}

func (r *Repository) DeleteInventoryItem(ctx context.Context, venueID, itemID int64) error {
	query := `
		UPDATE venue_inventory_items
		SET is_active = FALSE, updated_at = NOW()
		WHERE venue_id = $1 AND id = $2
	`

	result, err := r.db.Exec(ctx, query, venueID, itemID)
	if err != nil {
		return fmt.Errorf("delete inventory item: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrInventoryItemNotFound
	}

	return nil
}

func (r *Repository) ListActiveGames(ctx context.Context, venueID int64) ([]ActiveGame, error) {
	query := `
		SELECT
			id,
			venue_id,
			user_id,
			customer_name,
			customer_phone,
			start_time,
			end_time,
			total_price,
			status
		FROM bookings
		WHERE venue_id = $1
		  AND status = 'confirmed'
		  AND NOW() BETWEEN start_time AND end_time
		ORDER BY start_time ASC
	`

	rows, err := r.db.Query(ctx, query, venueID)
	if err != nil {
		return nil, fmt.Errorf("list active games: %w", err)
	}
	defer rows.Close()

	games := []ActiveGame{}

	for rows.Next() {
		var game ActiveGame

		err := rows.Scan(
			&game.BookingID,
			&game.VenueID,
			&game.UserID,
			&game.CustomerName,
			&game.CustomerPhone,
			&game.StartTime,
			&game.EndTime,
			&game.TotalPrice,
			&game.Status,
		)
		if err != nil {
			return nil, fmt.Errorf("scan active game: %w", err)
		}

		games = append(games, game)
	}

	return games, rows.Err()
}

// This function uses a transaction because adding an item to a booking must be atomic. If stock update succeeds but billing insert fails, the stock change should rollback. FOR UPDATE is used to prevent race conditions when multiple staff add the same item at the same time.
func (r *Repository) AddItemToBooking(
	ctx context.Context,
	venueID int64,
	bookingID int64,
	inventoryItemID int64,
	quantity int,
) (*BookingInventoryItem, error) {
	// Transaction is important here because this operation has multiple steps:
	// 1. check booking
	// 2. check inventory item
	// 3. reduce stock
	// 4. add/update booking item
	// If one step fails, everything should rollback.
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)

	var bookingExists bool

	// Only allow adding items to a booking/game that:
	// - belongs to this venue
	// - is confirmed
	// - is happening right now
	checkBookingQuery := `
		SELECT EXISTS (
			SELECT 1
			FROM bookings
			WHERE id = $1
			  AND venue_id = $2
			  AND status = 'confirmed'
		)
	`
	//TODO: AND NOW() BETWEEN start_time AND end_time add this in query later

	err = tx.QueryRow(ctx, checkBookingQuery, bookingID, venueID).Scan(&bookingExists)
	if err != nil {
		return nil, fmt.Errorf("check booking active: %w", err)
	}

	if !bookingExists {
		return nil, ErrBookingNotActive
	}

	var itemName string
	var unitPrice int
	var trackStock bool
	var stockQuantity sql.NullInt32

	// FOR UPDATE locks this inventory row during the transaction.
	// This prevents two requests from reading the same stock at the same time
	// and both subtracting from it incorrectly.
	itemQuery := `
		SELECT name, unit_price, track_stock, stock_quantity
		FROM venue_inventory_items
		WHERE id = $1
		  AND venue_id = $2
		  AND is_active = TRUE
		FOR UPDATE
	`

	err = tx.QueryRow(ctx, itemQuery, inventoryItemID, venueID).Scan(
		&itemName,
		&unitPrice,
		&trackStock,
		&stockQuantity,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrInventoryItemNotFound
		}
		return nil, fmt.Errorf("get inventory item for booking: %w", err)
	}

	// If stock tracking is enabled, make sure enough stock exists.
	if trackStock {
		if !stockQuantity.Valid || int(stockQuantity.Int32) < quantity {
			return nil, fmt.Errorf("not enough stock")
		}

		// Reduce stock only after confirming there is enough.
		_, err = tx.Exec(
			ctx,
			`
				UPDATE venue_inventory_items
				SET stock_quantity = stock_quantity - $1,
				    updated_at = NOW()
				WHERE id = $2 AND venue_id = $3
			`,
			quantity,
			inventoryItemID,
			venueID,
		)
		if err != nil {
			return nil, fmt.Errorf("decrease stock: %w", err)
		}
	}

	// Snapshot item name and price here.
	// Important: if owner changes item price later, old bills should not change.
	upsertQuery := `
		INSERT INTO booking_inventory_items (
			booking_id,
			venue_id,
			inventory_item_id,
			item_name_snapshot,
			unit_price_snapshot,
			quantity
		)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (booking_id, inventory_item_id)
		DO UPDATE SET
			quantity = booking_inventory_items.quantity + EXCLUDED.quantity,
			updated_at = NOW()
		RETURNING
			id,
			booking_id,
			venue_id,
			inventory_item_id,
			item_name_snapshot,
			unit_price_snapshot,
			quantity,
			unit_price_snapshot * quantity AS line_total,
			created_at,
			updated_at
	`

	var added BookingInventoryItem

	err = tx.QueryRow(
		ctx,
		upsertQuery,
		bookingID,
		venueID,
		inventoryItemID,
		itemName,
		unitPrice,
		quantity,
	).Scan(
		&added.ID,
		&added.BookingID,
		&added.VenueID,
		&added.InventoryItemID,
		&added.ItemNameSnapshot,
		&added.UnitPriceSnapshot,
		&added.Quantity,
		&added.LineTotal,
		&added.CreatedAt,
		&added.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("upsert booking inventory item: %w", err)
	}

	// Commit means all changes are saved.
	// After this, deferred rollback does nothing.
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit add item transaction: %w", err)
	}

	return &added, nil
}

func (r *Repository) ListBookingItems(ctx context.Context, venueID, bookingID int64) ([]BookingInventoryItem, error) {
	query := `
		SELECT
			id,
			booking_id,
			venue_id,
			inventory_item_id,
			item_name_snapshot,
			unit_price_snapshot,
			quantity,
			unit_price_snapshot * quantity AS line_total,
			created_at,
			updated_at
		FROM booking_inventory_items
		WHERE venue_id = $1 AND booking_id = $2
		ORDER BY created_at ASC
	`

	rows, err := r.db.Query(ctx, query, venueID, bookingID)
	if err != nil {
		return nil, fmt.Errorf("list booking items: %w", err)
	}
	defer rows.Close()

	items := []BookingInventoryItem{}

	for rows.Next() {
		var item BookingInventoryItem

		err := rows.Scan(
			&item.ID,
			&item.BookingID,
			&item.VenueID,
			&item.InventoryItemID,
			&item.ItemNameSnapshot,
			&item.UnitPriceSnapshot,
			&item.Quantity,
			&item.LineTotal,
			&item.CreatedAt,
			&item.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scan booking item: %w", err)
		}

		items = append(items, item)
	}

	return items, rows.Err()
}
func (r *Repository) GetBillingSummary(ctx context.Context, venueID, bookingID int64) (*BillingSummary, error) {
	query := `
		SELECT
			b.total_price AS booking_price,
			COALESCE(SUM(bii.unit_price_snapshot * bii.quantity), 0) AS inventory_total,
			b.total_price + COALESCE(SUM(bii.unit_price_snapshot * bii.quantity), 0) AS grand_total
		FROM bookings b
		LEFT JOIN booking_inventory_items bii
			ON bii.booking_id = b.id
		WHERE b.id = $1 AND b.venue_id = $2
		GROUP BY b.id, b.total_price
	`

	var summary BillingSummary

	err := r.db.QueryRow(ctx, query, bookingID, venueID).Scan(
		&summary.BookingPrice,
		&summary.InventoryTotal,
		&summary.GrandTotal,
	)
	if err != nil {
		return nil, fmt.Errorf("get billing summary: %w", err)
	}

	return &summary, nil
}

func (r *Repository) GetGameDetail(ctx context.Context, venueID, bookingID int64) (*GameDetail, error) {
	gameQuery := `
		SELECT
			id,
			venue_id,
			user_id,
			customer_name,
			customer_phone,
			start_time,
			end_time,
			total_price,
			status
		FROM bookings
		WHERE id = $1 AND venue_id = $2
	`

	var game ActiveGame

	err := r.db.QueryRow(ctx, gameQuery, bookingID, venueID).Scan(
		&game.BookingID,
		&game.VenueID,
		&game.UserID,
		&game.CustomerName,
		&game.CustomerPhone,
		&game.StartTime,
		&game.EndTime,
		&game.TotalPrice,
		&game.Status,
	)
	if err != nil {
		return nil, fmt.Errorf("get game: %w", err)
	}

	inventoryItems, err := r.ListInventoryItems(ctx, venueID)
	if err != nil {
		return nil, err
	}

	addedItems, err := r.ListBookingItems(ctx, venueID, bookingID)
	if err != nil {
		return nil, err
	}

	summary, err := r.GetBillingSummary(ctx, venueID, bookingID)
	if err != nil {
		return nil, err
	}

	return &GameDetail{
		Game:           game,
		Inventory:      inventoryItems,
		AddedItems:     addedItems,
		BillingSummary: *summary,
	}, nil
}

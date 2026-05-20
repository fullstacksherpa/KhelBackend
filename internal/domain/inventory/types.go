package inventory

import (
	"context"
	"errors"
	"time"
)

var ErrInventoryItemNotFound = errors.New("inventory item not found")
var ErrInventoryLimitReached = errors.New("venue inventory limit reached")
var ErrBookingNotActive = errors.New("booking is not active right now")

type InventoryItem struct {
	ID            int64     `json:"id"`
	VenueID       int64     `json:"venue_id"`
	Name          string    `json:"name"`
	Description   *string   `json:"description,omitempty"`
	UnitPrice     int       `json:"unit_price"`
	ImageURL      *string   `json:"image_url,omitempty"`
	StockQuantity *int      `json:"stock_quantity,omitempty"`
	TrackStock    bool      `json:"track_stock"`
	IsActive      bool      `json:"is_active"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

type UpdateInventoryItemPayload struct {
	Name          *string `json:"name,omitempty"`
	Description   *string `json:"description,omitempty"`
	UnitPrice     *int    `json:"unit_price,omitempty"`
	ImageURL      *string `json:"image_url,omitempty"`
	StockQuantity *int    `json:"stock_quantity,omitempty"`
	TrackStock    *bool   `json:"track_stock,omitempty"`
	IsActive      *bool   `json:"is_active,omitempty"`
}

type BookingInventoryItem struct {
	ID                int64     `json:"id"`
	BookingID         int64     `json:"booking_id"`
	VenueID           int64     `json:"venue_id"`
	InventoryItemID   int64     `json:"inventory_item_id"`
	ItemNameSnapshot  string    `json:"item_name"`
	UnitPriceSnapshot int       `json:"unit_price"`
	Quantity          int       `json:"quantity"`
	LineTotal         int       `json:"line_total"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type AddItemToBookingPayload struct {
	InventoryItemID int64 `json:"inventory_item_id" validate:"required"`
	Quantity        int   `json:"quantity" validate:"required,min=1"`
}

type BillingSummary struct {
	BookingPrice   int `json:"booking_price"`
	InventoryTotal int `json:"inventory_total"`
	GrandTotal     int `json:"grand_total"`
}

type ActiveGame struct {
	BookingID     int64     `json:"booking_id"`
	VenueID       int64     `json:"venue_id"`
	UserID        int64     `json:"user_id"`
	CustomerName  *string   `json:"customer_name,omitempty"`
	CustomerPhone *string   `json:"customer_phone,omitempty"`
	StartTime     time.Time `json:"start_time"`
	EndTime       time.Time `json:"end_time"`
	TotalPrice    int       `json:"total_price"`
	Status        string    `json:"status"`
}

type GameDetail struct {
	Game           ActiveGame             `json:"game"`
	Inventory      []InventoryItem        `json:"inventory_items"`
	AddedItems     []BookingInventoryItem `json:"added_items"`
	BillingSummary BillingSummary         `json:"billing_summary"`
}

type Store interface {
	CreateInventoryItem(ctx context.Context, item *InventoryItem) error
	ListInventoryItems(ctx context.Context, venueID int64) ([]InventoryItem, error)
	GetInventoryItemByID(ctx context.Context, venueID, itemID int64) (*InventoryItem, error)
	UpdateInventoryItem(ctx context.Context, venueID, itemID int64, payload UpdateInventoryItemPayload) error
	DeleteInventoryItem(ctx context.Context, venueID, itemID int64) error
	CountInventoryItems(ctx context.Context, venueID int64) (int, error)

	ListActiveGames(ctx context.Context, venueID int64) ([]ActiveGame, error)
	GetGameDetail(ctx context.Context, venueID, bookingID int64) (*GameDetail, error)

	AddItemToBooking(ctx context.Context, venueID, bookingID, inventoryItemID int64, quantity int) (*BookingInventoryItem, error)
	ListBookingItems(ctx context.Context, venueID, bookingID int64) ([]BookingInventoryItem, error)
	GetBillingSummary(ctx context.Context, venueID, bookingID int64) (*BillingSummary, error)
}

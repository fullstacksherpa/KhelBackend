package venuecustomers

import (
	"context"
	"errors"
	"time"
)

var ErrCustomerNotFound = errors.New("venue customer not found")

// Segment is used by the frontend to request different customer groups.
//
// Supported values:
//   - all
//   - regular
//   - high_value
//   - risky
//   - cancel_often
//   - spend_more
type Segment string

const (
	SegmentAll         Segment = "all"
	SegmentRegular     Segment = "regular"
	SegmentHighValue   Segment = "high_value"
	SegmentRisky       Segment = "risky"
	SegmentCancelOften Segment = "cancel_often"
	SegmentSpendMore   Segment = "spend_more"
)

func IsValidSegment(s string) bool {
	// This converts string s to type Segment ,not a function call.
	switch Segment(s) {
	case "", SegmentAll, SegmentRegular, SegmentHighValue, SegmentRisky, SegmentCancelOften, SegmentSpendMore:
		return true
	default:
		return false
	}
}

type VenueCustomer struct {
	UserID            int64   `json:"user_id"`
	FirstName         string  `json:"first_name"`
	LastName          string  `json:"last_name"`
	FullName          string  `json:"full_name"`
	Email             string  `json:"email"`
	Phone             string  `json:"phone"`
	ProfilePictureURL *string `json:"profile_picture_url,omitempty"`

	TotalBookings     int `json:"total_bookings"`
	PendingBookings   int `json:"pending_bookings"`
	ConfirmedBookings int `json:"confirmed_bookings"`
	DoneBookings      int `json:"done_bookings"`
	CanceledBookings  int `json:"canceled_bookings"`
	RejectedBookings  int `json:"rejected_bookings"`

	TotalBookingSpend   int `json:"total_booking_spend"`
	TotalInventorySpend int `json:"total_inventory_spend"`
	TotalSpend          int `json:"total_spend"`

	LastBookedAt *time.Time `json:"last_booked_at,omitempty"`
	LastPlayedAt *time.Time `json:"last_played_at,omitempty"`

	CancellationRate float64  `json:"cancellation_rate"`
	ReliabilityScore int      `json:"reliability_score"`
	Tags             []string `json:"tags"`
}

type ConsumedItem struct {
	InventoryItemID int64      `json:"inventory_item_id"`
	ItemName        string     `json:"item_name"`
	Quantity        int        `json:"quantity"`
	TotalSpend      int        `json:"total_spend"`
	LastConsumedAt  *time.Time `json:"last_consumed_at,omitempty"`
}

type CustomerBooking struct {
	BookingID      int64     `json:"booking_id"`
	StartTime      time.Time `json:"start_time"`
	EndTime        time.Time `json:"end_time"`
	Status         string    `json:"status"`
	BookingPrice   int       `json:"booking_price"`
	InventorySpend int       `json:"inventory_spend"`
	FinalAmount    int       `json:"final_amount"`
	CreatedAt      time.Time `json:"created_at"`
}

type VenueCustomerDetail struct {
	Customer       VenueCustomer     `json:"customer"`
	ConsumedItems  []ConsumedItem    `json:"consumed_items"`
	RecentBookings []CustomerBooking `json:"recent_bookings"`
}

type ListCustomersFilter struct {
	Segment Segment
	Limit   int
	Offset  int
}

type Store interface {
	ListVenueCustomers(ctx context.Context, venueID int64, filter ListCustomersFilter) ([]VenueCustomer, int, error)
	GetVenueCustomerDetail(ctx context.Context, venueID, userID int64) (*VenueCustomerDetail, error)
}

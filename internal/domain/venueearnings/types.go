package venueearnings

import (
	"context"
	"time"
)

// Period is used by frontend to filter venue owner's earnings.
//
// Supported values:
//   - today
//   - this_week
//   - last_month
//   - custom
type Period string

const (
	PeriodToday     Period = "today"
	PeriodThisWeek  Period = "this_week"
	PeriodLastMonth Period = "last_month"
	PeriodCustom    Period = "custom"
)

func IsValidPeriod(s string) bool {
	switch Period(s) {
	case "", PeriodToday, PeriodThisWeek, PeriodLastMonth, PeriodCustom:
		return true
	default:
		return false
	}
}

// VenueEarningSummary is the main summary card for venue owner.
//
// Important money meaning:
//
// SlotEarning:
//   - Comes from bookings.total_price
//   - This is only the venue booking price
//
// InventoryEarning:
//   - Comes from final_amount - total_price
//   - Because final_amount already includes inventory spend
//
// TotalEarning:
//   - Comes from bookings.final_amount
//   - This is the real total amount venue owner earned from booking + inventory
type VenueEarningSummary struct {
	Period string `json:"period"`

	StartDate time.Time `json:"start_date"`
	EndDate   time.Time `json:"end_date"`

	TotalBookings int `json:"total_bookings"`

	SlotEarning      int `json:"slot_earning"`
	InventoryEarning int `json:"inventory_earning"`
	TotalEarning     int `json:"total_earning"`

	CashEarning   int `json:"cash_earning"`
	OnlineEarning int `json:"online_earning"`
	OtherEarning  int `json:"other_earning"`
}

// DailyEarning is useful for frontend chart/list.
// Date is string because frontend usually wants "2026-05-11" for chart labels.
type DailyEarning struct {
	Date string `json:"date"`

	TotalBookings int `json:"total_bookings"`

	SlotEarning      int `json:"slot_earning"`
	InventoryEarning int `json:"inventory_earning"`
	TotalEarning     int `json:"total_earning"`
}

type GetVenueEarningsFilter struct {
	Period Period

	StartDate time.Time
	EndDate   time.Time

	Limit  int
	Offset int
}

type VenueEarningsResult struct {
	Summary VenueEarningSummary `json:"summary"`
	Daily   []DailyEarning      `json:"daily"`
}

type Store interface {
	GetVenueEarnings(ctx context.Context, venueID int64, filter GetVenueEarningsFilter) (*VenueEarningsResult, int, error)
}

package main

import (
	"context"
	"fmt"
	"khel/internal/domain/venueearnings"
	"khel/internal/params"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

type venueEarningsResponse struct {
	Summary    venueearnings.VenueEarningSummary `json:"summary"`
	Daily      []venueearnings.DailyEarning      `json:"daily"`
	Pagination params.Pagination                 `json:"pagination"`
}

// getVenueEarningsHandler godoc
//
//	@Summary		Get venue earnings
//	@Description	Returns earning summary and daily earning breakdown for a venue owner. The endpoint supports period filters such as today, this_week, last_month, and custom date range. Dates are calculated using Nepal timezone.
//	@Tags			Venue-Owner-Earnings
//	@Accept			json
//	@Produce		json
//	@Param			venueID		path		int		true	"Venue ID"
//	@Param			period		query		string	false	"Earning period"	Enums(today,this_week,last_month,custom)	default(today)
//	@Param			start_date	query		string	false	"Start date for custom period. Required when period=custom. Format: YYYY-MM-DD"
//	@Param			end_date	query		string	false	"End date for custom period. Required when period=custom. Format: YYYY-MM-DD"
//	@Param			page		query		int		false	"Page number for daily breakdown pagination. Default: 1"
//	@Param			limit		query		int		false	"Items per page for daily breakdown pagination. Default: 15, max: 30"
//	@Success		200			{object}	envelope{data=venueEarningsResponse}
//	@Failure		400			{object}	error	"Bad Request: invalid venue ID, period, or custom date range"
//	@Failure		401			{object}	error	"Unauthorized"
//	@Failure		403			{object}	error	"Forbidden: venue does not belong to owner"
//	@Failure		500			{object}	error	"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/venues/{venueID}/earnings [get]
func (app *application) getVenueEarningsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	venueID, err := strconv.ParseInt(chi.URLParam(r, "venueID"), 10, 64)
	if err != nil || venueID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid venueID"))
		return
	}

	period := strings.TrimSpace(r.URL.Query().Get("period"))
	if period == "" {
		period = string(venueearnings.PeriodToday)
	}

	if !venueearnings.IsValidPeriod(period) {
		app.badRequestResponse(w, r, fmt.Errorf("invalid period %q", period))
		return
	}

	startDate, endDate, err := parseVenueEarningDateRange(r, venueearnings.Period(period))
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	p := params.ParsePagination(r.URL.Query())

	earnings, total, err := app.store.VenueEarnings.GetVenueEarnings(ctx, venueID, venueearnings.GetVenueEarningsFilter{
		Period:    venueearnings.Period(period),
		StartDate: startDate,
		EndDate:   endDate,
		Limit:     p.Limit,
		Offset:    p.Offset,
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	p.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, venueEarningsResponse{
		Summary:    earnings.Summary,
		Daily:      earnings.Daily,
		Pagination: p,
	})
}

// parseVenueEarningDateRange calculates earning date ranges in Nepal time.
//
// Your database stores timestamptz values like:
// 2025-05-08 01:15:00+00
//
// That is good. PostgreSQL stores the instant correctly.
//
// But for this app, "today" should mean Nepal's today,
// not server timezone and not UTC day.
//
// Example:
// Nepal today starts at:
// 2026-05-11 00:00:00 +0545
//
// When passed to PostgreSQL as timestamptz,
// PostgreSQL compares the correct UTC instant automatically.
func parseVenueEarningDateRange(r *http.Request, period venueearnings.Period) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation("Asia/Kathmandu")
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("failed to load Nepal timezone: %w", err)
	}

	now := time.Now().In(loc)

	year, month, day := now.Date()

	todayStart := time.Date(year, month, day, 0, 0, 0, 0, loc)

	switch period {
	case venueearnings.PeriodToday:
		return todayStart, todayStart.AddDate(0, 0, 1), nil

	case venueearnings.PeriodThisWeek:
		// Go Weekday:
		// Sunday = 0
		// Monday = 1
		//
		// In this app, week starts from Monday.
		weekday := int(todayStart.Weekday())
		if weekday == 0 {
			weekday = 7
		}

		startOfWeek := todayStart.AddDate(0, 0, -(weekday - 1))
		endOfWeek := startOfWeek.AddDate(0, 0, 7)

		return startOfWeek, endOfWeek, nil

	case venueearnings.PeriodLastMonth:
		// Example:
		// If today is 2026-05-11 in Nepal,
		// last_month means:
		// 2026-04-01 00:00 Nepal time
		// to
		// 2026-05-01 00:00 Nepal time
		firstDayThisMonth := time.Date(year, month, 1, 0, 0, 0, 0, loc)
		firstDayLastMonth := firstDayThisMonth.AddDate(0, -1, 0)

		return firstDayLastMonth, firstDayThisMonth, nil

	case venueearnings.PeriodCustom:
		startDateStr := strings.TrimSpace(r.URL.Query().Get("start_date"))
		endDateStr := strings.TrimSpace(r.URL.Query().Get("end_date"))

		if startDateStr == "" || endDateStr == "" {
			return time.Time{}, time.Time{}, fmt.Errorf("start_date and end_date are required when period is custom")
		}

		// Parse custom dates as Nepal dates.
		// So start_date=2026-05-01 means:
		// 2026-05-01 00:00:00 +0545
		startDate, err := time.ParseInLocation("2006-01-02", startDateStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid start_date format, use YYYY-MM-DD")
		}

		endDate, err := time.ParseInLocation("2006-01-02", endDateStr, loc)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("invalid end_date format, use YYYY-MM-DD")
		}

		if !endDate.After(startDate) {
			return time.Time{}, time.Time{}, fmt.Errorf("end_date must be after start_date")
		}

		// Make end_date inclusive for frontend.
		//
		// Frontend sends:
		// start_date=2026-05-01&end_date=2026-05-10
		//
		// SQL receives:
		// paid_at >= 2026-05-01 00:00 Nepal
		// paid_at <  2026-05-11 00:00 Nepal
		return startDate, endDate.AddDate(0, 0, 1), nil

	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid period")
	}
}

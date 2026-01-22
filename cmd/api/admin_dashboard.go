package main

import (
	"context"
	"khel/internal/domain/venues"
	"khel/internal/params"
	"net/http"
	"strings"
	"time"
)

// AdminOverview godoc
//
//	@Summary		Admin overview totals
//	@Description	Returns totals for admin dashboard: users, games, venue requests, venues.
//	@Tags			superadmin-overview
//	@Produce		json
//	@Success		200	{object}	admindashboard.Overview
//	@Failure		401	{object}	error
//	@Failure		403	{object}	error
//	@Failure		500	{object}	error
//	@Security		ApiKeyAuth
//	@Router			/superadmin/overview [get]
func (app *application) adminOverviewHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()

	out, err := app.store.AdminDashboard.GetOverview(ctx)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	_ = app.jsonResponse(w, http.StatusOK, out)
}

// Filters applied to the venue list (admin)
type AdminVenueListFilters struct {
	Sport  string `json:"sport"`  // "" when not filtered
	Status string `json:"status"` // "" when not filtered
}

// Full paginated response for admin venue listing
type VenueListWithMetaResponse struct {
	Venues     []VenueListResponse   `json:"venues"`
	Pagination params.Pagination     `json:"pagination"`
	Filters    AdminVenueListFilters `json:"filters"`
}

// @Summary		List venues (admin)
// @Description	Paginated list of venues with optional filters (sport, status).
// @Tags			superadmin-venue
// @Produce		json
// @Param			sport	query		string	false	"Filter by sport type"
// @Param			status	query		string	false	"Filter by venue status (active|requested|inactive)"
// @Param			page	query		int		false	"Page number"		default(1)
// @Param			limit	query		int		false	"Items per page"	default(15)
// @Success		200		{object}	VenueListWithMetaResponse
// @Security		ApiKeyAuth
// @Router			/superadmin/venues/ [get]
func (app *application) AdminlistVenuesHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	q := r.URL.Query()

	// ✅ shared pagination
	p := params.ParsePagination(q)

	// ✅ optional sport
	var sportPtr *string
	if s := strings.TrimSpace(q.Get("sport")); s != "" {
		sportPtr = &s
	}

	// ✅ optional status (default handled in repo)
	var statusPtr *string
	if s := strings.TrimSpace(q.Get("status")); s != "" {
		switch s {
		case "requested", "active", "rejected", "hold":
			statusPtr = &s
		default:
			app.badRequestResponse(w, r, errInvalidRequest("invalid status"))
			return
		}
	}

	filter := venues.AdminVenueFilter{
		Sport:      sportPtr,
		Status:     statusPtr,
		Pagination: p,
	}

	result, err := app.store.Venues.ListWithTotal(ctx, filter)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// ✅ compute pagination meta from total
	p.ComputeMeta(result.Total)

	// ✅ Convert to response format (no favorites)
	respVenues := make([]VenueListResponse, 0, len(result.Venues))
	for _, v := range result.Venues {
		respVenues = append(respVenues, VenueListResponse{
			ID:            v.ID,
			Name:          v.Name,
			Address:       v.Address,
			Location:      []float64{v.Longitude, v.Latitude},
			ImageURLs:     v.ImageURLs,
			OpenTime:      v.OpenTime,
			PhoneNumber:   v.PhoneNumber,
			Sport:         v.Sport,
			TotalReviews:  v.TotalReviews,
			AverageRating: v.AverageRating,
			// IsFavorite removed ✅
		})
	}

	out := VenueListWithMetaResponse{
		Venues:     respVenues,
		Pagination: p,
		Filters: AdminVenueListFilters{
			Sport:  strings.TrimSpace(q.Get("sport")),
			Status: strings.TrimSpace(q.Get("status")),
		},
	}

	_ = app.jsonResponse(w, http.StatusOK, out)
}

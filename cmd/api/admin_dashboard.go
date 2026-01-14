package main

import (
	"context"
	"net/http"
	"time"
)

// AdminOverview godoc
//
//	@Summary		Admin overview totals
//	@Description	Returns totals for admin dashboard: users, games, venue requests, venues.
//	@Tags			superadmin-overview
//	@Produce		json
//	@Success		200	{object}	adminoverview.Overview
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

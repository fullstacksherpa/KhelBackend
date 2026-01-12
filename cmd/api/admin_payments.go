package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"khel/internal/params"
)

// AdminListPaymentsHandler godoc
//
//	@Summary		List payments (admin)
//	@Description	Returns a paginated list of payments. Optional filters: status, since.
//	@Tags			Store-Admin-Payments
//	@Produce		json
//	@Param			status	query		string			false	"payment_status filter: pending|paid|failed|refunded|partially_refunded"
//	@Param			since	query		string			false	"RFC3339 timestamp; returns payments created_at >= since"
//	@Param			page	query		int				false	"Page number (default: 1)"
//	@Param			limit	query		int				false	"Items per page (default from server, max usually 100)"
//	@Success		200		{object}	map[string]any	"Envelope: { data: { payments, pagination, status, since } }"
//	@Failure		400		{object}	error			"Bad Request"
//	@Failure		401		{object}	error			"Unauthorized"
//	@Failure		403		{object}	error			"Forbidden"
//	@Failure		500		{object}	error			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/admin/payments [get]
func (app *application) adminListPaymentsHandler(w http.ResponseWriter, r *http.Request) {
	// --- auth: must be logged in ---
	current := getUserFromContext(r)
	if current == nil {
		app.unauthorizedErrorResponse(w, r, errors.New("not authorized"))
		return
	}

	// Keep handler snappy and consistent
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// --- role check: merchant ---
	isMerchant, err := app.store.AccessControl.UserHasRole(ctx, current.ID, "merchant")
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("check role: %w", err))
		return
	}
	if !isMerchant {
		app.forbiddenResponse(w, r)
		return
	}

	// --- filters ---
	q := r.URL.Query()
	status := strings.TrimSpace(q.Get("status")) // "" => no filter

	// since is optional; expect RFC3339
	var since *time.Time
	if rawSince := strings.TrimSpace(q.Get("since")); rawSince != "" {
		t, parseErr := time.Parse(time.RFC3339, rawSince)
		if parseErr != nil {
			app.badRequestResponse(w, r, fmt.Errorf("invalid since (must be RFC3339): %w", parseErr))
			return
		}
		since = &t
	}

	// --- pagination ---
	pg := params.ParsePagination(q)

	// --- query store ---
	payments, total, err := app.store.Sales.Payments.List(ctx, status, since, pg.Limit, pg.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	pg.ComputeMeta(total)

	// --- respond (jsonResponse wraps { data: ... }) ---
	if err := app.jsonResponse(w, http.StatusOK, map[string]any{
		"payments":   payments,
		"pagination": pg,
		"status":     status,
		"since":      since, // null if not provided
	}); err != nil {
		app.internalServerError(w, r, err)
		return
	}
}

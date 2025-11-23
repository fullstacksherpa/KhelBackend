package main

import (
	"context"
	"fmt"
	"khel/internal/params"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// GET /v1/admin/orders?status=pending|processing|shipped|delivered|cancelled|refunded&page=1&limit=20
func (app *application) adminListOrdersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	p := params.ParsePagination(r.URL.Query())

	ordersList, total, err := app.store.Sales.Orders.ListAll(ctx, status, p.Limit, p.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	p.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"orders":     ordersList,
		"pagination": p,
		"status":     status,
	})
}

// GET /v1/admin/orders/{orderID}
func (app *application) adminGetOrderHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "orderID")
	orderID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || orderID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid orderID"))
		return
	}

	detail, err := app.store.Sales.Orders.GetDetail(ctx, orderID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, detail)
}

// PATCH /v1/admin/orders/{orderID}/status
// Body: { "status": "shipped", "cancelled_reason": "Customer requested" }
func (app *application) adminUpdateOrderStatusHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	idStr := chi.URLParam(r, "orderID")
	orderID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || orderID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid orderID"))
		return
	}

	var in struct {
		Status          string  `json:"status"`
		CancelledReason *string `json:"cancelled_reason"`
	}
	if err := readJSON(w, r, &in); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	newStatus := strings.TrimSpace(in.Status)
	if newStatus == "" {
		app.badRequestResponse(w, r, fmt.Errorf("status is required"))
		return
	}

	valid := map[string]bool{
		"pending":    true,
		"processing": true,
		"shipped":    true,
		"delivered":  true,
		"cancelled":  true,
		"refunded":   true,
	}
	if !valid[newStatus] {
		app.badRequestResponse(w, r, fmt.Errorf("invalid status %q", newStatus))
		return
	}

	if err := app.store.Sales.Orders.UpdateStatus(ctx, orderID, newStatus, in.CancelledReason); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"message": "status updated",
		"status":  newStatus,
	})
}

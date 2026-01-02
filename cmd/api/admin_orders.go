package main

import (
	"context"
	"fmt"
	"khel/internal/domain/orders"
	"khel/internal/params"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

// AdminOrderListResponse is the payload inside your standard envelope { "data": ... }.
type AdminOrderListResponse struct {
	Orders     []orders.Order    `json:"orders"`
	Pagination params.Pagination `json:"pagination"`
	Status     string            `json:"status"` // applied filter (echoed back)
}

// AdminOrderDetailResponse is the payload inside { "data": ... }.
type AdminOrderDetailResponse = orders.OrderDetail

// AdminUpdateOrderStatusRequest is PATCH body.
type AdminUpdateOrderStatusRequest struct {
	Status          string  `json:"status" example:"shipped"`
	CancelledReason *string `json:"cancelled_reason,omitempty" example:"Customer requested"`
}

// AdminUpdateOrderStatusResponse is the payload inside { "data": ... }.
type AdminUpdateOrderStatusResponse struct {
	Message string `json:"message" example:"status updated"`
	Status  string `json:"status" example:"shipped"`
}

type envelope struct {
	Data any `json:"data"`
}

// adminListOrdersHandler godoc
//
//	@Summary		List orders (admin)
//	@Description	List all orders for the admin dashboard. Supports optional status filter and pagination.
//	@Tags			Store-Admin-Orders
//	@Produce		json
//	@Param			status	query		string	false	"Filter by status"	Enums(pending,processing,shipped,delivered,cancelled,refunded)
//	@Param			page	query		int		false	"Page number (default: 1)"
//	@Param			limit	query		int		false	"Items per page (default: 15, max: 30)"
//	@Success		200		{object}	envelope{data=AdminOrderListResponse}
//	@Failure		400		{object}	error	"Bad Request"
//	@Failure		500		{object}	error	"Internal Server Error"
//	@Router			/store/admin/orders [get]
//	@Security		ApiKeyAuth
func (app *application) adminListOrdersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	p := params.ParsePagination(r.URL.Query())

	if status != "" {
		valid := map[string]bool{
			"pending": true, "processing": true, "shipped": true,
			"delivered": true, "cancelled": true, "refunded": true,
		}
		if !valid[status] {
			app.badRequestResponse(w, r, fmt.Errorf("invalid status %q", status))
			return
		}
	}

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

// adminGetOrderHandler godoc
//
//	@Summary		Get order detail (admin)
//	@Description	Get a single order with its line items for the admin dashboard.
//	@Tags			Store-Admin-Orders
//	@Produce		json
//	@Param			orderID	path		int64	true	"Order ID"
//	@Success		200		{object}	envelope{data=AdminOrderDetailResponse}
//	@Failure		400		{object}	error	"Bad Request: invalid orderID"
//	@Failure		404		{object}	error	"Not Found: order not found"
//	@Failure		500		{object}	error	"Internal Server Error"
//	@Router			/store/admin/orders/{orderID} [get]
//	@Security		ApiKeyAuth
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

// adminUpdateOrderStatusHandler godoc
//
//	@Summary		Update order status (admin)
//	@Description	Update an order's status. If status is cancelled, cancelled_reason may be provided and cancelled_at is set.
//	@Tags			Store-Admin-Orders
//	@Accept			json
//	@Produce		json
//	@Param			orderID	path		int64							true	"Order ID"
//	@Param			body	body		AdminUpdateOrderStatusRequest	true	"Status update payload"
//	@Success		200		{object}	envelope{data=AdminUpdateOrderStatusResponse}
//	@Failure		400		{object}	error	"Bad Request: invalid payload/status"
//	@Failure		404		{object}	error	"Not Found: order not found"
//	@Failure		500		{object}	error	"Internal Server Error"
//	@Router			/store/admin/orders/{orderID}/status [patch]
//	@Security		ApiKeyAuth
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
		"pending": true, "processing": true, "shipped": true,
		"delivered": true, "cancelled": true, "refunded": true,
	}
	if !valid[newStatus] {
		app.badRequestResponse(w, r, fmt.Errorf("invalid status %q", newStatus))
		return
	}

	if err := app.store.Sales.Orders.UpdateStatus(ctx, orderID, newStatus, in.CancelledReason); err != nil {
		if strings.Contains(err.Error(), "not found") {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"message": "status updated",
		"status":  newStatus,
	})
}

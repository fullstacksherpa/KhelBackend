package main

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"khel/internal/domain/orders"
	"khel/internal/domain/paymentsrepo"
	"khel/internal/domain/storage"
	"khel/internal/params"
	"khel/internal/payments"

	"github.com/go-chi/chi/v5"
)

// ============ CART ============

// GET /v1/store/cart
func (app *application) getCartHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user := getUserFromContext(r)
	userID := user.ID

	// optional: ensure cart exists
	if _, err := app.store.Sales.Carts.EnsureActive(ctx, userID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	view, err := app.store.Sales.Carts.GetView(ctx, userID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, view)
}

// POST /v1/store/cart/items  {variant_id, qty}
func (app *application) addCartItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	user := getUserFromContext(r)
	userID := user.ID

	var in struct {
		VariantID int64 `json:"variant_id"`
		Qty       int   `json:"qty"`
	}
	if err := readJSON(w, r, &in); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if in.VariantID <= 0 || in.Qty <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("variant_id and qty are required"))
		return
	}

	if err := app.store.Sales.Carts.AddItem(ctx, userID, in.VariantID, in.Qty); err != nil {
		if strings.Contains(err.Error(), "variant not found") {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}
	app.jsonResponse(w, http.StatusCreated, map[string]string{"message": "item added"})
}

// PATCH /v1/store/cart/items/{itemID}
func (app *application) updateCartItemQtyHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user := getUserFromContext(r)
	userID := user.ID

	itemStr := chi.URLParam(r, "itemID")
	itemID, err := strconv.ParseInt(itemStr, 10, 64)
	if err != nil || itemID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid itemID"))
		return
	}

	var in struct {
		Qty int `json:"qty"`
	}
	if err := readJSON(w, r, &in); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if in.Qty <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("qty must be > 0"))
		return
	}

	if err := app.store.Sales.Carts.UpdateItemQty(ctx, userID, itemID, in.Qty); err != nil {
		if strings.Contains(err.Error(), "item not found") {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}
	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "updated"})
}

// DELETE /v1/store/cart/items/{itemID}
func (app *application) removeCartItemHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user := getUserFromContext(r)
	userID := user.ID

	itemStr := chi.URLParam(r, "itemID")
	itemID, err := strconv.ParseInt(itemStr, 10, 64)
	if err != nil || itemID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid itemID"))
		return
	}

	if err := app.store.Sales.Carts.RemoveItem(ctx, userID, itemID); err != nil {
		if strings.Contains(err.Error(), "item not found") {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}
	app.jsonResponse(w, http.StatusOK, map[string]string{"message": "removed"})
}

// DELETE /v1/store/cart
func (app *application) clearCartHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user := getUserFromContext(r)
	userID := user.ID

	if err := app.store.Sales.Carts.Clear(ctx, userID); err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]string{
		"message": "cart cleared",
	})
}

// GET /v1/admin/carts
// Method / path: GET /v1/admin/carts?status=active|converted|abandoned&include_expired=true|false&page=1&limit=20
func (app *application) adminListCartsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	status := strings.TrimSpace(r.URL.Query().Get("status"))
	includeExpired := strings.EqualFold(r.URL.Query().Get("include_expired"), "true")

	pagination := params.ParsePagination(r.URL.Query())

	carts, total, err := app.store.Sales.Carts.List(ctx, status, includeExpired, pagination.Limit, pagination.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	pagination.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"carts":      carts,
		"pagination": pagination,
		"status":     status,
	})
}

// GET /v1/admin/carts/{cartID}
func (app *application) adminGetCartHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	cartStr := chi.URLParam(r, "cartID")
	cartID, err := strconv.ParseInt(cartStr, 10, 64)
	if err != nil || cartID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid cartID"))
		return
	}

	view, err := app.store.Sales.Carts.GetViewByCartID(ctx, cartID)
	if err != nil {
		if strings.Contains(err.Error(), "cart not found") {
			app.notFoundResponse(w, r, err)
			return
		}
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, view)
}

// POST /v1/admin/carts/mark-abandoned
func (app *application) adminMarkExpiredCartsHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	// admin auth…

	n, err := app.store.Sales.Carts.MarkExpiredAsAbandoned(ctx)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"updated": n,
	})
}

// ============ CHECKOUT (ORDER + PAYMENT) ============

// GET /v1/store/orders
func (app *application) listMyOrdersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user := getUserFromContext(r)
	userID := user.ID

	p := params.ParsePagination(r.URL.Query())

	ordersList, total, err := app.store.Sales.Orders.ListByUser(ctx, userID, p.Limit, p.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	p.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"orders":     ordersList,
		"pagination": p,
	})
}

// GET /v1/store/orders/{orderID}
func (app *application) getMyOrderHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user := getUserFromContext(r)
	userID := user.ID

	idStr := chi.URLParam(r, "orderID")
	orderID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || orderID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid orderID"))
		return
	}

	detail, err := app.store.Sales.Orders.GetDetailForUser(ctx, userID, orderID)
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

// POST /v1/store/payments/webhook
func (app *application) paymentWebhookHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1) Determine provider: from query or header or payload field
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))

	// Collect payload as key-value map[string]string
	payload := make(map[string]string)

	switch r.Method {
	case http.MethodGet:
		// e.g., redirect-back-style webhooks
		for k, vals := range r.URL.Query() {
			if len(vals) > 0 {
				payload[k] = vals[0]
			}
		}
	default:
		// Try JSON first
		if err := readJSON(w, r, &payload); err != nil {
			// Fallback to form-encoded
			if err2 := r.ParseForm(); err2 == nil {
				for k, vals := range r.PostForm {
					if len(vals) > 0 {
						payload[k] = vals[0]
					}
				}
			} else {
				app.badRequestResponse(w, r,
					fmt.Errorf("invalid webhook payload: json=%v form=%v", err, err2))
				return
			}
		}
	}

	// Maybe provider is inside payload
	if provider == "" {
		if p, ok := payload["provider"]; ok {
			provider = p
		}
	}
	if provider == "" {
		app.badRequestResponse(w, r, fmt.Errorf("missing provider"))
		return
	}

	// 2) Extract gateway-specific reference (provider_ref)
	var providerRef string
	switch strings.ToLower(provider) {
	case "khalti":
		providerRef = payload["pidx"]
		if providerRef == "" {
			providerRef = payload["txnId"]
		}
	case "esewa":
		providerRef = payload["transaction_uuid"]
		if providerRef == "" {
			providerRef = payload["refId"]
		}
	default:
		app.badRequestResponse(w, r, fmt.Errorf("unsupported provider: %s", provider))
		return
	}

	if providerRef == "" {
		app.badRequestResponse(w, r,
			fmt.Errorf("missing provider identifier (pidx/transaction_uuid)"))
		return
	}

	// 3) Ask gateway to verify (never trust webhook payload directly)
	ver, err := app.payments.VerifyPayment(ctx, provider, payments.PaymentVerifyRequest{
		TransactionID: providerRef, // your Khalti/Esewa adapters handle this
		Data:          payload,
	})
	if err != nil {
		app.logger.Errorw("verify payment failed", "provider", provider, "ref", providerRef, "err", err)
		// Return 5xx so gateway might retry
		http.Error(w, "verification error", http.StatusInternalServerError)
		return
	}

	// 4) If not successful → ACK but don't mark paid
	if !ver.Success {
		// optional: log as failed
		app.logger.Infow("webhook verification not successful",
			"provider", provider, "ref", providerRef)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
		return
	}

	// 5) Map provider_ref -> internal payment row
	// You need a repo method like:
	//   GetByProviderRef(ctx, provider, providerRef) (*Payment, error)
	pay, err := app.store.Sales.Payments.GetByProviderRef(ctx, provider, providerRef)
	if err != nil {
		app.logger.Errorw("get payment by provider_ref failed",
			"provider", provider, "ref", providerRef, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if pay == nil {
		// unknown payment, but ACK so gateway doesn't spam retries
		app.logger.Warnw("webhook for unknown provider_ref",
			"provider", provider, "ref", providerRef)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
		return
	}

	// 6) Idempotency: if already paid, do nothing
	if strings.EqualFold(pay.Status, "paid") {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
		return
	}

	// 7) Mark paid (this also updates orders via MarkPaid logic)
	if err := app.store.Sales.Payments.MarkPaid(ctx, pay.ID); err != nil {
		app.logger.Errorw("MarkPaid failed",
			"payment_id", pay.ID, "provider", provider, "ref", providerRef, "err", err)
		http.Error(w, "failed to update payment", http.StatusInternalServerError)
		return
	}

	// 8) Optional: store raw webhook payload in payment_logs
	// _ = app.store.Sales.Payments.InsertLog(ctx, pay.ID, "webhook", payload)

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

// POST /v1/store/checkout
func (app *application) checkoutHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user := getUserFromContext(r)
	userID := user.ID

	var in struct {
		Shipping struct {
			Name, Phone, Address, City string
			PostalCode                 *string
			Country                    *string
		} `json:"shipping"`
		PaymentMethod *string `json:"payment_method"` // "khalti" | "esewa" | "cash_on_delivery"
	}
	if err := readJSON(w, r, &in); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	ship := orders.ShippingInfo{
		Name: in.Shipping.Name, Phone: in.Shipping.Phone, Address: in.Shipping.Address,
		City: in.Shipping.City, PostalCode: in.Shipping.PostalCode, Country: in.Shipping.Country,
	}

	// Optional: validate shipping fields here…

	// 1) Load cart and ensure not empty
	cartView, err := app.store.Sales.Carts.GetView(ctx, userID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if len(cartView.Items) == 0 {
		app.badRequestResponse(w, r, fmt.Errorf("cart is empty"))
		return
	}

	// 2) Create order (+ order items) and a pending payment (if online) in a single tx
	var (
		orderID int64
		payID   *int64
		method  string
	)
	if in.PaymentMethod != nil {
		method = *in.PaymentMethod
	} else {
		method = "cash_on_delivery"
	}

	err = app.store.WithSalesTx(ctx, func(s *storage.SalesTx) error {
		order, err := s.Orders.CreateFromCart(ctx, userID, ship, &method)
		if err != nil {
			return err
		}
		orderID = order.ID

		if method != "cash_on_delivery" {
			p, err := s.Payments.Create(ctx, &paymentsrepo.Payment{
				OrderID:     order.ID,
				Provider:    method,
				AmountCents: order.TotalCents,
				Currency:    "NPR",
				Status:      "pending",
			})
			if err != nil {
				return err
			}
			payID = &p.ID

			if err := s.Payments.SetPrimaryToOrder(ctx, order.ID, p.ID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 3) (Outside tx) If online payment, call gateway now
	var paymentURL string
	var paymentData map[string]string

	if payID != nil {
		resp, gerr := app.payments.InitiatePayment(ctx, method, payments.PaymentRequest{
			Amount:        float64(cartView.TotalCents) / 100.0,
			TransactionID: fmt.Sprintf("%d", *payID), // your internal payment id
			ProductName:   fmt.Sprintf("Order #%d", orderID),
			CustomerName:  ship.Name,
			CustomerPhone: ship.Phone,
			// CustomerEmail: "...",
		})
		_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, *payID, "response", resp.Data)
		if gerr != nil {
			_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, *payID, "error", map[string]any{
				"stage": "initiate",
				"error": gerr.Error(),
			})
			app.internalServerError(w, r, fmt.Errorf("payment init: %w", gerr))
			return
		}
		paymentURL = resp.PaymentURL
		paymentData = map[string]string{}
		for k, v := range resp.Data {
			paymentData[k] = fmt.Sprint(v)
		}

		// 4) Save provider_ref (e.g., pidx) after gateway returns (tiny update)
		if ref, ok := paymentData["pidx"]; ok {
			if err := app.store.Sales.Payments.SetProviderRef(ctx, *payID, ref, resp.Data); err != nil {
				app.internalServerError(w, r, err)
				return
			}
		}
	}

	app.jsonResponse(w, http.StatusCreated, map[string]any{
		"order_id": orderID,
		"payment_id": func() any {
			if payID == nil {
				return nil
			}
			return *payID
		}(),
		"payment_url":  paymentURL,
		"payment_data": paymentData, // eSewa form fields, etc.
	})
}

// POST /v1/store/payments/verify  {payment_id, method, data:{...}}
func (app *application) verifyPaymentHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var in struct {
		PaymentID int64             `json:"payment_id"`
		Method    string            `json:"method"` // "khalti" | "esewa" | ...
		Data      map[string]string `json:"data"`   // gateway-specific
	}
	if err := readJSON(w, r, &in); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	if in.PaymentID <= 0 || strings.TrimSpace(in.Method) == "" {
		app.badRequestResponse(w, r, fmt.Errorf("payment_id and method are required"))
		return
	}

	// 1) Load payment
	pay, err := app.store.Sales.Payments.GetByID(ctx, in.PaymentID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if pay == nil {
		app.notFoundResponse(w, r, fmt.Errorf("payment not found"))
		return
	}

	// 2) Enforce provider consistency
	if !strings.EqualFold(pay.Provider, in.Method) {
		app.badRequestResponse(w, r, fmt.Errorf("payment method mismatch: expected %s", pay.Provider))
		return
	}

	// 3) Idempotency: if already paid, short-circuit success
	if strings.EqualFold(pay.Status, "paid") {
		app.jsonResponse(w, http.StatusOK, map[string]any{"success": true, "idempotent": true})
		return
	}

	_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, in.PaymentID, "webhook", in.Data)

	ver, err := app.payments.VerifyPayment(ctx, in.Method, payments.PaymentVerifyRequest{
		TransactionID: fmt.Sprintf("%d", in.PaymentID), // your adapters use this or Data["pidx"]
		Data:          in.Data,
	})
	if err != nil {
		_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, in.PaymentID, "error", map[string]any{
			"stage":  "verify",
			"method": in.Method,
			"error":  err.Error(),
			"data":   in.Data,
		})
		_ = app.store.Sales.Payments.SetStatus(ctx, in.PaymentID, "failed")
		app.badRequestResponse(w, r, err)
		return
	}

	// 6) Transition status
	if ver.Success {
		if err := app.store.Sales.Payments.MarkPaid(ctx, in.PaymentID); err != nil {
			app.internalServerError(w, r, err)
			return
		}
	} else {
		// optional: set failed to help support visibility
		_ = app.store.Sales.Payments.SetStatus(ctx, in.PaymentID, "failed")
	}

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"success": ver.Success,
	})
}

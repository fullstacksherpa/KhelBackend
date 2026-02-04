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

// GetCart godoc
//
//	@Summary		Get user's cart
//	@Description	Retrieves the current user's active or checkout_pending shopping cart
//	@Tags			User-Cart
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	carts.CartView	"Cart retrieved successfully"
//	@Failure		401	{object}	error			"Unauthorized"
//	@Failure		500	{object}	error			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/cart [get]
func (app *application) getCartHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	user := getUserFromContext(r)
	userID := user.ID

	view, err := app.store.Sales.Carts.GetView(ctx, userID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	if view == nil {
		// Create a new cart
		cartID, err := app.store.Sales.Carts.GetOrCreateCart(ctx, userID)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}

		// Get the newly created cart view
		view, err = app.store.Sales.Carts.GetViewByCartID(ctx, cartID)
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}
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

func normalizeOrderStatusFilter(raw string) (string, error) {
	s := strings.TrimSpace(strings.ToLower(raw))
	if s == "" || s == "all" {
		return "", nil // empty means "no filter"
	}

	switch s {
	case "pending", "processing", "shipped", "delivered", "cancelled", "refunded", "awaiting_payment", "payment_failed":
		return s, nil
	default:
		return "", fmt.Errorf("invalid status")
	}
}

// ListMyOrders godoc
//
//	@Summary		List my orders
//	@Description	Returns a paginated list of orders for the authenticated user.
//	@Tags			Orders
//	@Produce		json
//	@Param			page	query		int				false	"Page number (default: 1)"				minimum(1)
//	@Param			limit	query		int				false	"Items per page (default: 15, max: 30)"	minimum(1)	maximum(30)
//	@Success		200		{object}	map[string]any	"orders list + pagination"
//	@Failure		401		{object}	error			"Unauthorized"
//	@Failure		500		{object}	error			"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/orders [get]
func (app *application) listMyOrdersHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	user := getUserFromContext(r)
	userID := user.ID

	p := params.ParsePagination(r.URL.Query())

	// ✅ Parse status filter
	status, err := normalizeOrderStatusFilter(r.URL.Query().Get("status"))
	if err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	ordersList, total, err := app.store.Sales.Orders.ListByUser(ctx, userID, status, p.Limit, p.Offset)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	p.ComputeMeta(total)

	app.jsonResponse(w, http.StatusOK, map[string]any{
		"filters": map[string]any{
			"status": status, // will be "" when no filter
		},
		"orders":     ordersList,
		"pagination": p,
	})
}

// GetMyOrder godoc
//
//	@Summary		Get my order detail
//	@Description	Returns order detail (order + items) for the authenticated user. Only returns the order if it belongs to the user.
//	@Tags			Orders
//	@Produce		json
//	@Param			orderID	path		int					true	"Order ID"	minimum(1)
//	@Success		200		{object}	orders.OrderDetail	"order detail"
//	@Failure		400		{object}	error				"Bad Request: invalid orderID"
//	@Failure		401		{object}	error				"Unauthorized"
//	@Failure		404		{object}	error				"Order not found"
//	@Failure		500		{object}	error				"Internal Server Error"
//	@Security		ApiKeyAuth
//	@Router			/store/orders/{orderID} [get]
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
//
// Webhook responsibilities:
//   - Parse payload (GET query params or POST JSON/form)
//   - Extract provider + provider_ref (pidx/transaction_uuid/refId)
//   - Verify with gateway (never trust webhook payload directly)
//   - Map provider_ref -> internal payment row
//   - Apply idempotent state transition:
//     If verified paid -> (TX) MarkPaid + ConvertCheckoutCart
//     Else -> optionally mark failed + UnlockCheckoutCart (depends on provider semantics)
//   - Always ACK 200 for unknown provider_ref to avoid retry spam
func (app *application) paymentWebhookHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// 1) Determine provider
	provider := strings.TrimSpace(r.URL.Query().Get("provider"))

	// 2) Collect payload into map[string]string (gateway-specific)
	payload := make(map[string]string)

	switch r.Method {
	case http.MethodGet:
		for k, vals := range r.URL.Query() {
			if len(vals) > 0 {
				payload[k] = vals[0]
			}
		}
	default:
		// Try JSON body first, fallback to form-encoded.
		if err := readJSON(w, r, &payload); err != nil {
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

	// Provider may be in payload
	if provider == "" {
		if p, ok := payload["provider"]; ok {
			provider = strings.TrimSpace(p)
		}
	}
	if provider == "" {
		app.badRequestResponse(w, r, fmt.Errorf("missing provider"))
		return
	}
	provider = strings.ToLower(provider)

	// 3) Extract provider_ref (gateway transaction identifier)
	var providerRef string
	switch provider {
	case "khalti":
		// Khalti commonly uses pidx
		providerRef = payload["pidx"]
		if providerRef == "" {
			providerRef = payload["txnId"]
		}
	case "esewa":
		// eSewa commonly uses transaction_uuid or refId depending on flow
		providerRef = payload["transaction_uuid"]
		if providerRef == "" {
			providerRef = payload["refId"]
		}
	default:
		app.badRequestResponse(w, r, fmt.Errorf("unsupported provider: %s", provider))
		return
	}
	if providerRef == "" {
		app.badRequestResponse(w, r, fmt.Errorf("missing provider identifier (pidx/transaction_uuid/refId)"))
		return
	}

	// 4) Verify with gateway (do not trust webhook data)
	ver, err := app.payments.VerifyPayment(ctx, provider, payments.PaymentVerifyRequest{
		TransactionID: providerRef, // adapter should interpret this as provider_ref
		Data:          payload,
	})
	if err != nil {
		app.logger.Errorw("verify payment failed", "provider", provider, "ref", providerRef, "err", err)
		// Return 5xx so gateway retries (typical webhook behavior)
		http.Error(w, "verification error", http.StatusInternalServerError)
		return
	}

	// 5) Map provider_ref -> internal payment
	pay, err := app.store.Sales.Payments.GetByProviderRef(ctx, provider, providerRef)
	if err != nil {
		app.logger.Errorw("get payment by provider_ref failed", "provider", provider, "ref", providerRef, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Unknown providerRef: ACK 200 to prevent retries.
	if pay == nil {
		app.logger.Warnw("webhook for unknown provider_ref", "provider", provider, "ref", providerRef)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
		return
	}

	// Optional: persist raw webhook payload (best-effort, never block success)
	_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, pay.ID, "webhook", payload)

	// 6) If verification says NOT paid:
	// Depending on provider semantics, "not successful" could mean:
	// - user cancelled
	// - pending (some providers are async)
	// - failed
	//
	// If your provider guarantees webhook is only sent for terminal states, you can mark failed here.
	// Otherwise, safest is: ACK and do nothing.
	if !ver.Success {
		app.logger.Infow("webhook verification not successful", "provider", provider, "ref", providerRef, "payment_id", pay.ID)

		// Optionally mark failed + unlock cart (only if you are sure it's terminal failure):
		// _ = app.store.WithSalesTx(ctx, func(s *storage.SalesTx) error {
		//   p, _ := s.Payments.GetByID(ctx, pay.ID)
		//   if p == nil || strings.EqualFold(p.Status, "paid") { return nil }
		//   _ = s.Payments.SetStatus(ctx, pay.ID, "failed")
		//   _ = s.Orders.UpdateStatus(ctx, p.OrderID, "payment_failed", ordersrepo.UpdateStatusOpts{})
		//   _ = s.Carts.UnlockCheckoutCart(ctx, p.OrderID)
		//   return nil
		// })

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
		return
	}

	// 7) Apply PAID transition atomically:
	// MarkPaid updates payments + order, ConvertCheckoutCart finalizes cart only for the order linked checkout.
	if err := app.store.WithSalesTx(ctx, func(s *storage.SalesTx) error {
		// Re-check inside TX for true idempotency (webhooks can be duplicated / concurrent)
		p, err := s.Payments.GetByID(ctx, pay.ID)
		if err != nil {
			return err
		}
		if p == nil {
			return fmt.Errorf("payment not found")
		}
		if strings.EqualFold(p.Status, "paid") {
			return nil
		}

		// Mark payment + order as paid/processing
		if err := s.Payments.MarkPaid(ctx, pay.ID); err != nil {
			return err
		}

		// Convert cart locked for this order (no-op if none)
		if err := s.Carts.ConvertCheckoutCart(ctx, p.OrderID); err != nil {
			return err
		}

		return nil
	}); err != nil {
		app.logger.Errorw("paid transition failed", "payment_id", pay.ID, "provider", provider, "ref", providerRef, "err", err)
		http.Error(w, "failed to update payment", http.StatusInternalServerError)
		return
	}

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
		PaymentMethod *string `json:"payment_method"`
	}
	if err := readJSON(w, r, &in); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	method := "cash_on_delivery"
	if in.PaymentMethod != nil && strings.TrimSpace(*in.PaymentMethod) != "" {
		method = *in.PaymentMethod
	}

	ship := orders.ShippingInfo{
		Name: in.Shipping.Name, Phone: in.Shipping.Phone, Address: in.Shipping.Address,
		City: in.Shipping.City, PostalCode: in.Shipping.PostalCode, Country: in.Shipping.Country,
	}

	var (
		order   *orders.Order
		payment *paymentsrepo.Payment
	)

	// A) Transaction: create order snapshot + create payment row (if online) + lock cart (if online)
	err := app.store.WithSalesTx(ctx, func(s *storage.SalesTx) error {
		var err error
		order, _, err = s.Orders.CreateFromCart(ctx, userID, ship, method)
		if err != nil {
			return err
		}

		if method != "cash_on_delivery" {
			payment, err = s.Payments.Create(ctx, &paymentsrepo.Payment{
				OrderID:     order.ID,
				Provider:    method,
				AmountCents: order.TotalCents,
				Currency:    "NPR",
				Status:      "pending",
			})
			if err != nil {
				return err
			}

			// Keep a direct pointer from order -> primary_payment_id for quick access
			if err := s.Payments.SetPrimaryToOrder(ctx, order.ID, payment.ID); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// B) Outside tx: initiate gateway (only for online)
	var paymentURL string
	var paymentData map[string]string

	if payment != nil {
		resp, gerr := app.payments.InitiatePayment(ctx, method, payments.PaymentRequest{
			Amount:        float64(order.TotalCents) / 100.0, // NOTE: order snapshot amount
			TransactionID: fmt.Sprintf("%d", payment.ID),
			ProductName:   order.OrderNumber,
			CustomerName:  ship.Name,
			CustomerPhone: ship.Phone,
		})

		_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, payment.ID, "response", resp.Data)

		if gerr != nil {
			_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, payment.ID, "error", map[string]any{
				"stage": "initiate",
				"error": gerr.Error(),
			})

			// IMPORTANT: unlock cart so user can retry checkout immediately.
			// Also mark payment failed for support visibility.
			_ = app.store.Sales.Payments.SetStatus(ctx, payment.ID, "failed")
			_ = app.store.Sales.Carts.UnlockCheckoutCart(ctx, order.ID) // implement: set status='active', checkout_order_id=NULL TODO: check again

			app.internalServerError(w, r, fmt.Errorf("payment init: %w", gerr))
			return
		}

		paymentURL = resp.PaymentURL
		paymentData = map[string]string{}
		for k, v := range resp.Data {
			paymentData[k] = fmt.Sprint(v)
		}

		switch method {
		case "khalti":
			if ref, ok := paymentData["pidx"]; ok && ref != "" {
				_ = app.store.Sales.Payments.SetProviderRef(ctx, payment.ID, ref, resp.Data)
			}
		case "esewa":
			if ref, ok := paymentData["transaction_uuid"]; ok && ref != "" {
				_ = app.store.Sales.Payments.SetProviderRef(ctx, payment.ID, ref, resp.Data)
			}
		}
	}

	app.jsonResponse(w, http.StatusCreated, map[string]any{
		"order_id":     order.ID,
		"order_number": order.OrderNumber,
		"payment_id": func() any {
			if payment == nil {
				return nil
			}
			return payment.ID
		}(),
		"payment_url":  paymentURL,
		"payment_data": paymentData,
	})
}

// POST /v1/store/payments/verify
//
// Client-driven payment verification endpoint.
// Used after the user returns from a gateway (WebView redirect / app callback).
//
// Design goals (industry-standard):
//  1. Never trust client payload alone → always re-verify with gateway (lookup/status-check).
//  2. Do network calls OUTSIDE DB transactions → avoid holding locks while waiting on gateway.
//  3. Make DB transitions ATOMIC + IDEMPOTENT:
//     - If verified paid => (TX) MarkPaid + ConvertCheckoutCart
//     - If terminal failure => (TX) SetStatus=failed (+ optional order status) + UnlockCheckoutCart
//     - If non-terminal (pending/ambiguous) => keep payment pending, keep cart locked
//
// Assumptions:
//   - carts has status: active | checkout_pending | converted | abandoned
//   - carts has checkout_order_id pointing to the order created at checkout
//   - Carts.ConvertCheckoutCart(orderID) converts ONLY carts where status=checkout_pending AND checkout_order_id=orderID
//   - Carts.UnlockCheckoutCart(orderID) unlocks cart back to active for retry
//   - payments.VerifyPayment normalizes gateway statuses into PaymentVerifyResponse:
//     Success=true only when completed/captured
//     Terminal=true only when final (expired/canceled/not_found/etc)
//     Terminal=false for pending/ambiguous
func (app *application) verifyPaymentHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var in struct {
		PaymentID int64             `json:"payment_id"`
		Method    string            `json:"method"` // "khalti" | "esewa" | ...
		Data      map[string]string `json:"data"`   // gateway-specific fields (pidx, transaction_uuid, etc.)
	}
	if err := readJSON(w, r, &in); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}

	in.Method = strings.TrimSpace(in.Method)
	if in.PaymentID <= 0 || in.Method == "" {
		app.badRequestResponse(w, r, fmt.Errorf("payment_id and method are required"))
		return
	}
	if in.Data == nil {
		in.Data = map[string]string{}
	}

	// 1) Load payment (fast validation / early idempotency)
	pay, err := app.store.Sales.Payments.GetByID(ctx, in.PaymentID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if pay == nil {
		app.notFoundResponse(w, r, fmt.Errorf("payment not found"))
		return
	}

	// 2) Enforce provider consistency (prevents verifying with wrong adapter)
	if !strings.EqualFold(pay.Provider, in.Method) {
		app.badRequestResponse(w, r, fmt.Errorf("payment method mismatch: expected %s", pay.Provider))
		return
	}

	// 3) Early idempotency: if already paid, short-circuit.
	// NOTE: We also re-check inside the transaction to handle races (duplicate callbacks/webhooks).
	if strings.EqualFold(pay.Status, "paid") {
		app.jsonResponse(w, http.StatusOK, map[string]any{
			"success":    true,
			"idempotent": true,
		})
		return
	}

	// 4) Persist raw inbound callback data for support/debug (best-effort).
	_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, in.PaymentID, "request", in.Data)

	// Fill missing provider-specific fields using stored provider_ref (most reliable).
	switch strings.ToLower(in.Method) {
	case "khalti":
		if in.Data["pidx"] == "" && pay.ProviderRef != nil {
			in.Data["pidx"] = *pay.ProviderRef
		}
	case "esewa":
		if in.Data["transaction_uuid"] == "" && pay.ProviderRef != nil {
			in.Data["transaction_uuid"] = *pay.ProviderRef
		}
		// eSewa also needs total_amount and product_code for status check
		if in.Data["total_amount"] == "" {
			in.Data["total_amount"] = fmt.Sprintf("%.2f", float64(pay.AmountCents)/100.0)
		}
		// ✅ REQUIRED for eSewa verify
		if in.Data["product_code"] == "" {
			in.Data["product_code"] = app.config.payment.Esewa.MerchantID
		}
	}

	txid := ""
	if pay.ProviderRef != nil {
		txid = *pay.ProviderRef
	}

	// 5) Verify with gateway OUTSIDE tx (do not hold DB locks during network calls).
	//
	// TransactionID strategy:
	// - For Khalti you typically verify using pidx in in.Data["pidx"] (adapter handles it).
	// - For eSewa you verify using transaction_uuid in in.Data["transaction_uuid"] (adapter handles it).
	// - We still pass TransactionID as a fallback.
	ver, verify_err := app.payments.VerifyPayment(ctx, in.Method, payments.PaymentVerifyRequest{
		TransactionID: txid,
		Data:          in.Data,
	})

	// If gateway verification API fails (timeout/500/bad payload), do NOT guess success.
	// We keep things safe: mark failed (or keep pending if you prefer) and unlock cart so user can retry.
	//
	// NOTE: If you want "safer but less annoying" UX, you can choose to NOT mark failed on verify errors
	// and instead keep pending + show "We are confirming your payment. Try again." That's a product choice.
	if verify_err != nil {
		_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, in.PaymentID, "error", map[string]any{
			"stage":  "verify",
			"method": in.Method,
			"error":  verify_err.Error(),
			"data":   in.Data,
		})

		_ = app.store.WithSalesTx(ctx, func(s *storage.SalesTx) error {
			// Re-read payment inside tx for concurrency safety.
			p, err := s.Payments.GetByID(ctx, in.PaymentID)
			if err != nil {
				return err
			}
			if p == nil {
				return fmt.Errorf("payment not found")
			}
			if strings.EqualFold(p.Status, "paid") {
				// Another process already completed it. Don't override.
				return nil
			}

			// Mark failed for visibility (support can see "failed" instead of "pending forever").
			// If you prefer to keep pending on verify errors, change this to: no-op.
			_ = s.Payments.SetStatus(ctx, in.PaymentID, "pending")

			// Optional: If you added order_status=payment_failed, you can mirror it here.
			// This is optional; many systems keep order.status as "awaiting_payment" and rely on payment.status only.
			_ = s.Orders.UpdateStatus(ctx, p.OrderID, "payment_failed", orders.UpdateStatusOpts{})

			// Unlock cart so user can attempt checkout again.
			_ = s.Carts.UnlockCheckoutCart(ctx, p.OrderID)

			return nil
		})

		app.badRequestResponse(w, r, verify_err)
		return
	}

	// 6) Apply final state transition ATOMICALLY (single tx).
	// This prevents:
	// - payment marked paid but cart not converted
	// - cart converted without payment actually being paid
	err = app.store.WithSalesTx(ctx, func(s *storage.SalesTx) error {
		// True idempotency under concurrency: re-check inside tx.
		p, err := s.Payments.GetByID(ctx, in.PaymentID)
		if err != nil {
			return err
		}
		if p == nil {
			return fmt.Errorf("payment not found")
		}

		// If already paid, no-op.
		if strings.EqualFold(p.Status, "paid") {
			return nil
		}

		if ver.Success {
			// ✅ Paid transition:
			// MarkPaid should update:
			// - payments.status = paid
			// - orders.payment_status = paid
			// - orders.status = processing
			// - orders.paid_at = now()
			if err := s.Payments.MarkPaid(ctx, in.PaymentID); err != nil {
				return err
			}

			// Convert cart ONLY if it is the one locked for this order.
			// ConvertCheckoutCart should be strict:
			//   UPDATE carts SET status='converted'
			//   WHERE status='checkout_pending' AND checkout_order_id=$orderID;
			// If 0 rows affected, that’s OK (already converted or no cart).
			if err := s.Carts.ConvertCheckoutCart(ctx, p.OrderID); err != nil {
				return err
			}

			return nil
		}

		// Not successful. Decide using Terminal:
		//
		// - Terminal=false => gateway says "pending/ambiguous": keep pending + keep cart locked.
		// - Terminal=true  => gateway says "expired/canceled/not_found/etc": fail + unlock cart.
		if !ver.Terminal {
			// ⏳ Still pending / unknown: keep payment pending, do not unlock cart.
			// This is important for eSewa PENDING/AMBIGUOUS and Khalti Pending/Initiated.
			return nil
		}

		// ❌ Terminal failure: mark failed + unlock cart for retry.
		_ = s.Payments.SetStatus(ctx, in.PaymentID, "failed")
		_ = s.Orders.UpdateStatus(ctx, p.OrderID, "payment_failed", orders.UpdateStatusOpts{})
		_ = s.Carts.UnlockCheckoutCart(ctx, p.OrderID)

		return nil
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 7) Respond with normalized state so the app can show correct UI (success/pending/failure).
	app.jsonResponse(w, http.StatusOK, map[string]any{
		"success":  ver.Success,
		"terminal": ver.Terminal,
		"state":    ver.State,
	})
}

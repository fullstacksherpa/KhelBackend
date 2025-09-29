package main

import (
	"fmt"
	"khel/internal/payments"
	"net/http"
)

func (app *application) CheckoutHandler(w http.ResponseWriter, r *http.Request) {
	var payload payments.PaymentRequest
	if err := readJSON(w, r, &payload); err != nil {
		app.badRequestResponse(w, r, err)
		return
	}
	method := r.URL.Query().Get("method")
	if method == "" {
		app.badRequestResponse(w, r, fmt.Errorf("missing method query param"))
		return
	}

	// 1) persist order as PENDING (idempotent -- fail if already exists)
	// if err := app.store.Orders.CreatePendingOrder(r.Context(), payload.TransactionID, payload.ProductName, payload.Amount, payload.CustomerName, payload.CustomerEmail, payload.CustomerPhone, method); err != nil {
	// 	app.internalServerError(w, r, fmt.Errorf("failed to create pending order: %w", err))
	// 	return
	// }

	// 2) initiate payment with gateway adapter
	resp, err := app.payments.InitiatePayment(r.Context(), method, payload)
	if err != nil {
		app.internalServerError(w, r, fmt.Errorf("failed to initiate payment: %w", err))
		return
	}

	// 3) if gateway returns pidx (Khalti), persist it on the order for later lookup
	// if pidx, ok := resp.Data["pidx"]; ok && pidx != "" {
	// 	if err := app.store.Orders.UpdateOrderPidx(r.Context(), payload.TransactionID, pidx); err != nil {
	// 		// log but still return response (you might retry update asynchronously)
	// 		app.logger.Errorf("failed to save pidx for order %s: %v", payload.TransactionID, err)
	// 	}
	// }

	app.jsonResponse(w, http.StatusOK, resp)
}

func (app *application) PaymentWebhookHandler(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Determine provider: prefer query param `provider`, else in payload
	provider := r.URL.Query().Get("provider")
	var payload map[string]string

	if r.Method == http.MethodGet {
		// read key values from query string into payload map
		payload = make(map[string]string)
		for k, v := range r.URL.Query() {
			if len(v) > 0 {
				payload[k] = v[0]
			}
		}
	} else {
		// POST/other — parse body (some gateways send JSON, some form-encoded)
		if err := readJSON(w, r, &payload); err != nil {
			// fallback: try parse form values
			if err2 := r.ParseForm(); err2 == nil {
				payload = map[string]string{}
				for k, vals := range r.PostForm {
					if len(vals) > 0 {
						payload[k] = vals[0]
					}
				}
			} else {
				app.badRequestResponse(w, r, fmt.Errorf("invalid webhook payload: %v / %v", err, err2))
				return
			}
		}
		// try provider header or payload
		if provider == "" {
			if p, ok := payload["provider"]; ok {
				provider = p
			}
		}
	}

	if provider == "" {
		app.badRequestResponse(w, r, fmt.Errorf("missing provider"))
		return
	}

	// Extract gateway-specific identifier: Khalti uses pidx; eSewa uses transaction_uuid / refId
	var id string
	switch provider {
	case "khalti":
		id = payload["pidx"]
		if id == "" {
			// sometimes redirect uses 'pidx' or 'txnId'
			id = payload["txnId"]
		}
	case "esewa":
		id = payload["transaction_uuid"]
		if id == "" {
			id = payload["refId"]
		}
	default:
		app.badRequestResponse(w, r, fmt.Errorf("unsupported provider: %s", provider))
		return
	}

	if id == "" {
		app.badRequestResponse(w, r, fmt.Errorf("missing provider identifier (pidx/transaction_uuid)"))
		return
	}

	// Idempotency: has this pidx already been processed?
	// processed, err := app.store.Payments.PaymentLogExists(ctx, id, provider)
	// if err != nil {
	// 	app.logger.Errorw("payment log check failed", "id", id, "provider", provider, "err", err)
	// 	http.Error(w, "internal error", http.StatusInternalServerError)
	// 	return
	// }
	// if processed {
	// 	// already processed, just acknowledge
	// 	w.WriteHeader(http.StatusOK)
	// 	w.Write([]byte("OK"))
	// 	return
	// }

	// Call gateway verification (lookup) to be 100% sure
	verifyReq := payments.PaymentVerifyRequest{
		TransactionID: id,
		Data:          payload,
	}
	verifyResp, err := app.payments.VerifyPayment(ctx, provider, verifyReq)
	if err != nil {
		app.logger.Errorw("verify payment failed", "provider", provider, "id", id, "err", err)
		// return 5xx so provider may retry (network/temporary error)
		http.Error(w, "verification error", http.StatusInternalServerError)
		return
	}
	if verifyResp.Success {
		fmt.Print("delete later writing just not used error")
	}
	// // Save raw webhook payload and verification result (audit)
	// if err := app.store.Payments.InsertPaymentLog(ctx, id, provider, payload, verifyResp.Success); err != nil {
	// 	app.logger.Errorw("failed to insert payment log", "id", id, "err", err)
	// 	// continue — do not stop processing due to logging failure
	// }

	// // Update order status based on verification
	// if verifyResp.Success {
	// 	if err := app.store.Orders.MarkPaidByProviderID(ctx, provider, id, /* optional: transaction id from verifyResp */); err != nil {
	// 		app.logger.Errorw("MarkPaid failed", "id", id, "err", err)
	// 		http.Error(w, "failed to update order", http.StatusInternalServerError)
	// 		return
	// 	}
	// } else {
	// 	_ = app.store.Orders.MarkFailedByProviderID(ctx, provider, id)
	// }

	// Acknowledge provider with 200 OK
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

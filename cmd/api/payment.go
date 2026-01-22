package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"khel/internal/domain/storage"
	"khel/internal/payments"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"
)

// redirectToAppReturn serves an HTML page that:
// 1) tries to open your app via deep link: khel://payments/return?...params...
// 2) falls back to a web URL if the app is not installed
//
// Why HTML instead of 302 redirect directly?
// - iOS Safari / SFSafariViewController can be inconsistent with 302 -> custom scheme.
// - HTML + JS is more reliable and also provides a button for manual open.
//
// IMPORTANT: Call this and then `return` from your handler.
func (app *application) redirectToAppReturn(
	w http.ResponseWriter,
	result string, // "success" | "failed" | "pending"
	orderID, paymentID int64,
	provider string, // "esewa" | "khalti"
	providerRef string, // transaction_uuid | pidx (optional)
	gatewayState string, // COMPLETE/PENDING/... (optional)
	reason string, // optional internal reason for debugging
) {
	result = strings.ToLower(strings.TrimSpace(result))
	if result != "success" && result != "failed" && result != "pending" {
		result = "pending"
	}

	// All data goes in query params so the app can route + optionally verify again.
	q := url.Values{}
	q.Set("result", result) // 👈 IMPORTANT: this is how app knows which screen to show

	if orderID > 0 {
		q.Set("order_id", fmt.Sprintf("%d", orderID))
	}
	if paymentID > 0 {
		q.Set("payment_id", fmt.Sprintf("%d", paymentID))
	}
	if provider != "" {
		q.Set("provider", provider)
	}
	if providerRef != "" {
		q.Set("ref", providerRef)
	}
	if gatewayState != "" {
		q.Set("gateway_state", gatewayState)
	}
	if reason != "" {
		q.Set("reason", reason)
	}

	// ✅ MUST match what you pass in openAuthSessionAsync(..., redirectUrl)
	// In app: Linking.createURL("payments/return", { scheme: "khel" })
	deepLink := fmt.Sprintf("khel://payments/return?%s", q.Encode())

	// Optional web fallback (if app not installed)
	webFallback := fmt.Sprintf("%s/payments/return?%s", app.config.frontendURL, q.Encode())

	html := fmt.Sprintf(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Returning to app…</title>
    <style>
      body { font-family: system-ui, -apple-system, Segoe UI, Roboto; padding: 24px; }
      .btn { display: inline-block; padding: 12px 16px; border-radius: 10px; background:#111; color:#fff; text-decoration:none; }
      .muted { opacity: 0.7; margin-top: 12px; }
    </style>
  </head>
  <body>
    <h3>Returning to Khel…</h3>
    <p class="muted">If you are not redirected automatically, tap the button below.</p>
    <p><a class="btn" href="%s">Open in app</a></p>
    <p class="muted">Or continue on the web:</p>
    <p><a href="%s">%s</a></p>

    <script>
      // Try deep link immediately
      window.location.href = %q;

      // If the app isn't installed, redirect to web after a short delay
      setTimeout(function() {
        window.location.href = %q;
      }, 1200);
    </script>
  </body>
</html>`,
		deepLink,
		webFallback,
		webFallback,
		deepLink,
		webFallback,
	)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")

	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(html))
}

// GET /v1/store/payments/esewa/start?payment_id=123
func (app *application) esewaStartHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	paymentID, err := strconv.ParseInt(r.URL.Query().Get("payment_id"), 10, 64)
	if err != nil || paymentID <= 0 {
		app.badRequestResponse(w, r, fmt.Errorf("invalid payment_id"))
		return
	}

	pay, err := app.store.Sales.Payments.GetByID(ctx, paymentID)
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}
	if pay == nil {
		app.notFoundResponse(w, r, fmt.Errorf("payment not found"))
		return
	}
	if !strings.EqualFold(pay.Provider, "esewa") {
		app.badRequestResponse(w, r, fmt.Errorf("payment provider mismatch"))
		return
	}

	ref := ""
	if pay.ProviderRef != nil {
		ref = *pay.ProviderRef
	}

	// If already paid, send user back into the app immediately.
	if strings.EqualFold(pay.Status, "paid") {
		app.redirectToAppReturn(w,
			"success",
			pay.OrderID,
			paymentID,
			"esewa",
			ref,
			"COMPLETE",
			"already_paid",
		)
		return
	}

	// Initiate again (fresh transaction_uuid each attempt so eSewa doesn't complain)
	resp, gerr := app.payments.InitiatePayment(ctx, "esewa", payments.PaymentRequest{
		Amount:        float64(pay.AmountCents) / 100.0,
		TransactionID: fmt.Sprintf("%d", pay.ID), // adapter will make this unique (or you can do it here)
		ProductName:   fmt.Sprintf("Order #%d", pay.OrderID),
	})
	if gerr != nil {
		app.internalServerError(w, r, fmt.Errorf("esewa initiate: %w", gerr))
		return
	}

	// Make sure success/failure URLs include payment_id so the app can verify.
	resp.Data["success_url"] = addQuery(app.config.payment.Esewa.SuccessURL, "payment_id", fmt.Sprintf("%d", paymentID))
	resp.Data["failure_url"] = addQuery(app.config.payment.Esewa.FailureURL, "payment_id", fmt.Sprintf("%d", paymentID))

	// Save provider_ref = transaction_uuid + gateway_response=formFields
	txUUID := resp.Data["transaction_uuid"]
	if txUUID != "" {
		_ = app.store.Sales.Payments.SetProviderRef(ctx, paymentID, txUUID, resp.Data)
	}

	// Optional: logs
	_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, paymentID, "request", map[string]any{
		"stage":       "initiate",
		"payment_url": resp.PaymentURL,
		"fields":      resp.Data,
	})

	// Render auto-post form
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, max-age=0")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.WriteHeader(http.StatusOK)
	_ = renderAutoPostForm(w, resp.PaymentURL, resp.Data)
}

func addQuery(base, key, val string) string {
	u, err := url.Parse(base)
	if err != nil {
		// fallback
		if strings.Contains(base, "?") {
			return base + "&" + url.QueryEscape(key) + "=" + url.QueryEscape(val)
		}
		return base + "?" + url.QueryEscape(key) + "=" + url.QueryEscape(val)
	}
	q := u.Query()
	q.Set(key, val)
	u.RawQuery = q.Encode()
	return u.String()
}

func renderAutoPostForm(w http.ResponseWriter, action string, fields map[string]string) error {
	const tpl = `<!doctype html>
<html>
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>Redirecting…</title>
  <style>
    body { font-family: -apple-system, system-ui, Segoe UI, Roboto, Arial; padding: 24px; }
    .box { max-width: 480px; margin: 40px auto; text-align: center; }
  </style>
</head>
<body>
  <div class="box">
    <h3>Redirecting to eSewa…</h3>
    <p>Please wait.</p>

    <form id="f" method="POST" action="{{.Action}}">
      {{range $k, $v := .Fields}}
        <input type="hidden" name="{{$k}}" value="{{$v}}">
      {{end}}
      <noscript><button type="submit">Continue</button></noscript>
    </form>

    <script>
      (function(){ document.getElementById('f').submit(); })();
    </script>
  </div>
</body>
</html>`
	t := template.Must(template.New("p").Parse(tpl))
	return t.Execute(w, map[string]any{
		"Action": action,
		"Fields": fields,
	})
}

type esewaReturnPayload struct {
	TransactionCode  string `json:"transaction_code"`
	Status           string `json:"status"` // COMPLETE, PENDING, CANCELED, ...
	TotalAmount      any    `json:"total_amount"`
	TransactionUUID  string `json:"transaction_uuid"`
	ProductCode      string `json:"product_code"`
	SignedFieldNames string `json:"signed_field_names"`
	Signature        string `json:"signature"`
}

func normalizeEsewaAmount(v any) (string, error) {
	// eSewa may send number or string. We normalize to "100.00" style.
	switch t := v.(type) {
	case float64:
		return fmt.Sprintf("%.2f", t), nil
	case string:
		// could be "1000.0" -> keep numeric formatting stable
		f, err := strconv.ParseFloat(t, 64)
		if err != nil {
			return "", fmt.Errorf("invalid total_amount %q", t)
		}
		return fmt.Sprintf("%.2f", f), nil
	default:
		return "", fmt.Errorf("unsupported total_amount type %T", v)
	}
}

// eSewa response signature is generated the SAME way as request signature.
// The doc for request says: total_amount=...,transaction_uuid=...,product_code=...
// For response, they say verify integrity using same approach.
// We'll use those 3 fields because they are stable across flows.
func (app *application) verifyEsewaResponseSignature(totalAmount, transactionUUID, productCode, gotSig string) bool {
	raw := fmt.Sprintf("total_amount=%s,transaction_uuid=%s,product_code=%s", totalAmount, transactionUUID, productCode)

	mac := hmac.New(sha256.New, []byte(app.config.payment.Esewa.SecretKey))
	_, _ = mac.Write([]byte(raw))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	// Use constant-time compare
	return hmac.Equal([]byte(want), []byte(gotSig))
}

// eSewa redirects you here:
// /v1/store/payments/esewa/return?result=success|failure&data=<base64_json>
//
// This is the *canonical* completion endpoint for eSewa.
// eSewa redirects the user's browser/webview here after payment.
// We do NOT trust the redirect payload alone; we:
// 1) decode base64 payload
// 2) verify signature (integrity check)
// 3) call eSewa status-check API (source of truth)
// 4) update DB atomically: MarkPaid+ConvertCart OR MarkFailed+UnlockCart
// 5) redirect back into the mobile app (deep link) for UX
// NOTE: This endpoint is opened in a browser/webview.
// Always return HTML + deep link (not JSON), otherwise user sees raw JSON error.
func (app *application) esewaReturnHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	result := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("result")))
	dataB64 := strings.TrimSpace(r.URL.Query().Get("data"))
	if dataB64 == "" {
		app.redirectToAppReturn(w, "failed", 0, 0, "esewa", "", "", "missing_data")
		return
	}

	// 1) Decode base64 -> JSON payload
	rawJSON, err := base64.StdEncoding.DecodeString(dataB64)
	if err != nil {
		app.redirectToAppReturn(w, "failed", 0, 0, "esewa", "", "", "decoding base64 error")
		return
	}

	var p esewaReturnPayload
	if err := json.Unmarshal(rawJSON, &p); err != nil {
		app.redirectToAppReturn(w, "failed", 0, 0, "esewa", "", "", "invalid esewa payload")
		return
	}
	// 2) Validate minimum required fields (do NOT trust redirect blindly)
	p.TransactionUUID = strings.TrimSpace(p.TransactionUUID)
	p.ProductCode = strings.TrimSpace(p.ProductCode)
	p.Signature = strings.TrimSpace(p.Signature)

	if p.TransactionUUID == "" || p.ProductCode == "" || p.Signature == "" {
		app.redirectToAppReturn(w, "failed", 0, 0, "esewa", "", "", "missing required field")
		return
	}

	totalAmount, err := normalizeEsewaAmount(p.TotalAmount)
	if err != nil {
		app.redirectToAppReturn(w, "failed", 0, 0, "esewa", p.TransactionUUID, "", "invalid_total_amount")
		return
	}

	// 3) Signature check = integrity check (prevents tampering).
	// NOTE: Even if signature fails, we still do a status-check API call.
	// We just tag it as suspicious for logs and response.
	sigOK := app.verifyEsewaResponseSignature(totalAmount, p.TransactionUUID, p.ProductCode, p.Signature)

	// 4) Map eSewa transaction_uuid -> our internal payment using provider_ref.
	pay, err := app.store.Sales.Payments.GetByProviderRef(ctx, "esewa", p.TransactionUUID)
	if err != nil {
		app.logger.Errorw("db lookup failed", "provider", "esewa", "ref", p.TransactionUUID, "err", err.Error())
		app.redirectToAppReturn(w, "pending", 0, 0, "esewa", p.TransactionUUID, "", "db_lookup_failed")
		return
	}

	if pay == nil {
		app.redirectToAppReturn(w, "failed", 0, 0, "esewa", p.TransactionUUID, "", "payment_not_found")
		return
	}

	// Log the redirect payload for support/debug (best-effort).
	_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, pay.ID, "redirect", map[string]any{
		"result":  result,
		"sig_ok":  sigOK,
		"payload": p,
	})

	// 5) Source-of-truth: call eSewa transaction status API.
	ver, verifyErr := app.payments.VerifyPayment(ctx, "esewa", payments.PaymentVerifyRequest{
		TransactionID: p.TransactionUUID,
		Data: map[string]string{
			"product_code":     p.ProductCode,
			"transaction_uuid": p.TransactionUUID,
			"total_amount":     totalAmount,
		},
	})

	if verifyErr != nil {
		_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, pay.ID, "error", map[string]any{
			"stage": "esewa_status_check",
			"error": verifyErr.Error(),
		})

		reason := "status_check_failed"
		if !sigOK {
			reason = "bad_signature_and_status_check_failed"
		}

		app.redirectToAppReturn(w, "pending", pay.OrderID,
			pay.ID,
			"esewa",
			p.TransactionUUID, ver.State, reason)
		return

	}

	// 6) Apply DB changes atomically (idempotent).
	if ver.Success {
		err = app.store.WithSalesTx(ctx, func(s *storage.SalesTx) error {
			cur, err := s.Payments.GetByID(ctx, pay.ID)
			if err != nil {
				return err
			}
			if cur == nil {
				return fmt.Errorf("payment not found")
			}
			if strings.EqualFold(cur.Status, "paid") {
				return nil // idempotent
			}

			// Mark paid + convert checkout cart -> prevents "paid but cart not converted" bug.
			if err := s.Payments.MarkPaid(ctx, pay.ID); err != nil {
				return err
			}
			return s.Carts.ConvertCheckoutCart(ctx, cur.OrderID)
		})
		if err != nil {
			app.internalServerError(w, r, err)
			return
		}

		app.redirectToAppReturn(w, "success", pay.OrderID, pay.ID, "esewa", p.TransactionUUID, ver.State, "")
		return
	}

	// Not success: decide Pending vs Failed.
	// PENDING/AMBIGUOUS => keep pending + keep cart locked.
	finalState := strings.ToUpper(strings.TrimSpace(ver.State))

	err = app.store.WithSalesTx(ctx, func(s *storage.SalesTx) error {
		cur, err := s.Payments.GetByID(ctx, pay.ID)
		if err != nil {
			return err
		}
		if cur == nil {
			return fmt.Errorf("payment not found")
		}
		if strings.EqualFold(cur.Status, "paid") {
			return nil
		}

		switch finalState {
		case "PENDING", "AMBIGUOUS":
			// keep pending; do not unlock cart
			return nil
		default:
			// terminal fail/cancel/not_found/refund => fail + unlock for retry
			_ = s.Payments.SetStatus(ctx, pay.ID, "failed")
			_ = s.Carts.UnlockCheckoutCart(ctx, cur.OrderID)
			return nil
		}
	})
	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// Redirect app based on state.
	if finalState == "PENDING" || finalState == "AMBIGUOUS" {
		app.redirectToAppReturn(w, "pending", pay.OrderID, pay.ID, "esewa", p.TransactionUUID, ver.State, "")
		return

	}

	app.redirectToAppReturn(w, "failed", pay.OrderID, pay.ID, "esewa", p.TransactionUUID, ver.State, "gateway_terminal")

}

// GET /v1/store/payments/khalti
func (app *application) khaltiReturnHandler(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	q := r.URL.Query()

	pidx := strings.TrimSpace(q.Get("pidx"))

	if pidx == "" {
		app.redirectToAppReturn(w, "failed", 0, 0, "khalti", "", "", "missing_pidx")
		return
	}

	// 1) Load payment by provider_ref (pidx)
	payment, err := app.store.Sales.Payments.GetByProviderRef(ctx, "khalti", pidx)
	if err != nil {
		app.redirectToAppReturn(w, "pending", 0, 0, "khalti", pidx, "", "db_lookup_failed")
		return
	}
	if payment == nil {
		app.redirectToAppReturn(w, "failed", 0, 0, "khalti", pidx, "", "payment_not_found")
		return
	}

	// 2) Persist raw redirect payload (for debugging / support)
	_ = app.store.Sales.PayLogs.InsertPaymentLog(ctx, payment.ID, "redirect", q)

	// 3) Call your EXISTING verify logic (lookup API)
	ver, err := app.payments.VerifyPayment(ctx, "khalti", payments.PaymentVerifyRequest{
		TransactionID: pidx,
		Data: map[string]string{
			"pidx": pidx,
		},
	})

	if err != nil {
		app.redirectToAppReturn(w, "pending", payment.OrderID,
			payment.ID,
			"khalti",
			pidx,
			ver.State, "")
		// safest UX: treat as pending
		return
	}

	// 4) Apply DB transitions (reuse same logic as POST /verify)
	err = app.store.WithSalesTx(ctx, func(s *storage.SalesTx) error {
		p, err := s.Payments.GetByID(ctx, payment.ID)
		if err != nil || p == nil {
			return err
		}
		if p.Status == "paid" {
			return nil // idempotent
		}

		if ver.Success {
			if err := s.Payments.MarkPaid(ctx, p.ID); err != nil {
				return err
			}
			return s.Carts.ConvertCheckoutCart(ctx, p.OrderID)
		}

		if ver.Terminal {
			_ = s.Payments.SetStatus(ctx, p.ID, "failed")
			_ = s.Carts.UnlockCheckoutCart(ctx, p.OrderID)
		}

		return nil
	})

	if err != nil {
		app.internalServerError(w, r, err)
		return
	}

	// 5) Redirect user back to app
	switch {
	case ver.Success:
		app.redirectToAppReturn(w, "success", payment.OrderID, payment.ID, "khalti", pidx, ver.State, "")
		return

	case ver.Terminal:
		app.redirectToAppReturn(w, "failed", payment.OrderID, payment.ID, "khalti", pidx, ver.State, "")
		return

	default:
		app.redirectToAppReturn(w, "pending", payment.OrderID, payment.ID, "khalti", pidx, ver.State, "")
		return

	}
}

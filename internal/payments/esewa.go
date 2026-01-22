package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type EsewaAdapter struct {
	MerchantCode string
	SecretKey    string
	SuccessURL   string
	FailureURL   string
	IsProduction bool
	httpClient   *http.Client
}

func NewEsewaAdapter(merchant, secret, success, failure string, isProd bool) *EsewaAdapter {
	return &EsewaAdapter{
		MerchantCode: merchant,
		SecretKey:    secret,
		SuccessURL:   success,
		FailureURL:   failure,
		IsProduction: isProd,
		httpClient:   http.DefaultClient,
	}
}

func (e *EsewaAdapter) formURL() string {
	// user-facing payment page
	if e.IsProduction {
		return "https://epay.esewa.com.np/api/epay/main/v2/form"
	}
	return "https://rc-epay.esewa.com.np/api/epay/main/v2/form"
}

func (e *EsewaAdapter) statusBaseURL() string {
	// server-to-server status check endpoint base
	if e.IsProduction {
		return "https://esewa.com.np"
	}
	return "https://rc.esewa.com.np"
}

func (e *EsewaAdapter) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	// If TransactionID is just payment.ID, it will duplicate on retries.
	// Make it unique per initiation attempt (digits + hyphen only).
	transactionUUID := req.TransactionID

	transactionUUID = fmt.Sprintf("%s-%d", transactionUUID, time.Now().Unix())
	total := fmt.Sprintf("%.2f", req.Amount)

	// If you don't use tax/service/delivery, eSewa still wants them as "0"
	amount := total
	taxAmount := "0"
	serviceCharge := "0"
	deliveryCharge := "0"

	// IMPORTANT: eSewa requires signed_field_names to be present
	signedFields := "total_amount,transaction_uuid,product_code"

	// Signature input must match docs EXACTLY and same order as signed_field_names
	raw := fmt.Sprintf(
		"total_amount=%s,transaction_uuid=%s,product_code=%s",
		total, transactionUUID, e.MerchantCode,
	)

	mac := hmac.New(sha256.New, []byte(e.SecretKey))
	_, _ = mac.Write([]byte(raw))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	formFields := map[string]string{
		// required fields
		"amount":                  amount,
		"tax_amount":              taxAmount,
		"total_amount":            total,
		"transaction_uuid":        transactionUUID,
		"product_code":            e.MerchantCode,
		"product_service_charge":  serviceCharge,
		"product_delivery_charge": deliveryCharge,
		"success_url":             e.SuccessURL,
		"failure_url":             e.FailureURL,
		"signed_field_names":      signedFields,
		"signature":               signature,
	}

	return PaymentResponse{
		PaymentURL: e.formURL(),
		Data:       formFields,
	}, nil
}

// VerifyPayment calls eSewa "Status Check" API (source of truth).
//
// eSewa flow is redirect-based; you should never trust redirect/webhook payload alone.
// After you receive a success/failure redirect, call this API using:
//   - product_code
//   - total_amount
//   - transaction_uuid
//
// Expected statuses include: COMPLETE, PENDING, AMBIGUOUS, NOT_FOUND, CANCELED,
// FULL_REFUND, PARTIAL_REFUND.
//
// Contract we return:
//   - Success=true  only when status == COMPLETE (paid)
//   - Success=false otherwise (pending/failed/canceled/refund/etc)
//   - State is the raw eSewa status string for caller decisions (unlock cart vs keep pending).
func (e *EsewaAdapter) VerifyPayment(ctx context.Context, req PaymentVerifyRequest) (PaymentVerifyResponse, error) {
	productCode := strings.TrimSpace(req.Data["product_code"])
	totalAmount := strings.TrimSpace(req.Data["total_amount"])
	transactionUUID := strings.TrimSpace(req.Data["transaction_uuid"])
	if transactionUUID == "" {
		transactionUUID = strings.TrimSpace(req.TransactionID) // treat TransactionID as transaction_uuid
	}

	if productCode == "" || totalAmount == "" || transactionUUID == "" {
		return PaymentVerifyResponse{}, fmt.Errorf("esewa verify requires product_code, total_amount, transaction_uuid")
	}

	base := e.statusBaseURL()

	u, _ := url.Parse(base + "/api/epay/transaction/status/")
	q := u.Query()
	q.Set("product_code", productCode)
	q.Set("total_amount", totalAmount)
	q.Set("transaction_uuid", transactionUUID)
	u.RawQuery = q.Encode()

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	resp, err := e.httpClient.Do(httpReq)
	if err != nil {
		return PaymentVerifyResponse{Success: false}, fmt.Errorf("esewa status check request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return PaymentVerifyResponse{Success: false}, fmt.Errorf("esewa http %d: %s", resp.StatusCode, string(body))
	}

	var out struct {
		ProductCode     string  `json:"product_code"`
		TransactionUUID string  `json:"transaction_uuid"`
		TotalAmount     float64 `json:"total_amount"`
		Status          string  `json:"status"` // COMPLETE, PENDING, AMBIGUOUS, NOT_FOUND, CANCELED...
		RefID           *string `json:"ref_id"`
		Code            *int    `json:"code,omitempty"`
		ErrorMessage    *string `json:"error_message,omitempty"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return PaymentVerifyResponse{Success: false}, fmt.Errorf("decode esewa response: %w (body=%s)", err, string(body))
	}

	state := strings.ToUpper(strings.TrimSpace(out.Status))
	success := state == "COMPLETE"

	terminal := false
	switch state {
	case "COMPLETE":
		terminal = true
	case "PENDING", "AMBIGUOUS":
		terminal = false
	case "NOT_FOUND", "CANCELED", "FULL_REFUND", "PARTIAL_REFUND":
		terminal = true
	default:
		// Unknown: safest treat as non-terminal (hold)
		terminal = false
	}

	ref := transactionUUID // provider_ref for esewa in your system should be transaction_uuid
	return PaymentVerifyResponse{
		Success:  success,
		State:    state,
		Terminal: terminal,
		Raw: map[string]any{
			"http_status":     resp.StatusCode,
			"transactionUUID": ref,
			"body":            json.RawMessage(body),
		},
	}, nil
}

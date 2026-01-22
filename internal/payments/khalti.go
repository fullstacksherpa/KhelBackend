package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

type KhaltiAdapter struct {
	SecretKey    string
	ReturnURL    string
	WebsiteURL   string
	IsProduction bool
	httpClient   *http.Client
}

func NewKhaltiAdapter(secret, returnURL, websiteURL string, isProd bool) *KhaltiAdapter {
	return &KhaltiAdapter{
		SecretKey:    secret,
		ReturnURL:    returnURL,
		WebsiteURL:   websiteURL,
		IsProduction: isProd,
		httpClient:   http.DefaultClient,
	}
}

func (k *KhaltiAdapter) initiateURL() string {
	if k.IsProduction {
		return "https://khalti.com/api/v2/epayment/initiate/"
	}
	return "https://dev.khalti.com/api/v2/epayment/initiate/"
}

func (k *KhaltiAdapter) lookupURL() string {
	if k.IsProduction {
		return "https://khalti.com/api/v2/epayment/lookup/"
	}
	return "https://dev.khalti.com/api/v2/epayment/lookup/"
}

func (k *KhaltiAdapter) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	// Khalti amount is in paisa (integer). Docs examples use "1000" (string),
	// but integer works fine in JSON.
	amountPaisa := int(req.Amount * 100)

	payload := map[string]any{
		"return_url":          k.ReturnURL,
		"website_url":         k.WebsiteURL,
		"amount":              amountPaisa,
		"purchase_order_id":   req.TransactionID,
		"purchase_order_name": req.ProductName,
		"customer_info": map[string]string{
			"name":  req.CustomerName,
			"email": req.CustomerEmail,
			"phone": req.CustomerPhone,
		},
	}

	body, _ := json.Marshal(payload)

	url := k.initiateURL()

	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "key "+k.SecretKey) // 👈 match docs
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(httpReq)
	if err != nil {
		return PaymentResponse{}, fmt.Errorf("khalti initiate request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		// return raw error for logging/support
		return PaymentResponse{}, fmt.Errorf("khalti initiate failed: http=%d body=%s", resp.StatusCode, string(raw))
	}

	var res struct {
		Pidx       string `json:"pidx"`
		PaymentURL string `json:"payment_url"`
		ExpiresAt  string `json:"expires_at"`
		ExpiresIn  int    `json:"expires_in"`
	}

	if err := json.Unmarshal(raw, &res); err != nil {
		return PaymentResponse{}, fmt.Errorf("khalti initiate decode: %w body=%s", err, string(raw))
	}

	return PaymentResponse{
		PaymentURL: res.PaymentURL,
		Data: map[string]string{
			"pidx":        res.Pidx,
			"payment_url": res.PaymentURL,
			"expires_at":  res.ExpiresAt,
		},
	}, nil
}

func (k *KhaltiAdapter) VerifyPayment(ctx context.Context, req PaymentVerifyRequest) (PaymentVerifyResponse, error) {
	pidx := strings.TrimSpace(req.Data["pidx"])
	if pidx == "" {
		pidx = strings.TrimSpace(req.TransactionID)
	}
	if pidx == "" {
		return PaymentVerifyResponse{Success: false}, fmt.Errorf("khalti verify requires pidx")
	}

	payload := map[string]string{"pidx": pidx}
	body, _ := json.Marshal(payload)

	url := k.lookupURL()
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "key "+k.SecretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := k.httpClient.Do(httpReq)
	if err != nil {
		return PaymentVerifyResponse{Success: false}, fmt.Errorf("khalti lookup request: %w", err)
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)

	// Khalti may return 400 for Expired/User canceled (docs table).
	// So: try to decode anyway. If decode fails, treat as real error.
	var res struct {
		Pidx        string `json:"pidx"`
		TotalAmount int    `json:"total_amount"`
		Status      string `json:"status"` // Completed, Pending, Initiated, Expired, User canceled, Refunded, Partially refunded
		Transaction any    `json:"transaction_id"`
		Fee         any    `json:"fee"`
		Refunded    any    `json:"refunded"`
	}

	if err := json.Unmarshal(raw, &res); err != nil {
		return PaymentVerifyResponse{Success: false}, fmt.Errorf("khalti lookup decode: http=%d err=%w body=%s", resp.StatusCode, err, string(raw))
	}

	state := strings.TrimSpace(res.Status)

	// Docs: ONLY Completed is success. :contentReference[oaicite:10]{index=10}
	success := strings.EqualFold(state, "Completed")

	terminal := false
	switch strings.ToLower(state) {
	case "completed", "refunded", "expired", "user canceled", "partially refunded":
		terminal = true
	case "pending", "initiated":
		terminal = false
	default:
		// safest: hold and re-check / manual review
		terminal = false
	}

	return PaymentVerifyResponse{
		Success:  success,
		State:    state,
		Terminal: terminal,
		// ProviderRef for Khalti should be pidx in your system
		ProviderRef: pidx,
		Raw: map[string]any{
			"http_status": resp.StatusCode,
			"body":        json.RawMessage(raw),
		},
	}, nil
}

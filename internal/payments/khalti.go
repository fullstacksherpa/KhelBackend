package payments

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type KhaltiAdapter struct {
	SecretKey string
	ReturnURL string
}

func NewKhaltiAdapter(secret, returnURL string) *KhaltiAdapter {
	return &KhaltiAdapter{SecretKey: secret, ReturnURL: returnURL}
}

func (k *KhaltiAdapter) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {

	//refer to this docs: https://docs.khalti.com/khalti-epayment/
	payload := map[string]interface{}{
		"return_url":          k.ReturnURL,
		"website_url":         k.ReturnURL,           // recommended real site
		"amount":              int(req.Amount * 100), // paisa
		"purchase_order_id":   req.TransactionID,
		"purchase_order_name": req.ProductName,
		"customer_info": map[string]string{
			"name":  req.CustomerName,
			"email": req.CustomerEmail,
			"phone": req.CustomerPhone,
		},
	}

	body, _ := json.Marshal(payload)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", "https://dev.khalti.com/api/v2/epayment/initiate/", bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "Key "+k.SecretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return PaymentResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errBody map[string]any
		json.NewDecoder(resp.Body).Decode(&errBody)
		return PaymentResponse{}, fmt.Errorf("khalti initiate failed: status=%d body=%v", resp.StatusCode, errBody)
	}

	var res struct {
		Pidx       string `json:"pidx"`
		PaymentURL string `json:"payment_url"`
		ExpiresAt  string `json:"expires_at"`
		ExpiresIn  int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return PaymentResponse{}, fmt.Errorf("invalid response from Khalti: %w", err)
	}

	data := map[string]string{
		"pidx":        res.Pidx,
		"payment_url": res.PaymentURL,
		"expires_at":  res.ExpiresAt,
	}

	return PaymentResponse{
		PaymentURL: res.PaymentURL,
		Data:       data,
	}, nil
}

func (k *KhaltiAdapter) VerifyPayment(ctx context.Context, req PaymentVerifyRequest) (PaymentVerifyResponse, error) {
	// prefer pidx in Data map, otherwise use TransactionID
	pidx := req.Data["pidx"]
	if pidx == "" {
		pidx = req.TransactionID
	}
	if pidx == "" {
		return PaymentVerifyResponse{Success: false}, fmt.Errorf("no pidx provided")
	}

	payload := map[string]string{"pidx": pidx}
	body, _ := json.Marshal(payload)
	httpReq, _ := http.NewRequestWithContext(ctx, "POST", "https://dev.khalti.com/api/v2/epayment/lookup/", bytes.NewBuffer(body))
	httpReq.Header.Set("Authorization", "Key "+k.SecretKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := http.Client{}
	resp, err := client.Do(httpReq)
	if err != nil {
		return PaymentVerifyResponse{Success: false}, err
	}
	defer resp.Body.Close()

	var res struct {
		Pidx          string `json:"pidx"`
		TotalAmount   int    `json:"total_amount"`
		Status        string `json:"status"` // Completed, Pending, Initiated, Expired, User canceled, Refunded
		TransactionID string `json:"transaction_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return PaymentVerifyResponse{Success: false}, fmt.Errorf("failed to decode khalti lookup: %w", err)
	}

	// Treat only "Completed" as success as per khalti docs
	success := res.Status == "Completed"
	// you can pass back transaction id or other metadata
	// include it in the response if you expand PaymentVerifyResponse
	return PaymentVerifyResponse{Success: success}, nil
}

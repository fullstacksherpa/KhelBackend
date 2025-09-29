package payments

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
)

type EsewaAdapter struct {
	MerchantCode string
	SecretKey    string
	SuccessURL   string
	FailureURL   string
}

func NewEsewaAdapter(merchant, secret, success, failure string) *EsewaAdapter {
	return &EsewaAdapter{
		MerchantCode: merchant,
		SecretKey:    secret,
		SuccessURL:   success,
		FailureURL:   failure,
	}
}

func (e *EsewaAdapter) InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error) {
	transactionUUID := req.TransactionID
	total := fmt.Sprintf("%.2f", req.Amount)

	// Generate signature. raw is formatted as per esewa docs.
	raw := fmt.Sprintf("total_amount=%s,transaction_uuid=%s,product_code=%s", total, transactionUUID, e.MerchantCode)
	mac := hmac.New(sha256.New, []byte(e.SecretKey))
	mac.Write([]byte(raw))
	signature := base64.StdEncoding.EncodeToString(mac.Sum(nil))

	formFields := map[string]string{
		"total_amount":     total,
		"transaction_uuid": transactionUUID,
		"product_code":     e.MerchantCode,
		"success_url":      e.SuccessURL,
		"failure_url":      e.FailureURL,
		"signature":        signature,
	}

	return PaymentResponse{
		PaymentURL: "https://rc-epay.esewa.com.np/api/epay/main/v2/form",
		Data:       formFields,
	}, nil
}

func (e *EsewaAdapter) VerifyPayment(ctx context.Context, req PaymentVerifyRequest) (PaymentVerifyResponse, error) {
	// eSewa verification logic (optional: call their verification API)
	status := req.Data["status"] == "success"
	return PaymentVerifyResponse{Success: status}, nil
}

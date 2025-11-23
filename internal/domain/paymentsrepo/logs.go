package paymentsrepo

import (
	"context"
	"time"
)

type PaymentLog struct {
	ID        int64     `json:"id"`
	PaymentID int64     `json:"payment_id"`
	LogType   string    `json:"log_type"` // request, response, webhook, error
	Payload   any       `json:"payload,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type LogsStore interface {
	InsertPaymentLog(ctx context.Context, paymentID int64, logType string, payload any) error
}

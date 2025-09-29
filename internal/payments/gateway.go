package payments

import "context"

// PaymentGateway defines a common interface for all payment providers
type PaymentGateway interface {
	InitiatePayment(ctx context.Context, req PaymentRequest) (PaymentResponse, error)
	VerifyPayment(ctx context.Context, req PaymentVerifyRequest) (PaymentVerifyResponse, error)
}

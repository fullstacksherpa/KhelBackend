package payments

import (
	"context"
	"fmt"
)

type PaymentManager struct {
	gateways map[string]PaymentGateway
}

func NewPaymentManager() *PaymentManager {
	return &PaymentManager{gateways: make(map[string]PaymentGateway)}
}

func (m *PaymentManager) RegisterGateway(name string, gateway PaymentGateway) {
	m.gateways[name] = gateway
}

func (m *PaymentManager) InitiatePayment(ctx context.Context, method string, req PaymentRequest) (PaymentResponse, error) {
	gateway, ok := m.gateways[method]
	if !ok {
		return PaymentResponse{}, fmt.Errorf("gateway not registered: %s", method)
	}
	return gateway.InitiatePayment(ctx, req)
}

func (m *PaymentManager) VerifyPayment(ctx context.Context, method string, req PaymentVerifyRequest) (PaymentVerifyResponse, error) {
	gateway, ok := m.gateways[method]
	if !ok {
		return PaymentVerifyResponse{}, fmt.Errorf("gateway not registered: %s", method)
	}
	return gateway.VerifyPayment(ctx, req)
}

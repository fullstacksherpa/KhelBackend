package payments

type PaymentRequest struct {
	TransactionID string
	Amount        float64
	ProductName   string
	CustomerName  string
	CustomerEmail string
	CustomerPhone string
}

type PaymentResponse struct {
	PaymentURL string
	Data       map[string]string // form fields for esewa
}

type PaymentVerifyRequest struct {
	TransactionID string
	Data          map[string]string
}

type PaymentVerifyResponse struct {
	Success bool
}

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

// for khalti Transactionid is also pidx as fallback and pidx is inside Data["pidx"]
type PaymentVerifyRequest struct {
	TransactionID string
	Data          map[string]string
}

// PaymentVerifyResponse normalizes verification results across gateways.
//
// Success must be true ONLY when the gateway confirms payment is fully completed/captured.
// Terminal indicates whether the state is final (won't change by waiting/retrying).
//
// Examples:
//
//   - Khalti: Completed => Success=true, Terminal=true
//     Pending/Initiated => Success=false, Terminal=false
//     Expired/User canceled => Success=false, Terminal=true
//
//   - eSewa: COMPLETE => Success=true, Terminal=true
//     PENDING/AMBIGUOUS => Success=false, Terminal=false
//     CANCELED/NOT_FOUND => Success=false, Terminal=true
type PaymentVerifyResponse struct {
	Success     bool           `json:"success"`
	Terminal    bool           `json:"terminal,omitempty"`
	State       string         `json:"state,omitempty"`        // gateway state for debugging/decisioning
	ProviderRef string         `json:"provider_ref,omitempty"` //used for khalti pidx
	Raw         map[string]any `json:"raw,omitempty"`          // optional, safe metadata (no secrets)
}

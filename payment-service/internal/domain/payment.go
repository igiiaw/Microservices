package domain

import "errors"

// Payment is the core domain entity of the Payment bounded context.
// It is completely independent of the Order Service – no shared models.
type Payment struct {
	ID            string `json:"id"`
	OrderID       string `json:"order_id"`
	TransactionID string `json:"transaction_id"`
	Amount        int64  `json:"amount"` // cents, int64 – never float64
	Status        string `json:"status"`
}

// Payment status constants
const (
	StatusAuthorized = "Authorized"
	StatusDeclined   = "Declined"

	// MaxPaymentAmount: payments above $1 000.00 (100 000 cents) are declined.
	MaxPaymentAmount = int64(100_000)
)

// Domain errors
var (
	ErrPaymentNotFound = errors.New("payment not found")
)

// NewPayment creates a Payment and applies the payment-limit business rule.
// Business rule: if amount > 100 000 cents → Declined.
func NewPayment(orderID string, amount int64) *Payment {
	status := StatusAuthorized
	if amount > MaxPaymentAmount {
		status = StatusDeclined
	}
	return &Payment{
		OrderID: orderID,
		Amount:  amount,
		Status:  status,
	}
}

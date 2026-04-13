package domain

import (
	"errors"
	"time"
)

// Order is the core domain entity – no HTTP, JSON, or framework imports allowed here.
type Order struct {
	ID         string    `json:"id"`
	CustomerID string    `json:"customer_id"`
	ItemName   string    `json:"item_name"`
	Amount     int64     `json:"amount"` // Amount in cents; int64 – never float64 for money
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

// Order status constants
const (
	StatusPending   = "Pending"
	StatusPaid      = "Paid"
	StatusFailed    = "Failed"
	StatusCancelled = "Cancelled"
)

// Domain errors
var (
	ErrOrderNotFound              = errors.New("order not found")
	ErrInvalidAmount              = errors.New("amount must be greater than 0")
	ErrCannotCancel               = errors.New("only pending orders can be cancelled")
	ErrAlreadyPaid                = errors.New("paid orders cannot be cancelled")
	ErrPaymentServiceUnavailable  = errors.New("payment service unavailable")
	ErrPaymentDeclined            = errors.New("payment declined")
)

// NewOrder is a domain factory – validates invariants before construction.
func NewOrder(customerID, itemName string, amount int64) (*Order, error) {
	if amount <= 0 {
		return nil, ErrInvalidAmount
	}
	return &Order{
		CustomerID: customerID,
		ItemName:   itemName,
		Amount:     amount,
		Status:     StatusPending,
		CreatedAt:  time.Now().UTC(),
	}, nil
}

// Cancel transitions the Order to Cancelled state.
// Business rule: only Pending orders can be cancelled.
func (o *Order) Cancel() error {
	if o.Status == StatusPaid {
		return ErrAlreadyPaid
	}
	if o.Status != StatusPending {
		return ErrCannotCancel
	}
	o.Status = StatusCancelled
	return nil
}

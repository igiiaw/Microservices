package domain

// PaymentRepository — how we talk to the DB for payments
type PaymentRepository interface {
	Save(payment *Payment) error
	FindByOrderID(orderID string) (*Payment, error)
	ListByStatus(status string) ([]*Payment, error)
}

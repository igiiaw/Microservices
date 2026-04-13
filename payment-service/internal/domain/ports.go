package domain

// PaymentRepository is the Port for Payment persistence.
type PaymentRepository interface {
	Save(payment *Payment) error
	FindByOrderID(orderID string) (*Payment, error)
}

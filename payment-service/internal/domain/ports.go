package domain

// PaymentRepository handles the storage and retrieval of payment data.
type PaymentRepository interface {
	Save(payment *Payment) error
	FindByOrderID(orderID string) (*Payment, error)
	ListByStatus(status string) ([]*Payment, error)
}

// EventPublisher broadcasts domain events, hiding the underlying message broker implementation.
type EventPublisher interface {
	PublishPaymentCompleted(event PaymentCompletedEvent) error
}

// PaymentCompletedEvent holds the data published when a payment succeeds.
type PaymentCompletedEvent struct {
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`
	CustomerEmail string `json:"customer_email"`
	Status        string `json:"status"`
}

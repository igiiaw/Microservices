package domain

// OrderRepository is the Port for persistence – the use case depends on this interface,
// not on any concrete DB implementation.
type OrderRepository interface {
	Save(order *Order) error
	FindByID(id string) (*Order, error)
	FindByCustomerID(customerID string) ([]*Order, error)
	Update(order *Order) error
}

// PaymentClient is the Port for outbound HTTP communication with the Payment Service.
// This keeps the use case free of any HTTP client details.
type PaymentClient interface {
	// AuthorizePayment sends a payment request. Returns the transaction ID on success.
	AuthorizePayment(orderID string, amount int64) (transactionID string, err error)
}

// IdempotencyRepository is the Port for storing idempotency keys (Bonus).
type IdempotencyRepository interface {
	FindOrderByKey(key string) (*Order, error)
	SaveKey(key string, orderID string) error
}

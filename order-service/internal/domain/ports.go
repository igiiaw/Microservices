package domain

// OrderRepository — how we talk to the DB, any adapter just needs to implement this
type OrderRepository interface {
	Save(order *Order) error
	FindByID(id string) (*Order, error)
	FindByCustomerID(customerID string) ([]*Order, error)
	Update(order *Order) error
}

// PaymentClient — how we talk to the Payment Service, REST or gRPC, doesn't matter
type PaymentClient interface {
	// returns transaction ID on success
	AuthorizePayment(orderID string, amount int64) (transactionID string, err error)
}

// IdempotencyRepository — prevents processing the same request twice
type IdempotencyRepository interface {
	FindOrderByKey(key string) (*Order, error)
	SaveKey(key string, orderID string) error
}

// OrderEventPublisher — use case calls this when an order status changes
type OrderEventPublisher interface {
	PublishStatusChanged(orderID, newStatus string)
}

// OrderEventSubscriber — for whoever wants to listen, like the gRPC streaming server.
// always call unsubscribe when done or the channel leaks
type OrderEventSubscriber interface {
	Subscribe(orderID string) (ch <-chan OrderStatusEvent, unsubscribe func())
}

// OrderStatusEvent is the small struct that gets passed around on status changes.
// lives in domain because both ports above reference it
type OrderStatusEvent struct {
	OrderID   string
	NewStatus string
}

package handler

import (
	"encoding/json"
	"fmt"
	"log"

	amqp "github.com/rabbitmq/amqp091-go"

	"notification-service/internal/idempotency"
)

// PaymentCompletedEvent mirrors the Payment Service event.
// Redefined here to keep services decoupled (bounded contexts).
type PaymentCompletedEvent struct {
	OrderID       string `json:"order_id"`
	Amount        int64  `json:"amount"`
	CustomerEmail string `json:"customer_email"`
	Status        string `json:"status"`
}

// NotificationHandler processes payment.completed events.
type NotificationHandler struct {
	store *idempotency.MemoryStore
}

func NewNotificationHandler(store *idempotency.MemoryStore) *NotificationHandler {
	return &NotificationHandler{store: store}
}

// Handle processes a message. The caller handles errors (e.g., Nack or DLQ).
func (h *NotificationHandler) Handle(d amqp.Delivery) error {
	// 1. Idempotency check: Use MessageId, fallback to DeliveryTag if missing.
	msgID := d.MessageId
	if msgID == "" {
		msgID = fmt.Sprintf("tag-%d", d.DeliveryTag)
	}

	if h.store.IsDuplicate(msgID) {
		log.Printf("[Notification] Duplicate message %s, skipping", msgID)
		return nil // Already processed, safe to ignore
	}

	// 2. Parse payload.
	var event PaymentCompletedEvent
	if err := json.Unmarshal(d.Body, &event); err != nil {
		return fmt.Errorf("invalid message body: %w", err)
	}

	// 3. Simulate sending an email.
	amountDollars := float64(event.Amount) / 100.0
	log.Printf("[Notification] Sent email to %s for Order #%s. Amount: $%.2f. Status: %s",
		event.CustomerEmail,
		event.OrderID,
		amountDollars,
		event.Status,
	)

	// 4. Record as done BEFORE acknowledging to survive crashes safely.
	h.store.MarkProcessed(msgID)

	return nil
}

package rabbitmq

import (
	"encoding/json"
	"log"

	"github.com/google/uuid"
	amqp "github.com/rabbitmq/amqp091-go"

	"payment-service/internal/domain"
)

const (
	ExchangeName = "payment.events"
	RoutingKey   = "payment.completed"
)

// Publisher implements domain.EventPublisher via RabbitMQ.
type Publisher struct {
	ch *amqp.Channel
}

// NewPublisher creates a publisher and sets up a durable exchange.
func NewPublisher(ch *amqp.Channel) (*Publisher, error) {
	err := ch.ExchangeDeclare(
		ExchangeName, // name
		"direct",     // type
		true,         // durable (survives restarts)
		false,        // auto-deleted
		false,        // internal
		false,        // no-wait
		nil,          // args
	)
	if err != nil {
		return nil, err
	}

	return &Publisher{ch: ch}, nil
}

// PublishPaymentCompleted sends a persistent JSON event to RabbitMQ.
func (p *Publisher) PublishPaymentCompleted(event domain.PaymentCompletedEvent) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	err = p.ch.Publish(
		ExchangeName,
		RoutingKey,
		false,
		false,
		amqp.Publishing{
			ContentType:  "application/json",
			DeliveryMode: amqp.Persistent,
			MessageId:    uuid.New().String(),
			Body:         body,
		},
	)
	if err != nil {
		return err
	}

	log.Printf("Published payment.completed event for order %s", event.OrderID)
	return nil
}

// Close safely closes the channel.
func (p *Publisher) Close() {
	if p.ch != nil {
		p.ch.Close()
	}
}

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"notification-service/internal/handler"
	"notification-service/internal/idempotency"
)

const (
	exchangeName = "payment.events"
	queueName    = "notification.payment.completed"
	routingKey   = "payment.completed"
	maxRetries   = 3
)

func main() {
	// --- RabbitMQ Connection ---
	rabbitURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	var conn *amqp.Connection
	var err error
	for i := 1; i <= 12; i++ {
		conn, err = amqp.Dial(rabbitURL)
		if err == nil {
			break
		}
		log.Printf("[%d/12] waiting for RabbitMQ...", i)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("RabbitMQ not reachable: %v", err)
	}
	defer conn.Close()
	log.Println("Connected to RabbitMQ")

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("failed to open channel: %v", err)
	}
	defer ch.Close()

	// --- Exchange ---
	// Idempotent declaration; safe to call even if it already exists.
	err = ch.ExchangeDeclare(
		exchangeName,
		"direct",
		true,  // durable
		false, // auto-deleted
		false, // internal
		false, // no-wait
		nil,
	)
	if err != nil {
		log.Fatalf("failed to declare exchange: %v", err)
	}

	// --- Dead Letter Exchange & Queue (DLX/DLQ) ---
	err = ch.ExchangeDeclare(
		exchangeName+".dlx",
		"direct",
		true, false, false, false, nil,
	)
	if err != nil {
		log.Fatalf("failed to declare DLX: %v", err)
	}

	// Messages that fail maxRetries end up here.
	_, err = ch.QueueDeclare(
		queueName+".dlq",
		true, false, false, false, nil,
	)
	if err != nil {
		log.Fatalf("failed to declare DLQ: %v", err)
	}
	err = ch.QueueBind(queueName+".dlq", routingKey, exchangeName+".dlx", false, nil)
	if err != nil {
		log.Fatalf("failed to bind DLQ: %v", err)
	}

	// --- Main Queue ---
	// Durable with DLX routing. Nacked messages (without requeue) go to DLX.
	_, err = ch.QueueDeclare(
		queueName,
		true,  // durable
		false, // auto-delete
		false, // exclusive
		false, // no-wait
		amqp.Table{
			"x-dead-letter-exchange":    exchangeName + ".dlx",
			"x-dead-letter-routing-key": routingKey,
		},
	)
	if err != nil {
		log.Fatalf("failed to declare queue: %v", err)
	}

	err = ch.QueueBind(queueName, routingKey, exchangeName, false, nil)
	if err != nil {
		log.Fatalf("failed to bind queue: %v", err)
	}

	// Process one message at a time (prefetch 1). Increase in prod for throughput.
	err = ch.Qos(1, 0, false)
	if err != nil {
		log.Fatalf("failed to set QoS: %v", err)
	}

	// --- Consumer ---
	msgs, err := ch.Consume(
		queueName,
		"",    // auto-generated tag
		false, // auto-ack: false (manual ACK required)
		false, // exclusive
		false, // no-local
		false, // no-wait
		nil,
	)
	if err != nil {
		log.Fatalf("failed to start consuming: %v", err)
	}

	store := idempotency.NewMemoryStore()
	notifHandler := handler.NewNotificationHandler(store)

	// --- Graceful Shutdown Setup ---
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	log.Println("Notification Service listening for payment.completed events...")

	// --- Message Loop ---
	for {
		select {
		case sig := <-quit:
			log.Printf("Received %s, shutting down...", sig)
			// Stop receiving new messages, but finish the current one.
			if err := ch.Cancel("", false); err != nil {
				log.Printf("failed to cancel consumer: %v", err)
			}
			log.Println("Notification Service shutdown complete")
			return

		case d, ok := <-msgs:
			if !ok {
				log.Println("RabbitMQ channel closed, exiting")
				return
			}

			// Build a stable message ID for idempotency and retry tracking.
			msgID := d.MessageId
			if msgID == "" {
				msgID = fmt.Sprintf("tag-%d", d.DeliveryTag)
			}

			err := notifHandler.Handle(d)

			if err == nil {
				d.Ack(false)
				continue
			}

			// Track retries in memory. Works because Nack(requeue: true)
			// redelivers to the same consumer on the same connection.
			attempt := store.IncrementAttempts(msgID)
			if attempt >= maxRetries {
				log.Printf("Message %s failed %d times, sending to DLQ: %v",
					msgID, attempt, err)
				d.Nack(false, false) // requeue: false → routed to DLX → DLQ
			} else {
				log.Printf("Message %s failed (attempt %d/%d), requeueing: %v",
					msgID, attempt, maxRetries, err)
				d.Nack(false, true) // requeue: true → back to main queue
			}
		}
	}
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

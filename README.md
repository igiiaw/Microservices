# AP2 Assignments – Order, Payment & Notification Microservices

**Stack:** Go 1.24 · gRPC · Protocol Buffers · RabbitMQ · PostgreSQL · Docker Compose · Gin (external REST)

---

## What This Project Is

Three microservices — Order, Payment, and Notification — built across three assignments:

- **Assignment 1**: REST-based Order and Payment services with Clean Architecture
- **Assignment 2**: Migrated internal communication from REST to gRPC, added server-side streaming and a logging interceptor
- **Assignment 3**: Added event-driven architecture with RabbitMQ. The Payment Service publishes events after every payment, and the new Notification Service consumes them. Includes manual ACKs, idempotency, graceful shutdown, and a Dead Letter Queue.

The whole thing follows Clean Architecture. The domain and use case layers stayed unchanged through the gRPC migration — the gRPC adapter implements the same port interface as the old REST adapter, so the use cases can't tell the difference.

---

## Repo Structure (Contract-First)

| Repo | Purpose | Link |
|------|---------|------|
| **ap2-proto** | `.proto` files only — the source of truth | [github.com/igiiaw/ap2-proto](https://github.com/igiiaw/ap2-proto) |
| **ap2-proto-gen** | Auto-generated `.pb.go` stubs (never edited by hand) | [github.com/igiiaw/ap2-proto-gen](https://github.com/igiiaw/ap2-proto-gen) |
| **ap2-assignment1** | The actual services (this repo) | you're looking at it |

When I push a `.proto` change to `ap2-proto`, a GitHub Actions workflow runs `protoc`, generates Go code, and pushes it to `ap2-proto-gen`. Then I bump the version in my services with `go get github.com/igiiaw/ap2-proto-gen@v0.3.0`. No manual code generation needed.

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Docker Network                                 │
│                                                                             │
│   ┌──────────────────┐    gRPC     ┌────────────────┐    RabbitMQ           │
│   │  Order Service   │ ──────────► │ Payment Service│ ──────────────▼       │
│   │  REST :8080      │             │ HTTP :8081     │        ┌────────────┐ │
│   │  gRPC :9090      │             │ gRPC :9091     │        │Notification│ │
│   └────────┬─────────┘             └───────┬────────┘        │  Service   │ │
│            │ SQL                           │ SQL             └──────┬─────┘ │
│   ┌────────▼─────────┐             ┌───────▼────────┐               │       │
│   │    orders-db     │             │  payments-db   │         (no DB —      │
│   │ PostgreSQL :5433 │             │PostgreSQL :5434│         logs only)    │
│   └──────────────────┘             └────────────────┘                       │
│                                                                             │
│                          ┌──────────────┐                                   │
│                          │   RabbitMQ   │                                   │
│                          │  AMQP :5672  │                                   │
│                          │  UI  :15672  │                                   │
│                          └──────────────┘                                   │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## Clean Architecture Layers

```
cmd/service-name/main.go              ← Composition Root (wires everything together)
internal/
  domain/                             ← Entities + Errors + Port interfaces
  usecase/                            ← Business logic (depends ONLY on ports)
  repository/postgres/                ← DB adapter (implements domain port)
  client/  (order-service only)       ← gRPC adapter to Payment Service
  transport/http/                     ← REST handlers (Gin)
  transport/grpc/                     ← gRPC handlers + interceptor
  messaging/rabbitmq/ (payment-svc)   ← RabbitMQ publisher adapter
  events/  (order-service only)       ← In-memory pub/sub for streaming
```

Dependencies always point inward toward the domain. The domain package imports nothing external — no Gin, no gRPC, no database/sql, no RabbitMQ. That's the whole point.

---

## How to Run

### Prerequisites

- Docker Desktop (running)
- `grpcurl` (for testing gRPC endpoints): `brew install grpcurl` or `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`

### Start Everything

```bash
docker compose up --build
```

Services available at:
- Order Service REST → `http://localhost:8080`
- Order Service gRPC → `localhost:9090`
- Payment Service HTTP → `http://localhost:8081`
- Payment Service gRPC → `localhost:9091`
- RabbitMQ Management UI → `http://localhost:15672` (guest/guest)

### Clean Restart (wipe everything)

```bash
docker compose down -v
docker compose up --build
```

---

## Testing the APIs

### REST — Create an Order

```bash
curl -s -X POST localhost:8080/orders \
  -H 'content-type: application/json' \
  -d '{"customer_id":"c1","item_name":"book","amount":500}'
```

This goes through the full chain: REST → Order Service → gRPC → Payment Service → RabbitMQ → Notification Service.

### REST — Get / Cancel Orders

```bash
curl localhost:8080/orders/<order-id>
curl -X PATCH localhost:8080/orders/<order-id>/cancel
```

### gRPC — List Payments by Status

```bash
# All authorized payments
grpcurl -plaintext -d '{"status":"Authorized"}' \
  localhost:9091 payment.v1.PaymentService/ListPayments

# All declined payments
grpcurl -plaintext -d '{"status":"Declined"}' \
  localhost:9091 payment.v1.PaymentService/ListPayments

# All payments (no filter)
grpcurl -plaintext -d '{}' \
  localhost:9091 payment.v1.PaymentService/ListPayments
```

### gRPC — Stream Order Updates

**Terminal 1** — create an order:
```bash
curl -s -X POST localhost:8080/orders \
  -H 'content-type: application/json' \
  -d '{"customer_id":"demo","item_name":"book","amount":500}'
```

**Terminal 2** — subscribe to updates:
```bash
grpcurl -plaintext -d '{"order_id":"<paste-id>"}' \
  localhost:9090 order.v1.OrderService/SubscribeToOrderUpdates
```

### Notification Service — Verify Events

```bash
docker compose logs notification-service | tail -5
```

You should see:
```
[Notification] Sent email to customer_806a25a4@example.com for Order #806a25a4-... Amount: $5.00. Status: Authorized
```

---

## Event-Driven Architecture (Assignment 3)

### How It Works

The Payment Service publishes a `payment.completed` event to RabbitMQ after every successful payment. The Notification Service consumes these events and simulates sending an email by logging it. The two services are fully decoupled — the Notification Service does not use gRPC or REST to talk to the Payment Service.

```
Order Service ──gRPC──► Payment Service ──RabbitMQ──► Notification Service
                              │                              │
                        publishes to                   consumes from
                      payment.events               notification.payment.completed
                     (direct exchange)                  (durable queue)
```

### ACK Strategy

Auto-acknowledge is disabled (`auto-ack: false`). The consumer only ACKs a message after it has been fully processed. This guarantees at-least-once delivery: if the consumer crashes before ACKing, RabbitMQ redelivers the message.

The ordering matters:

1. Process the message (log the email)
2. Mark the message ID in the idempotency store
3. ACK the message

If the consumer crashes between step 2 and 3, the message is redelivered but caught by the idempotency check. If it crashes between step 1 and 2, the message is redelivered and the email is logged again — acceptable for notifications since a duplicate email is better than no email.

### Idempotency Strategy

The Notification Service uses an in-memory map to track which message IDs have already been processed. Before handling a message, it checks `IsDuplicate(messageID)`. If the message was already processed, it skips it and ACKs immediately.

This works because RabbitMQ's at-least-once delivery means the same message can arrive more than once (broker restart, consumer crash before ACK, network issues). Without idempotency, every redelivery would trigger a duplicate notification.

The in-memory store resets on restart, which is acceptable for a notification service because re-sending an email is harmless. For a service where duplicates are dangerous (like payment processing), the store would be backed by a database table with a unique constraint on the message ID.

### Dead Letter Queue (DLQ)

Messages that fail processing 3 times are rejected without requeue. Because the main queue is configured with `x-dead-letter-exchange`, RabbitMQ automatically routes rejected messages to the DLQ.

The DLQ holds failed messages for manual inspection. They are never retried automatically — a developer checks them via the RabbitMQ management UI at `http://localhost:15672`.

**Testing the DLQ:**
```bash
# Create a declined order (triggers simulated failure)
curl -s -X POST localhost:8080/orders \
  -H 'content-type: application/json' \
  -d '{"customer_id":"cVIP","item_name":"yacht","amount":150000}'

sleep 10
docker compose logs notification-service | tail -10

# Main queue empty, DLQ has the failed message
docker compose exec rabbitmq rabbitmqctl list_queues name messages
```

Expected:
```
notification.payment.completed       0
notification.payment.completed.dlq   1
```

### Graceful Shutdown

Both the Payment Service and Notification Service handle `SIGINT` and `SIGTERM` using `os/signal`. On shutdown:

- **Payment Service**: stops accepting new gRPC/HTTP requests, finishes in-flight ones, then closes the RabbitMQ connection and database.
- **Notification Service**: cancels the RabbitMQ consumer, finishes the current message, then closes the connection.

This prevents message loss during container stops or deployments.

### Queues and Exchanges

| Name | Type | Durable | Purpose |
|------|------|---------|---------|
| `payment.events` | Direct exchange | Yes | Routes payment events by routing key |
| `notification.payment.completed` | Queue | Yes | Main queue for notification consumer |
| `payment.events.dlx` | Direct exchange | Yes | Dead letter exchange |
| `notification.payment.completed.dlq` | Queue | Yes | Holds messages that failed 3 times |

---

## Logging Interceptor

The Payment Service has a unary gRPC interceptor that logs every RPC call with the method name, status code, and duration. Registered at the server level, covers all RPCs automatically.

```
gRPC /payment.v1.PaymentService/ProcessPayment | OK | 5.5ms
gRPC /payment.v1.PaymentService/ListPayments | OK | 3.9ms
gRPC /payment.v1.PaymentService/ListPayments | InvalidArgument | 15µs
```

```bash
docker compose logs payment-service | grep "gRPC /"
```

---

## Business Rules

| Rule | Where It Lives |
|------|---------------|
| Amount must be > 0 | `domain.NewOrder()` |
| Payments over $1,000 (100,000 cents) are declined | `domain.NewPayment()` |
| Only Pending orders can be cancelled | `order.Cancel()` domain method |
| Money is always `int64` in cents | Everywhere — never float64 |
| gRPC client timeout: 2 seconds | `context.WithTimeout` in the gRPC adapter |

All business rules live in the domain layer. Handlers only translate between transport types and domain types.

---

## What Changed Across Assignments

| What | Assignment 1 | Assignment 2 | Assignment 3 |
|------|-------------|-------------|-------------|
| Order → Payment communication | REST | gRPC | gRPC (unchanged) |
| Payment → Notification | N/A | N/A | RabbitMQ events |
| Notification Service | N/A | N/A | New consumer service |
| Message broker | N/A | N/A | RabbitMQ with DLQ |
| Streaming | N/A | `SubscribeToOrderUpdates` | Unchanged |
| Interceptor | N/A | Logging interceptor | Unchanged |
| Graceful shutdown | N/A | N/A | `os/signal` on Payment + Notification |
| Manual ACK + Idempotency | N/A | N/A | Notification Service |
| Domain layer | — | Unchanged | EventPublisher port added |
| Use case layer | — | Unchanged | Publishes event after payment |

---

## Environment Variables

| Variable | Service | Default | Description |
|----------|---------|---------|-------------|
| `DB_HOST` | Order, Payment | `localhost` | PostgreSQL host |
| `DB_PORT` | Order, Payment | `5432` | PostgreSQL port |
| `DB_USER` | Order, Payment | `postgres` | DB username |
| `DB_PASSWORD` | Order, Payment | `postgres` | DB password |
| `DB_NAME` | Order | `orders_db` | Database name |
| `DB_NAME` | Payment | `payments_db` | Database name |
| `PORT` | Order | `8080` | HTTP server port |
| `PORT` | Payment | `8081` | HTTP server port |
| `GRPC_PORT` | Order | `9090` | gRPC server port |
| `GRPC_PORT` | Payment | `9091` | gRPC server port |
| `PAYMENT_GRPC_ADDR` | Order | `localhost:9091` | Payment Service gRPC address |
| `RABBITMQ_URL` | Payment, Notification | `amqp://guest:guest@localhost:5672/` | RabbitMQ connection string |

---

## Idempotency (Bonus from Assignment 1)

Still works. Send the same request twice with the same `Idempotency-Key` header and you get the same order back — no duplicate payment:

```bash
curl -X POST localhost:8080/orders \
  -H 'content-type: application/json' \
  -H 'Idempotency-Key: my-key-123' \
  -d '{"customer_id":"c1","item_name":"book","amount":500}'

# Same key again → same order, no duplicate
curl -X POST localhost:8080/orders \
  -H 'content-type: application/json' \
  -H 'Idempotency-Key: my-key-123' \
  -d '{"customer_id":"c1","item_name":"book","amount":500}'
```

# AP2 Assignment 2 – gRPC Migration (Order & Payment Microservices)

**Stack:** Go 1.24 · gRPC · Protocol Buffers · PostgreSQL · Docker Compose · Gin (external REST)

---

## What This Project Is

The two microservices - “Order” and “Payment” - which were originally built using REST in Assignment 1, have now been migrated to gRPC for internal communication. The “Order” service still exposes a REST interface to the outside world (so clients like `curl` still work), but “under the hood” it communicates with the “Payment” service via gRPC. As part of the assignment, I also added server-side streaming and a logging interceptor.

All of this adheres to the principles of “Clean Architecture.” The key benefit of this transition: **the domain and use case layers required no changes**, since the gRPC adapter implements the same port interface as the old REST adapter. The use cases literally cannot tell them apart.

---

## Repo Structure (Contract-First)

This project uses three repositories:

| Repo | Purpose                                              | Link |
|------|------------------------------------------------------|------|
| **ap2-proto** | `.proto` files only - the source of truth            | [github.com/igiiaw/ap2-proto](https://github.com/igiiaw/ap2-proto) |
| **ap2-proto-gen** | Auto-generated `.pb.go` stubs (never edited by hand) | [github.com/igiiaw/ap2-proto-gen](https://github.com/igiiaw/ap2-proto-gen) |
| **ap2-assignment1** | The actual services (this repo)                      | you're looking at it |

When I push changes to the `.proto` file to the `ap2-proto` repository, the GitHub Actions workflow runs `protoc`, generates Go code, and pushes it to the `ap2-proto-gen` repository. I then update the version in my services using the command `go get github.com/igiiaw/ap2-proto-gen@v0.3.0`. Manual code generation is not required.

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────┐
│                          Docker Network                              │
│                                                                      │
│   ┌──────────────────────┐     gRPC (internal)    ┌────────────────┐ │
│   │    Order Service     │ ─────────────────────► │ Payment Service│ │
│   │  REST :8080          │                        │ HTTP :8081     │ │
│   │  gRPC :9090          │                        │ gRPC :9091     │ │
│   └──────────┬───────────┘                        └───────┬────────┘ │
│              │ SQL                                        │ SQL      │
│   ┌──────────▼───────────┐                        ┌───────▼────────┐ │
│   │     orders-db        │                        │   payments-db  │ │
│   │   PostgreSQL :5433   │                        │ PostgreSQL:5434│ │
│   └──────────────────────┘                        └────────────────┘ │
│                                                                      │
│                                                                      │
└──────────────────────────────────────────────────────────────────────┘
```

**What changed from Assignment 1:**
- Data exchange between the “Order” and “Payment” services has been migrated from REST to gRPC (gRPC)
- The “Order” service now has a gRPC server running on port `:9090` for streaming data
- The “Payment” service now has a gRPC server running on port `:9091` for the `ProcessPayment` and `ListPayments` methods
- Both services continue to use their HTTP servers to ensure backward compatibility

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
  events/  (order-service only)       ← In-memory pub/sub for streaming
```

Dependencies always point inward, toward the domain. The domain package does not import anything from outside-neither Gin, nor gRPC, nor database/sql. That is the whole point.

---

## gRPC Services

### PaymentService (port 9091)

| RPC | Type | Description |
|-----|------|-------------|
| `ProcessPayment` | Unary | Takes order_id + amount, returns payment with Authorized/Declined status |
| `ListPayments` | Unary | Returns payments filtered by status. Empty status = return all |

### OrderService (port 9090)

| RPC | Type | Description |
|-----|------|-------------|
| `SubscribeToOrderUpdates` | Server-side streaming | Subscribe by order_id, get real-time status updates from the DB |

The streaming RPC server generates a frame every time the order status actually changes in Postgres. As the first frame, it sends the current status (so that subscribers who connect later are not left in the dark), and then continues to operate until the order reaches its final state (“Paid,” “Failed,” or “Canceled”), after which the stream is closed.

---

## How to Run

### Prerequisites

- Docker Desktop (running)
- `grpcurl` (for testing gRPC endpoints): `brew install grpcurl` or `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`

### Start Everything

```bash
docker compose up --build
```

Services will be available at:
- Order Service REST → `http://localhost:8080`
- Order Service gRPC → `localhost:9090`
- Payment Service HTTP → `http://localhost:8081`
- Payment Service gRPC → `localhost:9091`

### Clean Restart (wipe DBs)

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

This process runs through the entire chain: a REST request to the order processing service → gRPC to the payment processing service → payment authorization → the order is marked as “Paid.”

### REST — Get / Cancel Orders

```bash
# Get an order
curl localhost:8080/orders/<order-id>

# Cancel a pending order
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

Open two terminals:

**Terminal 1** — create an order:
```bash
curl -s -X POST localhost:8080/orders \
  -H 'content-type: application/json' \
  -d '{"customer_id":"demo","item_name":"book","amount":500}'
```

**Terminal 2** — subscribe to updates (paste the order ID):
```bash
grpcurl -plaintext -d '{"order_id":"<paste-id>"}' \
  localhost:9090 order.v1.OrderService/SubscribeToOrderUpdates
```

As the order status changes, you will see the corresponding frames. If the order has already reached its final status, you will see a single frame, and the stream will close-this is normal behavior.

### gRPC — Validation

```bash
# Invalid status → InvalidArgument
grpcurl -plaintext -d '{"status":"banana"}' \
  localhost:9091 payment.v1.PaymentService/ListPayments

# Non-existent order → NotFound
grpcurl -plaintext -d '{"order_id":"does-not-exist"}' \
  localhost:9090 order.v1.OrderService/SubscribeToOrderUpdates
```

---

## Logging Interceptor

The payment service implements a single-parameter gRPC interceptor that logs every RPC call, recording the method name, status code, and duration. It is registered at the server level, so it automatically covers all current and future RPC calls-no code is required at the individual handler level.

Example output:
```
gRPC /payment.v1.PaymentService/ProcessPayment | OK | 5.5ms
gRPC /payment.v1.PaymentService/ListPayments | OK | 3.9ms
gRPC /payment.v1.PaymentService/ListPayments | InvalidArgument | 15µs
```

Check it with:
```bash
docker compose logs payment-service | grep "gRPC /"
```

---

## Business Rules

| Rule | Where It Lives                            |
|------|-------------------------------------------|
| Amount must be > 0 | `domain.NewOrder()`                       |
| Payments over $1,000 (100,000 cents) are declined | `domain.NewPayment()`                     |
| Only Pending orders can be cancelled | `order.Cancel()` domain method            |
| Money is always `int64` in cents | Everywhere - never float64                |
| gRPC client timeout: 2 seconds | `context.WithTimeout` in the gRPC adapter |

All business logic resides in the domain layer. gRPC handlers merely handle the conversion between Protobuf types and domain types-they do not make decisions.

---

## What Changed

| What | Assignment 1 | Assignment 2 |
|------|-------------|-------------|
| Order → Payment communication | REST (net/http) | gRPC (unary) |
| Payment Service delivery | HTTP only | HTTP + gRPC |
| Order Service delivery | HTTP only | HTTP + gRPC (streaming) |
| Streaming | N/A | `SubscribeToOrderUpdates` (server-side) |
| Proto contracts | N/A | Separate repo with CI generation |
| Interceptor | N/A | Logging interceptor on Payment Service |
| New RPC | N/A | `ListPayments` with status filter |
| Domain layer | — | **Unchanged** |
| Use case layer | — | **Unchanged** |

The key point lies in the last two lines. Replacing REST with gRPC was a change at the transport layer. The business logic remained intact; it wasn’t duplicated and didn’t require any changes. That’s exactly how “Clean Architecture” works.

---

## Environment Variables

| Variable | Service | Default | Description |
|----------|---------|---------|-------------|
| `DB_HOST` | Both | `localhost` | PostgreSQL host |
| `DB_PORT` | Both | `5432` | PostgreSQL port |
| `DB_USER` | Both | `postgres` | DB username |
| `DB_PASSWORD` | Both | `postgres` | DB password |
| `DB_NAME` | Order | `orders_db` | Database name |
| `DB_NAME` | Payment | `payments_db` | Database name |
| `PORT` | Order | `8080` | HTTP server port |
| `PORT` | Payment | `8081` | HTTP server port |
| `GRPC_PORT` | Order | `9090` | gRPC server port |
| `GRPC_PORT` | Payment | `9091` | gRPC server port |
| `PAYMENT_GRPC_ADDR` | Order | `localhost:9091` | Payment Service gRPC address |

---

## Idempotency

Everything works as expected. Send the same request twice with the same `Idempotency-Key` header, and you’ll receive the same order-you won’t be charged twice:

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
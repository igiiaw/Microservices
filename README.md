# AP2 Assignment 1 – Clean Architecture Microservices (Order & Payment)

**Author:** Taubakabyl Nurlybek  
**Tech Stack:** Go 1.22 · Gin · PostgreSQL · Docker Compose

---

## Table of Contents

1. [Architecture Overview](#architecture-overview)
2. [Bounded Contexts](#bounded-contexts)
3. [Clean Architecture Layers](#clean-architecture-layers)
4. [Dependency Flow Diagram](#dependency-flow-diagram)
5. [Service Interaction Diagram](#service-interaction-diagram)
6. [How to Run](#how-to-run)
7. [API Reference & Examples](#api-reference--examples)
8. [Business Rules](#business-rules)
9. [Failure Handling](#failure-handling)
10. [Bonus – Idempotency](#bonus--idempotency)
11. [Architecture Decisions & Trade-offs](#architecture-decisions--trade-offs)

---

## Architecture Overview

The system consists of **two independent microservices**, each owning its own database and following **Clean Architecture** (Ports & Adapters / Hexagonal) principles internally.

```
┌─────────────────────────────────────────────────────────────────┐
│                        Docker Network                           │
│                                                                 │
│   ┌──────────────────────┐    REST (2s timeout)                 │
│   │    Order Service     │ ──────────────────────►              │
│   │      :8080           │                        │             │
│   └──────────┬───────────┘          ┌─────────────▼──────────┐ │
│              │                      │   Payment Service      │ │
│              │ SQL                  │       :8081            │ │
│   ┌──────────▼───────────┐          └─────────────┬──────────┘ │
│   │     orders-db        │                        │ SQL        │
│   │   PostgreSQL :5433   │          ┌─────────────▼──────────┐ │
│   └──────────────────────┘          │     payments-db        │ │
│                                     │  PostgreSQL :5434      │ │
│                                     └────────────────────────┘ │
└─────────────────────────────────────────────────────────────────┘
```

---

## Bounded Contexts

| Concern                | Order Service              | Payment Service             |
|------------------------|----------------------------|-----------------------------|
| **Owns**               | Orders, their lifecycle    | Payments, transaction IDs   |
| **Database**           | `orders_db`                | `payments_db`               |
| **Domain model**       | `Order` entity             | `Payment` entity            |
| **Knows about**        | What was ordered & by whom | Amount limits & auth status |
| **Does NOT know about**| Payment transaction details| Order items or customers    |

There is **no shared package** between the services. Each service defines its own domain models independently — this is the correct microservices decomposition.

---

## Clean Architecture Layers

Each service follows the same layered structure:

```
cmd/service-name/main.go          ← Composition Root (manual DI wiring)
internal/
  domain/                         ← Entities + Errors + Port interfaces
  usecase/                        ← Business logic (depends only on Ports)
  repository/postgres/            ← Persistence adapter (implements Port)
  client/ (order-service only)    ← HTTP adapter to Payment Service
  transport/http/                 ← Gin handlers (thin delivery layer)
```

### Dependency Rule

Dependencies always point **inward** — toward the domain:

```
transport/http  →  usecase  →  domain
repository      →             domain
client          →             domain
```

The `domain` package has **zero external dependencies** (no Gin, no database/sql, no HTTP).

---

## Dependency Flow Diagram

```
┌─────────────────────────────────────────────────────────────┐
│                      Order Service                          │
│                                                             │
│  ┌────────────┐     ┌──────────────┐     ┌──────────────┐  │
│  │  transport │────►│   usecase    │────►│    domain    │  │
│  │  /http     │     │              │     │  (entities + │  │
│  │  (Handler) │     │ OrderUseCase │     │   ports)     │  │
│  └────────────┘     └──────┬───────┘     └──────────────┘  │
│                            │ uses Port interfaces           │
│               ┌────────────┼────────────┐                   │
│               ▼            ▼            ▼                   │
│  ┌─────────────────┐ ┌──────────┐ ┌──────────────────────┐  │
│  │ repository/     │ │ client/  │ │ repository/          │  │
│  │ postgres        │ │ payment  │ │ postgres             │  │
│  │ (OrderRepo)     │ │ Client   │ │ (IdempotencyRepo)    │  │
│  └────────┬────────┘ └────┬─────┘ └──────────┬───────────┘  │
│           │               │                  │              │
│           ▼               ▼                  ▼              │
│      PostgreSQL     Payment Service      PostgreSQL         │
│      (orders_db)    REST :8081          (orders_db)         │
└─────────────────────────────────────────────────────────────┘
```

---

## Service Interaction Diagram

```
Client                Order Service              Payment Service
  │                        │                           │
  │  POST /orders          │                           │
  │───────────────────────►│                           │
  │                        │  1. Validate amount > 0   │
  │                        │  2. Save Order (Pending)  │
  │                        │  3. POST /payments ───────►│
  │                        │     (2s timeout)          │  4. Apply limit rule
  │                        │                           │  5. Save Payment
  │                        │◄──────────────────────────│  6. Return status
  │                        │  7. Update Order status   │
  │◄───────────────────────│     (Paid / Failed)       │
  │  201 { order }         │                           │
```

### Payment Service Unavailable Scenario

```
Client                Order Service              Payment Service
  │                        │                     (DOWN / SLOW)
  │  POST /orders          │                           │
  │───────────────────────►│                           │
  │                        │  Save Order (Pending)     │
  │                        │  POST /payments ──────────X (timeout 2s)
  │                        │  Update Order → Failed    │
  │◄───────────────────────│                           │
  │  503 Service           │                           │
  │  Unavailable           │                           │
```

---

## How to Run

### Prerequisites

- Docker Desktop (running)
- Go 1.22+ (for local development in GoLand)

### Start All Services

```bash
# From the project root (ap2-assignment1/)
docker compose up --build
```

Services will be available at:
- Order Service → `http://localhost:8080`
- Payment Service → `http://localhost:8081`
- orders-db → `localhost:5433`
- payments-db → `localhost:5434`

### Stop Everything

```bash
docker compose down
```

### Wipe Databases and Restart Clean

```bash
docker compose down -v
docker compose up --build
```

### Local Development in GoLand

Start only the databases and payment service in Docker:

```bash
docker compose up orders-db payments-db payment-service
```

Then run order-service directly in GoLand. Set these environment variables in **Run/Debug Configurations**:

```
DB_HOST=localhost
DB_PORT=5433
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=orders_db
PAYMENT_SERVICE_URL=http://localhost:8081
PORT=8080
```

For payment-service local run:

```
DB_HOST=localhost
DB_PORT=5434
DB_USER=postgres
DB_PASSWORD=postgres
DB_NAME=payments_db
PORT=8081
```

---

## API Reference & Examples

### Order Service (`:8080`)

#### `POST /orders` — Create Order

```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{"customer_id": "cust-001", "item_name": "MacBook Pro", "amount": 50000}'
```

**Response 201 — Paid:**
```json
{
  "id": "a1b2c3d4-...",
  "customer_id": "cust-001",
  "item_name": "MacBook Pro",
  "amount": 50000,
  "status": "Paid",
  "created_at": "2026-04-01T10:00:00Z"
}
```

**Response 201 — Failed (amount > 100 000, payment declined):**
```json
{
  "id": "e5f6g7h8-...",
  "customer_id": "cust-001",
  "item_name": "Private Jet",
  "amount": 999999,
  "status": "Failed",
  "created_at": "2026-04-01T10:01:00Z"
}
```

**Response 400 — Invalid amount:**
```json
{ "error": "amount must be greater than 0" }
```

**Response 503 — Payment Service down:**
```json
{ "error": "payment service unavailable" }
```

---

#### `GET /orders/:id` — Get Order

```bash
curl http://localhost:8080/orders/a1b2c3d4-...
```

**Response 200:**
```json
{
  "id": "a1b2c3d4-...",
  "customer_id": "cust-001",
  "item_name": "MacBook Pro",
  "amount": 50000,
  "status": "Paid",
  "created_at": "2026-04-01T10:00:00Z"
}
```

**Response 404:**
```json
{ "error": "order not found" }
```

---

#### `PATCH /orders/:id/cancel` — Cancel Order

```bash
curl -X PATCH http://localhost:8080/orders/a1b2c3d4-.../cancel
```

**Response 200 — Cancelled (was Pending):**
```json
{
  "id": "a1b2c3d4-...",
  "status": "Cancelled",
  ...
}
```

**Response 409 — Cannot cancel a Paid order:**
```json
{ "error": "paid orders cannot be cancelled" }
```

---

### Payment Service (`:8081`)

#### `POST /payments` — Authorize Payment

```bash
curl -X POST http://localhost:8081/payments \
  -H "Content-Type: application/json" \
  -d '{"order_id": "a1b2c3d4-...", "amount": 50000}'
```

**Response 201 — Authorized:**
```json
{
  "id": "pay-uuid-...",
  "order_id": "a1b2c3d4-...",
  "transaction_id": "txn-uuid-...",
  "amount": 50000,
  "status": "Authorized"
}
```

**Response 201 — Declined (amount > 100 000):**
```json
{
  "id": "pay-uuid-...",
  "order_id": "a1b2c3d4-...",
  "transaction_id": "txn-uuid-...",
  "amount": 999999,
  "status": "Declined"
}
```

---

#### `GET /payments/:order_id` — Get Payment by Order

```bash
curl http://localhost:8081/payments/a1b2c3d4-...
```

**Response 200:**
```json
{
  "id": "pay-uuid-...",
  "order_id": "a1b2c3d4-...",
  "transaction_id": "txn-uuid-...",
  "amount": 50000,
  "status": "Authorized"
}
```

---

## Business Rules

| Rule | Where Enforced | Details |
|------|---------------|---------|
| `amount > 0` | `domain.NewOrder()` | Domain factory validates invariant |
| `Paid` orders cannot be cancelled | `order.Cancel()` | Domain method enforces state machine |
| Only `Pending` orders can be cancelled | `order.Cancel()` | Returns `ErrCannotCancel` for other states |
| Payment limit: `amount > 100 000` → Declined | `domain.NewPayment()` | Payment Service domain rule |
| Money is `int64` (cents) | All layers | Never `float64` — financial accuracy |
| HTTP timeout: 2 seconds | `http.Client{Timeout: 2s}` | Composition Root in `main.go` |

---

## Failure Handling

### Payment Service Unavailable

**Design Decision: Order is marked `Failed` (not left as `Pending`).**

**Reasoning:**
- `Pending` implies the order *may still succeed* and could be retried.
- Since the Order Service makes a **synchronous** call and never retries, there is no background process that will later transition the order.
- Leaving orders permanently `Pending` would cause data inconsistency — the customer placed an order with no outcome.
- `Failed` is an honest, deterministic outcome: *"We tried; no payment was authorised."*
- The client receives a **503 Service Unavailable** so they know the service was the problem, not their request.

### Payment Declined

- The Payment Service stores the declined payment record (audit trail).
- The Order Service transitions the order to `Failed`.
- The response to the client is **201 Created** with `status: "Failed"` — the order resource was created, the payment was not.

---

## Bonus – Idempotency

Send the same `POST /orders` request twice with the same `Idempotency-Key` header:

```bash
# First call — creates the order
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: my-unique-key-abc123" \
  -d '{"customer_id": "cust-001", "item_name": "Keyboard", "amount": 15000}'

# Second call — returns the SAME order, no duplicate created, no duplicate payment charged
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -H "Idempotency-Key: my-unique-key-abc123" \
  -d '{"customer_id": "cust-001", "item_name": "Keyboard", "amount": 15000}'
```

**Implementation:**
- `idempotency_keys` table stores `key → order_id` mappings.
- Checked in `OrderUseCase.CreateOrder()` before any side effects.
- Key is saved only **after** successful order creation (atomically correct — failed orders are never cached).
- The feature is optional (header omission skips all idempotency logic).

---

## Architecture Decisions & Trade-offs

### Why Manual DI instead of a framework?

The assignment requires demonstrating Dependency Inversion explicitly. Manual DI in `main.go` makes the wiring visible, auditable, and simple. A DI framework would hide this relationship.

### Why PostgreSQL and not an in-memory store?

The assignment mandates a **real database**. PostgreSQL ensures ACID guarantees for order state transitions and idempotency key storage.

### Why separate schemas/databases?

**Database-per-service** is a core microservices principle. Shared databases create hidden coupling — one service's schema change can break another. Separate databases enforce the bounded context boundary at the infrastructure level.

### Why `int64` (cents) for money?

IEEE 754 floating-point cannot represent all decimal fractions exactly. `0.1 + 0.2 ≠ 0.3` in float arithmetic. Financial systems always use integer arithmetic in the smallest currency unit (cents).

### Why does the Order Service return 503 on timeout but 201 on Declined?

- **503**: The service itself failed — the client's request was valid, but infrastructure prevented fulfilment. The client should retry (possibly later).
- **201 with `status: "Failed"`**: The request was processed successfully end-to-end; the *business outcome* was a declined payment. No retry will help without changing the amount.

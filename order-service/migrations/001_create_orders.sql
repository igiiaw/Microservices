-- migrations/001_create_orders.sql

CREATE TABLE IF NOT EXISTS orders (
    id          VARCHAR(36)  PRIMARY KEY,
    customer_id VARCHAR(255) NOT NULL,
    item_name   VARCHAR(255) NOT NULL,
    amount      BIGINT       NOT NULL CHECK (amount > 0),
    status      VARCHAR(50)  NOT NULL,
    created_at  TIMESTAMPTZ  NOT NULL
);

-- Bonus: idempotency table to prevent duplicate order creation
CREATE TABLE IF NOT EXISTS idempotency_keys (
    key        VARCHAR(255) PRIMARY KEY,
    order_id   VARCHAR(36)  NOT NULL REFERENCES orders(id),
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_orders_customer_id ON orders (customer_id);
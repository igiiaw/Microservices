-- migrations/001_create_payments.sql

CREATE TABLE IF NOT EXISTS payments (
    id             VARCHAR(36)  PRIMARY KEY,
    order_id       VARCHAR(36)  NOT NULL,
    transaction_id VARCHAR(36)  NOT NULL,
    amount         BIGINT       NOT NULL CHECK (amount > 0),
    status         VARCHAR(50)  NOT NULL         -- "Authorized" | "Declined"
);

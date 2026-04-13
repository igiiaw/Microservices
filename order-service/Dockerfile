# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Download dependencies first (layer-cache friendly)
COPY go.mod ./
RUN go mod download 2>/dev/null; true

COPY . .

# go mod tidy resolves any missing sums, then build a static binary
RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -o /order-service ./cmd/order-service

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/
COPY --from=builder /order-service .

EXPOSE 8080
CMD ["./order-service"]

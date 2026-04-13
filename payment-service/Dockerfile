# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download 2>/dev/null; true

COPY . .

RUN go mod tidy && \
    CGO_ENABLED=0 GOOS=linux go build -o /payment-service ./cmd/payment-service

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM alpine:3.19

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/
COPY --from=builder /payment-service .

EXPOSE 8081
CMD ["./payment-service"]

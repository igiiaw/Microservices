# Stage 1: build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# copy go.mod first so Docker caches the deps layer separately
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# static binary — no libc needed, works fine in alpine
RUN CGO_ENABLED=0 GOOS=linux go build -o /payment-service ./cmd/payment-service

# Stage 2: runtime — only the binary, keeps the image small
FROM alpine:3.19

# need these for HTTPS calls and timezone support
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/
COPY --from=builder /payment-service .

EXPOSE 8081
CMD ["./payment-service"]
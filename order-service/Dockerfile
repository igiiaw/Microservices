# Stage 1: build
FROM golang:1.24-alpine AS builder

WORKDIR /app

# copy go.mod first so Docker can cache the deps layer separately
COPY go.mod ./
RUN go mod download

COPY . .

# static binary — no libc needed, works fine in alpine
RUN CGO_ENABLED=0 GOOS=linux go build -o /order-service ./cmd/order-service

# Stage 2: runtime — only the binary, keeps the image small
FROM alpine:3.19

# need these for HTTPS calls and timezone support
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /root/
COPY --from=builder /order-service .

EXPOSE 8080
CMD ["./order-service"]
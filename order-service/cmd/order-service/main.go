package main

import (
	"database/sql"
	"fmt"
	"log"
	"net"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	orderv1 "github.com/igiiaw/ap2-proto-gen/gen/go/order/v1"
	"order-service/internal/client"
	"order-service/internal/events"
	pgRepo "order-service/internal/repository/postgres"
	transportGRPC "order-service/internal/transport/grpc"
	transportHTTP "order-service/internal/transport/http"
	"order-service/internal/usecase"
)

func main() {
	// ── Database ──────────────────────────────────────────────────────────────
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", "postgres"),
		getEnv("DB_NAME", "orders_db"),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	// Retry loop – give PostgreSQL time to start inside Docker.
	for i := 1; i <= 12; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		log.Printf("[%d/12] waiting for orders-db…", i)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("orders-db not reachable: %v", err)
	}
	log.Println("Connected to orders-db")

	// ── Migrations ────────────────────────────────────────────────────────────
	if err := runMigrations(db); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	// ── Composition Root (manual DI) ──────────────────────────────────────────
	// ── Payment Service gRPC client ───────────────────────────────────────────
	paymentGRPCAddr := getEnv("PAYMENT_GRPC_ADDR", "localhost:9091")

	// Long-lived connection — gRPC multiplexes all calls over it.
	// `insecure` is fine inside a trusted docker-compose network; use TLS in prod.
	paymentConn, err := grpc.NewClient(
		paymentGRPCAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("failed to dial payment service: %v", err)
	}
	defer paymentConn.Close()

	// Adapter implementing domain.PaymentClient — same port, gRPC transport.
	paymentClientAdapter := client.NewPaymentGRPCClient(paymentConn, 10*time.Second)
	orderRepo := pgRepo.NewOrderRepository(db)
	idempotencyRepo := pgRepo.NewIdempotencyRepository(db, orderRepo)

	// NEW: Event broker — single instance, shared by use case (publisher) and
	// gRPC server (subscriber).
	broker := events.NewOrderBroker()

	// NEW: Use case now receives the broker as the OrderEventPublisher port.
	orderUseCase := usecase.NewOrderUseCase(orderRepo, paymentClientAdapter, idempotencyRepo, broker)

	// Delivery layer
	orderHandler := transportHTTP.NewOrderHandler(orderUseCase)

	// NEW: Initialize the gRPC server delivery adapter
	orderGRPCServer := transportGRPC.NewOrderServer(orderUseCase, broker)

	// ── Servers Setup ─────────────────────────────────────────────────────────

	// NEW: gRPC server for SubscribeToOrderUpdates
	// Run this in a separate goroutine so it doesn't block the HTTP server below.
	grpcPort := getEnv("GRPC_PORT", "9090")
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("failed to listen on :%s: %v", grpcPort, err)
	}
	grpcServer := grpc.NewServer()
	orderv1.RegisterOrderServiceServer(grpcServer, orderGRPCServer)

	reflection.Register(grpcServer)

	go func() {
		log.Printf("Order Service gRPC listening on :%s", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("grpc server error: %v", err)
		}
	}()

	// ── Router ────────────────────────────────────────────────────────────────
	router := gin.Default()
	orderHandler.RegisterRoutes(router)

	port := getEnv("PORT", "8080")
	log.Printf("Order Service HTTP listening on :%s", port)

	// External REST stays as it was (blocking call).
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// runMigrations applies schema changes idempotently.
func runMigrations(db *sql.DB) error {
	_, err := db.Exec(`
       CREATE TABLE IF NOT EXISTS orders (
          id          VARCHAR(36)  PRIMARY KEY,
          customer_id VARCHAR(255) NOT NULL,
          item_name   VARCHAR(255) NOT NULL,
          amount      BIGINT       NOT NULL CHECK (amount > 0),
          status      VARCHAR(50)  NOT NULL,
          created_at  TIMESTAMPTZ  NOT NULL
       );

       CREATE TABLE IF NOT EXISTS idempotency_keys (
          key       VARCHAR(255) PRIMARY KEY,
          order_id  VARCHAR(36)  NOT NULL REFERENCES orders(id),
          created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
       );
    `)
	return err
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

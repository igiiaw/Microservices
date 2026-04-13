package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	"order-service/internal/client"
	pgRepo "order-service/internal/repository/postgres"
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
	// A single shared http.Client with a hard 2-second timeout as required.
	httpClient := &http.Client{Timeout: 2 * time.Second}
	paymentBaseURL := getEnv("PAYMENT_SERVICE_URL", "http://localhost:8081")

	// Adapters (implementations of Ports)
	paymentClientAdapter := client.NewPaymentClient(httpClient, paymentBaseURL)
	orderRepo := pgRepo.NewOrderRepository(db)
	idempotencyRepo := pgRepo.NewIdempotencyRepository(db, orderRepo)

	// Use case (depends on Port interfaces, not concrete types)
	orderUseCase := usecase.NewOrderUseCase(orderRepo, paymentClientAdapter, idempotencyRepo)

	// Delivery layer
	orderHandler := transportHTTP.NewOrderHandler(orderUseCase)

	// ── Router ────────────────────────────────────────────────────────────────
	router := gin.Default()
	orderHandler.RegisterRoutes(router)

	port := getEnv("PORT", "8080")
	log.Printf("Order Service listening on :%s", port)
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

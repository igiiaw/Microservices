package main

import (
	"database/sql"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"

	pgRepo "payment-service/internal/repository/postgres"
	transportHTTP "payment-service/internal/transport/http"
	"payment-service/internal/usecase"
)

func main() {
	// ── Database ──────────────────────────────────────────────────────────────
	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnv("DB_HOST", "localhost"),
		getEnv("DB_PORT", "5432"),
		getEnv("DB_USER", "postgres"),
		getEnv("DB_PASSWORD", "postgres"),
		getEnv("DB_NAME", "payments_db"),
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("failed to open DB: %v", err)
	}
	defer db.Close()

	for i := 1; i <= 12; i++ {
		if err = db.Ping(); err == nil {
			break
		}
		log.Printf("[%d/12] waiting for payments-db…", i)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("payments-db not reachable: %v", err)
	}
	log.Println("Connected to payments-db")

	// ── Migrations ────────────────────────────────────────────────────────────
	if err := runMigrations(db); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	// ── Composition Root (manual DI) ──────────────────────────────────────────
	paymentRepo := pgRepo.NewPaymentRepository(db)
	paymentUseCase := usecase.NewPaymentUseCase(paymentRepo)
	paymentHandler := transportHTTP.NewPaymentHandler(paymentUseCase)

	// ── Router ────────────────────────────────────────────────────────────────
	router := gin.Default()
	paymentHandler.RegisterRoutes(router)

	port := getEnv("PORT", "8081")
	log.Printf("Payment Service listening on :%s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func runMigrations(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS payments (
			id             VARCHAR(36)  PRIMARY KEY,
			order_id       VARCHAR(36)  NOT NULL,
			transaction_id VARCHAR(36)  NOT NULL,
			amount         BIGINT       NOT NULL CHECK (amount > 0),
			status         VARCHAR(50)  NOT NULL
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

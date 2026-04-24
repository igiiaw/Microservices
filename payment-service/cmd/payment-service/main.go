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

	paymentv1 "github.com/igiiaw/ap2-proto-gen/gen/go/payment/v1"
	"google.golang.org/grpc/reflection"
	pgRepo "payment-service/internal/repository/postgres"
	transportGRPC "payment-service/internal/transport/grpc"
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

	// HTTP delivery (kept for backwards-compat during migration) ─────────────
	httpHandler := transportHTTP.NewPaymentHandler(paymentUseCase)
	router := gin.Default()
	httpHandler.RegisterRoutes(router)

	httpPort := getEnv("PORT", "8081")
	go func() {
		log.Printf("Payment Service HTTP listening on :%s", httpPort)
		if err := router.Run(":" + httpPort); err != nil {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// gRPC delivery ───────────────────────────────────────────────────────────
	grpcPort := getEnv("GRPC_PORT", "9091")
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("failed to listen on :%s: %v", grpcPort, err)
	}

	// Logging interceptor added here
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(transportGRPC.LoggingUnaryInterceptor),
	)
	paymentv1.RegisterPaymentServiceServer(grpcServer, transportGRPC.NewPaymentServer(paymentUseCase))
	reflection.Register(grpcServer)

	log.Printf("Payment Service gRPC listening on :%s", grpcPort)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("grpc server error: %v", err)
	}
}

// getEnv is a helper function to read an environment variable or return a default value
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// runMigrations applies schema changes idempotently.
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

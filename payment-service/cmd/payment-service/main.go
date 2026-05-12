package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	amqp "github.com/rabbitmq/amqp091-go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	paymentv1 "github.com/igiiaw/ap2-proto-gen/gen/go/payment/v1"
	"payment-service/internal/messaging/rabbitmq"
	pgRepo "payment-service/internal/repository/postgres"
	transportGRPC "payment-service/internal/transport/grpc"
	transportHTTP "payment-service/internal/transport/http"
	"payment-service/internal/usecase"
)

func main() {
	// Database setup
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
		log.Printf("[%d/12] waiting for payments-db...", i)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("payments-db not reachable: %v", err)
	}
	log.Println("Connected to payments-db")

	// Run migrations
	if err := runMigrations(db); err != nil {
		log.Fatalf("migration failed: %v", err)
	}

	// RabbitMQ setup
	rabbitURL := getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/")

	var rabbitConn *amqp.Connection
	for i := 1; i <= 12; i++ {
		rabbitConn, err = amqp.Dial(rabbitURL)
		if err == nil {
			break
		}
		log.Printf("[%d/12] waiting for RabbitMQ...", i)
		time.Sleep(3 * time.Second)
	}
	if err != nil {
		log.Fatalf("RabbitMQ not reachable: %v", err)
	}
	defer rabbitConn.Close()
	log.Println("Connected to RabbitMQ")

	rabbitCh, err := rabbitConn.Channel()
	if err != nil {
		log.Fatalf("failed to open RabbitMQ channel: %v", err)
	}
	defer rabbitCh.Close()

	publisher, err := rabbitmq.NewPublisher(rabbitCh)
	if err != nil {
		log.Fatalf("failed to create publisher: %v", err)
	}

	// Dependency Injection
	paymentRepo := pgRepo.NewPaymentRepository(db)
	paymentUseCase := usecase.NewPaymentUseCase(paymentRepo, publisher)

	// HTTP Server
	httpHandler := transportHTTP.NewPaymentHandler(paymentUseCase)
	router := gin.Default()
	httpHandler.RegisterRoutes(router)

	httpPort := getEnv("PORT", "8081")
	httpServer := &http.Server{
		Addr:    ":" + httpPort,
		Handler: router,
	}
	go func() {
		log.Printf("Payment Service HTTP listening on :%s", httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server error: %v", err)
		}
	}()

	// gRPC Server
	grpcPort := getEnv("GRPC_PORT", "9091")
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("failed to listen on :%s: %v", grpcPort, err)
	}

	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(transportGRPC.LoggingUnaryInterceptor),
	)
	paymentv1.RegisterPaymentServiceServer(grpcServer, transportGRPC.NewPaymentServer(paymentUseCase))
	reflection.Register(grpcServer)

	go func() {
		log.Printf("Payment Service gRPC listening on :%s", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("grpc server error: %v", err)
		}
	}()

	// Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Printf("Received %s, shutting down...", sig)

	// 1. Stop gRPC server gracefully
	grpcServer.GracefulStop()
	log.Println("gRPC server stopped")

	// 2. Stop HTTP server with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Printf("HTTP server forced shutdown: %v", err)
	}
	log.Println("HTTP server stopped")

	// 3 & 4. RabbitMQ and DB close automatically via defers above
	log.Println("Payment Service shutdown complete")
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

// runMigrations ensures the payments table exists.
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

// payment-service/internal/transport/grpc/logging_interceptor.go
package grpc

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// LoggingUnaryInterceptor logs every unary RPC: method name, duration, and
// the resulting gRPC status code. It is transport-level concern only —
// no business data is inspected, so this is safe to apply to every RPC.
//
// Signature is fixed by gRPC: it must match grpc.UnaryServerInterceptor.
func LoggingUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	// Call the actual handler. Everything before this line runs "before" the RPC;
	// everything after runs "after". This is the whole middleware pattern in one call.
	resp, err := handler(ctx, req)

	duration := time.Since(start)
	code := status.Code(err) // returns codes.OK when err == nil

	log.Printf("gRPC %s | %s | %s", info.FullMethod, code, duration)

	return resp, err
}

package grpc

import (
	"context"
	"log"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

// LoggingUnaryInterceptor logs method name, duration, and status code for every RPC.
// doesn't touch any business data — safe to apply globally.
// signature is fixed by gRPC, has to match grpc.UnaryServerInterceptor
func LoggingUnaryInterceptor(
	ctx context.Context,
	req interface{},
	info *grpc.UnaryServerInfo,
	handler grpc.UnaryHandler,
) (interface{}, error) {
	start := time.Now()

	// everything before this is "before the RPC", everything after is "after"
	resp, err := handler(ctx, req)

	duration := time.Since(start)
	code := status.Code(err) // codes.OK when err == nil

	log.Printf("gRPC %s | %s | %s", info.FullMethod, code, duration)

	return resp, err
}

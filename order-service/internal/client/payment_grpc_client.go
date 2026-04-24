package client

import (
	"context"
	"errors"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"order-service/internal/domain"

	paymentv1 "github.com/igiiaw/ap2-proto-gen/gen/go/payment/v1"
)

// PaymentGRPCClient is the gRPC version of the payment adapter.
// Implements the same interface as the REST one so the use case doesn't care which is used.
type PaymentGRPCClient struct {
	conn       *grpc.ClientConn
	grpcClient paymentv1.PaymentServiceClient
	timeout    time.Duration
}

// NewPaymentGRPCClient dials once and reuses the connection — gRPC handles multiplexing.
func NewPaymentGRPCClient(conn *grpc.ClientConn, timeout time.Duration) *PaymentGRPCClient {
	return &PaymentGRPCClient{
		conn:       conn,
		grpcClient: paymentv1.NewPaymentServiceClient(conn),
		timeout:    timeout,
	}
}

// AuthorizePayment — same contract as the REST client, just over gRPC.
// network/timeout → ErrPaymentServiceUnavailable
// declined        → ErrPaymentDeclined
// success         → transaction ID, nil
func (c *PaymentGRPCClient) AuthorizePayment(orderID string, amount int64) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	resp, err := c.grpcClient.ProcessPayment(ctx, &paymentv1.ProcessPaymentRequest{
		OrderId: orderID,
		Amount:  amount,
	})
	if err != nil {
		// map gRPC codes to our domain errors
		if s, ok := status.FromError(err); ok {
			switch s.Code() {
			case codes.DeadlineExceeded, codes.Unavailable, codes.Internal, codes.Unknown:
				return "", domain.ErrPaymentServiceUnavailable
			}
		}
		return "", domain.ErrPaymentServiceUnavailable
	}

	if resp.GetStatus() == paymentv1.PaymentStatus_PAYMENT_STATUS_DECLINED {
		return "", domain.ErrPaymentDeclined
	}

	// not authorized and not declined — something weird happened, fail safe
	if resp.GetStatus() != paymentv1.PaymentStatus_PAYMENT_STATUS_AUTHORIZED {
		return "", errors.New("unexpected payment status from payment service")
	}

	return resp.GetTransactionId(), nil
}

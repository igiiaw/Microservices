package grpc

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"payment-service/internal/domain"
	"payment-service/internal/usecase"

	paymentv1 "github.com/igiiaw/ap2-proto-gen/gen/go/payment/v1"
)

// PaymentServer is the gRPC delivery adapter — same idea as the HTTP handler, different transport
type PaymentServer struct {
	paymentv1.UnimplementedPaymentServiceServer // embedding this means new RPCs won't break old servers
	uc                                          *usecase.PaymentUseCase
}

func NewPaymentServer(uc *usecase.PaymentUseCase) *PaymentServer {
	return &PaymentServer{uc: uc}
}

// ProcessPayment translates proto → use case → proto.
// the $1000 limit lives in domain.NewPayment, not here
func (s *PaymentServer) ProcessPayment(
	ctx context.Context,
	req *paymentv1.ProcessPaymentRequest,
) (*paymentv1.ProcessPaymentResponse, error) {
	if req.GetOrderId() == "" {
		return nil, status.Error(codes.InvalidArgument, "order_id is required")
	}
	if req.GetAmount() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "amount must be > 0")
	}

	// Mock email for the demo. In prod, fetch from User Service or request.
	email := "customer_" + req.GetOrderId()[:8] + "@example.com"

	p, err := s.uc.ProcessPayment(req.GetOrderId(), req.GetAmount(), email)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	return toProtoPayment(p), nil
}

// toProtoStatus maps our domain status strings to proto enum values
func toProtoStatus(s string) paymentv1.PaymentStatus {
	switch s {
	case domain.StatusAuthorized:
		return paymentv1.PaymentStatus_PAYMENT_STATUS_AUTHORIZED
	case domain.StatusDeclined:
		return paymentv1.PaymentStatus_PAYMENT_STATUS_DECLINED
	default:
		return paymentv1.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	}
}

// toProtoPayment maps a domain.Payment to the response proto.
// shared by ProcessPayment and ListPayments since the shape is the same
func toProtoPayment(p *domain.Payment) *paymentv1.ProcessPaymentResponse {
	return &paymentv1.ProcessPaymentResponse{
		Id:            p.ID,
		OrderId:       p.OrderID,
		TransactionId: p.TransactionID,
		Amount:        p.Amount,
		Status:        toProtoStatus(p.Status),
	}
}

// ListPayments returns payments filtered by status.
// status must be "Authorized", "Declined", or "" — anything else is rejected here
func (s *PaymentServer) ListPayments(
	ctx context.Context,
	req *paymentv1.ListPaymentsRequest,
) (*paymentv1.ListPaymentsResponse, error) {
	// validate the status value before it goes any deeper
	switch req.GetStatus() {
	case domain.StatusAuthorized, domain.StatusDeclined, "":
		// ok
	default:
		return nil, status.Errorf(codes.InvalidArgument,
			"invalid status %q: must be %q or %q",
			req.GetStatus(), domain.StatusAuthorized, domain.StatusDeclined)
	}

	payments, err := s.uc.ListPaymentsByStatus(req.GetStatus())
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}

	out := make([]*paymentv1.ProcessPaymentResponse, 0, len(payments))
	for _, p := range payments {
		out = append(out, toProtoPayment(p))
	}

	return &paymentv1.ListPaymentsResponse{Payments: out}, nil
}

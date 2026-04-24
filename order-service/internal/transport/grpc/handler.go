package grpc

import (
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"order-service/internal/domain"
	"order-service/internal/usecase"

	orderv1 "github.com/igiiaw/ap2-proto-gen/gen/go/order/v1"
)

// OrderServer handles gRPC calls for the Order Service
type OrderServer struct {
	orderv1.UnimplementedOrderServiceServer
	uc     *usecase.OrderUseCase
	events domain.OrderEventSubscriber
}

func NewOrderServer(uc *usecase.OrderUseCase, events domain.OrderEventSubscriber) *OrderServer {
	return &OrderServer{uc: uc, events: events}
}

// SubscribeToOrderUpdates streams status changes until the order hits a terminal state.
//
// Flow:
//  1. check the order exists
//  2. subscribe to broker BEFORE reading current status — avoids missing events in between
//  3. send current status as the first frame
//  4. forward events, re-reading from DB each time (broker says when, DB says what)
//  5. stop when Paid/Failed/Cancelled or client disconnects
func (s *OrderServer) SubscribeToOrderUpdates(
	req *orderv1.SubscribeToOrderUpdatesRequest,
	stream orderv1.OrderService_SubscribeToOrderUpdatesServer,
) error {
	orderID := req.GetOrderId()
	if orderID == "" {
		return status.Error(codes.InvalidArgument, "order_id is required")
	}

	order, err := s.uc.GetOrder(orderID)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			return status.Error(codes.NotFound, "order not found")
		}
		return status.Error(codes.Internal, err.Error())
	}

	// subscribe first, then send snapshot — order matters here
	ch, unsubscribe := s.events.Subscribe(orderID)
	defer unsubscribe()

	if err := stream.Send(toProtoUpdate(order.ID, order.Status)); err != nil {
		return err
	}
	if isTerminal(order.Status) {
		return nil // already done, snapshot was the last frame
	}

	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			// client disconnected
			return ctx.Err()

		case evt, ok := <-ch:
			if !ok {
				// channel closed, broker shutting down or we unsubscribed
				return nil
			}

			// re-read from DB — broker tells us when to look, not what changed
			fresh, err := s.uc.GetOrder(evt.OrderID)
			if err != nil {
				return status.Error(codes.Internal, err.Error())
			}
			if err := stream.Send(toProtoUpdate(fresh.ID, fresh.Status)); err != nil {
				return err
			}
			if isTerminal(fresh.Status) {
				return nil
			}
		}
	}
}

// isTerminal — order won't change status after these
func isTerminal(s string) bool {
	return s == domain.StatusPaid ||
		s == domain.StatusFailed ||
		s == domain.StatusCancelled
}

func toProtoUpdate(orderID, s string) *orderv1.OrderUpdate {
	return &orderv1.OrderUpdate{
		OrderId:   orderID,
		Status:    toProtoStatus(s),
		UpdatedAt: timestamppb.Now(),
	}
}

// toProtoStatus maps our domain strings to proto enum values
func toProtoStatus(s string) orderv1.OrderStatus {
	switch s {
	case domain.StatusPending:
		return orderv1.OrderStatus_ORDER_STATUS_PENDING
	case domain.StatusPaid:
		return orderv1.OrderStatus_ORDER_STATUS_PAID
	case domain.StatusFailed:
		return orderv1.OrderStatus_ORDER_STATUS_FAILED
	case domain.StatusCancelled:
		return orderv1.OrderStatus_ORDER_STATUS_CANCELLED
	default:
		return orderv1.OrderStatus_ORDER_STATUS_UNSPECIFIED
	}
}

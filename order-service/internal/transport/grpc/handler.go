// order-service/internal/transport/grpc/handler.go
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

// OrderServer is the gRPC delivery adapter for the Order Service.
// It uses the use case for reads (no business logic here) and the event
// subscriber port for live updates.
type OrderServer struct {
	orderv1.UnimplementedOrderServiceServer
	uc     *usecase.OrderUseCase
	events domain.OrderEventSubscriber
}

func NewOrderServer(uc *usecase.OrderUseCase, events domain.OrderEventSubscriber) *OrderServer {
	return &OrderServer{uc: uc, events: events}
}

// SubscribeToOrderUpdates streams real DB-backed status changes for one order.
//
// Flow:
//  1. Validate the order exists (DB read via the use case).
//  2. Send the CURRENT status as the first frame — so late subscribers aren't
//     left hanging if the terminal transition already happened.
//  3. Subscribe to the broker and forward every event.
//  4. Close the stream as soon as the order reaches a terminal state
//     (Paid / Failed / Cancelled), OR the client disconnects.
func (s *OrderServer) SubscribeToOrderUpdates(
	req *orderv1.SubscribeToOrderUpdatesRequest,
	stream orderv1.OrderService_SubscribeToOrderUpdatesServer,
) error {
	orderID := req.GetOrderId()
	if orderID == "" {
		return status.Error(codes.InvalidArgument, "order_id is required")
	}

	// Step 1 — the order must exist. Read goes through the use case.
	order, err := s.uc.GetOrder(orderID)
	if err != nil {
		if errors.Is(err, domain.ErrOrderNotFound) {
			return status.Error(codes.NotFound, "order not found")
		}
		return status.Error(codes.Internal, err.Error())
	}

	// Step 2 — subscribe BEFORE sending the snapshot, so we can't miss an event
	// that happens between the read and the subscribe (classic TOCTOU).
	ch, unsubscribe := s.events.Subscribe(orderID)
	defer unsubscribe()

	// Step 2b — send the current status as frame #1.
	if err := stream.Send(toProtoUpdate(order.ID, order.Status)); err != nil {
		return err
	}
	if isTerminal(order.Status) {
		return nil // already finished — snapshot was the last frame.
	}

	// Step 3 — forward events until terminal or client disconnect.
	ctx := stream.Context()
	for {
		select {
		case <-ctx.Done():
			// Client went away. gRPC will report this as codes.Canceled upstream.
			return ctx.Err()

		case evt, ok := <-ch:
			if !ok {
				// Broker closed our channel (unsubscribe was called somewhere
				// else, or server is shutting down).
				return nil
			}

			// Step 4 — verify against the DB before we send. This is the
			// "tied to real database status changes" guarantee: the broker
			// tells us *when* to look, but the DB is the source of truth.
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

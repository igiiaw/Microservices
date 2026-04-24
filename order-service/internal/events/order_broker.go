package events

import (
	"sync"

	"order-service/internal/domain"
)

// OrderBroker is an in-memory pub/sub for order status events.
// Works fine here since the REST handler and gRPC server are in the same binary.
// If we ever go multi-instance, swap this for Redis pub/sub — interfaces stay the same.
type OrderBroker struct {
	mu   sync.RWMutex
	subs map[string]map[chan domain.OrderStatusEvent]struct{} // orderID → set of channels
}

func NewOrderBroker() *OrderBroker {
	return &OrderBroker{
		subs: make(map[string]map[chan domain.OrderStatusEvent]struct{}),
	}
}

// PublishStatusChanged sends the event to all subscribers of this order.
// Non-blocking — slow subscribers get their event dropped, not waited on.
func (b *OrderBroker) PublishStatusChanged(orderID, newStatus string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	evt := domain.OrderStatusEvent{OrderID: orderID, NewStatus: newStatus}
	for ch := range b.subs[orderID] {
		select {
		case ch <- evt:
		default:
			// subscriber is too slow, skip them
		}
	}
}

// Subscribe returns a channel + cleanup func.
// seriously, always defer the unsubscribe
func (b *OrderBroker) Subscribe(orderID string) (<-chan domain.OrderStatusEvent, func()) {
	ch := make(chan domain.OrderStatusEvent, 8) // small buffer handles bursts

	b.mu.Lock()
	if _, ok := b.subs[orderID]; !ok {
		b.subs[orderID] = make(map[chan domain.OrderStatusEvent]struct{})
	}
	b.subs[orderID][ch] = struct{}{}
	b.mu.Unlock()

	unsubscribe := func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if set, ok := b.subs[orderID]; ok {
			delete(set, ch)
			// clean up the map entry if nobody's left listening
			if len(set) == 0 {
				delete(b.subs, orderID)
			}
		}
		close(ch)
	}

	return ch, unsubscribe
}

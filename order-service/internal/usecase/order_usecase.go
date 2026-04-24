package usecase

import (
	"errors"

	"github.com/google/uuid"

	"order-service/internal/domain"
)

// OrderUseCase is where the business logic lives.
// only talks to interfaces — never to concrete DB or HTTP clients directly
type OrderUseCase struct {
	repo            domain.OrderRepository
	paymentClient   domain.PaymentClient
	idempotencyRepo domain.IdempotencyRepository // nil if feature not wired in
	events          domain.OrderEventPublisher   // nil if no broker wired in
}

func NewOrderUseCase(
	repo domain.OrderRepository,
	paymentClient domain.PaymentClient,
	idempotencyRepo domain.IdempotencyRepository,
	events domain.OrderEventPublisher,
) *OrderUseCase {
	return &OrderUseCase{
		repo:            repo,
		paymentClient:   paymentClient,
		idempotencyRepo: idempotencyRepo,
		events:          events,
	}
}

// CreateOrder creates the order and immediately tries to authorize payment.
func (uc *OrderUseCase) CreateOrder(idempotencyKey, customerID, itemName string, amount int64) (*domain.Order, error) {
	// already processed this key? return the original result, no side effects
	if idempotencyKey != "" && uc.idempotencyRepo != nil {
		existing, err := uc.idempotencyRepo.FindOrderByKey(idempotencyKey)
		if err == nil {
			return existing, nil
		}
	}

	order, err := domain.NewOrder(customerID, itemName, amount)
	if err != nil {
		return nil, err
	}
	order.ID = uuid.New().String()

	// save as Pending first — we want a DB record before calling the payment service
	if err := uc.repo.Save(order); err != nil {
		return nil, err
	}

	_, payErr := uc.paymentClient.AuthorizePayment(order.ID, order.Amount)
	if payErr != nil {
		// payment failed for any reason → mark Failed
		// better than leaving it Pending since no payment was actually authorized
		order.Status = domain.StatusFailed
		_ = uc.repo.Update(order) // best-effort, ignore the error
		uc.publishStatus(order.ID, order.Status)
		return order, payErr
	}

	order.Status = domain.StatusPaid
	if err := uc.repo.Update(order); err != nil {
		return nil, err
	}
	uc.publishStatus(order.ID, order.Status)

	// save the key so future duplicates get this same order back
	if idempotencyKey != "" && uc.idempotencyRepo != nil {
		_ = uc.idempotencyRepo.SaveKey(idempotencyKey, order.ID)
	}

	return order, nil
}

func (uc *OrderUseCase) GetOrder(id string) (*domain.Order, error) {
	return uc.repo.FindByID(id)
}

// CancelOrder cancels a Pending order — domain.Cancel() enforces the rules
func (uc *OrderUseCase) CancelOrder(id string) (*domain.Order, error) {
	order, err := uc.repo.FindByID(id)
	if err != nil {
		return nil, err
	}

	if err := order.Cancel(); err != nil {
		return nil, err
	}

	if err := uc.repo.Update(order); err != nil {
		return nil, err
	}
	uc.publishStatus(order.ID, order.Status)
	return order, nil
}

// IsCancelConflict returns true if the error came from trying to cancel an uncancellable order
func IsCancelConflict(err error) bool {
	return errors.Is(err, domain.ErrAlreadyPaid) || errors.Is(err, domain.ErrCannotCancel)
}

func (uc *OrderUseCase) GetOrdersByCustomerID(customerID string) ([]*domain.Order, error) {
	return uc.repo.FindByCustomerID(customerID)
}

// publishStatus is nil-safe — does nothing if no broker is configured
func (uc *OrderUseCase) publishStatus(orderID, status string) {
	if uc.events != nil {
		uc.events.PublishStatusChanged(orderID, status)
	}
}

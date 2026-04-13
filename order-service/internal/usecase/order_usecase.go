package usecase

import (
	"errors"

	"github.com/google/uuid"

	"order-service/internal/domain"
)

// OrderUseCase orchestrates order workflows.
// It depends only on domain interfaces (Ports) – never on concrete implementations.
type OrderUseCase struct {
	repo            domain.OrderRepository
	paymentClient   domain.PaymentClient
	idempotencyRepo domain.IdempotencyRepository // may be nil if feature disabled
}

func NewOrderUseCase(
	repo domain.OrderRepository,
	paymentClient domain.PaymentClient,
	idempotencyRepo domain.IdempotencyRepository,
) *OrderUseCase {
	return &OrderUseCase{
		repo:            repo,
		paymentClient:   paymentClient,
		idempotencyRepo: idempotencyRepo,
	}
}

// CreateOrder creates a new order and synchronously requests payment authorisation.
// idempotencyKey may be empty – when provided, duplicate submissions are safely rejected.
func (uc *OrderUseCase) CreateOrder(idempotencyKey, customerID, itemName string, amount int64) (*domain.Order, error) {
	// --- Bonus: Idempotency check ---
	if idempotencyKey != "" && uc.idempotencyRepo != nil {
		existing, err := uc.idempotencyRepo.FindOrderByKey(idempotencyKey)
		if err == nil {
			// Already processed – return the original result without side effects.
			return existing, nil
		}
	}

	// --- Domain validation (business rule: amount > 0) ---
	order, err := domain.NewOrder(customerID, itemName, amount)
	if err != nil {
		return nil, err
	}
	order.ID = uuid.New().String()

	// Persist as Pending before calling Payment Service.
	if err := uc.repo.Save(order); err != nil {
		return nil, err
	}

	// --- Call Payment Service (synchronous REST, 2-second timeout enforced by the client) ---
	_, payErr := uc.paymentClient.AuthorizePayment(order.ID, order.Amount)
	if payErr != nil {
		// Mark the order as Failed regardless of whether the cause was a timeout,
		// network error, or an explicit Declined response.
		// Design choice: "Failed" is preferred over leaving it "Pending" because
		// the outcome is deterministic – no payment was authorised.
		order.Status = domain.StatusFailed
		_ = uc.repo.Update(order) // best-effort status update
		return order, payErr
	}

	order.Status = domain.StatusPaid
	if err := uc.repo.Update(order); err != nil {
		return nil, err
	}

	// --- Bonus: Persist idempotency key after success ---
	if idempotencyKey != "" && uc.idempotencyRepo != nil {
		_ = uc.idempotencyRepo.SaveKey(idempotencyKey, order.ID)
	}

	return order, nil
}

// GetOrder retrieves an order by its ID.
func (uc *OrderUseCase) GetOrder(id string) (*domain.Order, error) {
	return uc.repo.FindByID(id)
}

// CancelOrder cancels a Pending order. Business rule: Paid orders cannot be cancelled.
func (uc *OrderUseCase) CancelOrder(id string) (*domain.Order, error) {
	order, err := uc.repo.FindByID(id)
	if err != nil {
		return nil, err
	}

	// Cancel() enforces the domain invariant.
	if err := order.Cancel(); err != nil {
		return nil, err
	}

	if err := uc.repo.Update(order); err != nil {
		return nil, err
	}

	return order, nil
}

// IsCancelConflict returns true when the error is a domain cancellation constraint.
func IsCancelConflict(err error) bool {
	return errors.Is(err, domain.ErrAlreadyPaid) || errors.Is(err, domain.ErrCannotCancel)
}

func (uc *OrderUseCase) GetOrdersByCustomerID(customerID string) ([]*domain.Order, error) {
	return uc.repo.FindByCustomerID(customerID)
}

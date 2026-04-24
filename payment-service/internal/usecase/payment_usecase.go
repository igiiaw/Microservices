package usecase

import (
	"github.com/google/uuid"

	"payment-service/internal/domain"
)

// PaymentUseCase handles payment processing — generates IDs, applies domain rules, saves
type PaymentUseCase struct {
	repo domain.PaymentRepository
}

func NewPaymentUseCase(repo domain.PaymentRepository) *PaymentUseCase {
	return &PaymentUseCase{repo: repo}
}

// ProcessPayment runs the limit check (via domain.NewPayment) and persists the result
func (uc *PaymentUseCase) ProcessPayment(orderID string, amount int64) (*domain.Payment, error) {
	payment := domain.NewPayment(orderID, amount)
	payment.ID = uuid.New().String()
	payment.TransactionID = uuid.New().String()

	if err := uc.repo.Save(payment); err != nil {
		return nil, err
	}
	return payment, nil
}

// GetPaymentByOrderID fetches the stored payment for a given order
func (uc *PaymentUseCase) GetPaymentByOrderID(orderID string) (*domain.Payment, error) {
	return uc.repo.FindByOrderID(orderID)
}

// ListPaymentsByStatus returns payments filtered by status.
// the gRPC handler validates the status value before calling this — we trust it here
func (uc *PaymentUseCase) ListPaymentsByStatus(status string) ([]*domain.Payment, error) {
	return uc.repo.ListByStatus(status)
}

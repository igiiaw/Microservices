package usecase

import (
	"github.com/google/uuid"

	"payment-service/internal/domain"
)

// PaymentUseCase orchestrates payment processing.
// Business logic: generates a transaction ID, enforces the payment limit (via domain),
// and persists the result.
type PaymentUseCase struct {
	repo domain.PaymentRepository
}

func NewPaymentUseCase(repo domain.PaymentRepository) *PaymentUseCase {
	return &PaymentUseCase{repo: repo}
}

// ProcessPayment applies the payment-limit rule and persists the payment.
func (uc *PaymentUseCase) ProcessPayment(orderID string, amount int64) (*domain.Payment, error) {
	payment := domain.NewPayment(orderID, amount)
	payment.ID = uuid.New().String()
	payment.TransactionID = uuid.New().String()

	if err := uc.repo.Save(payment); err != nil {
		return nil, err
	}
	return payment, nil
}

// GetPaymentByOrderID retrieves a stored payment for the given order.
func (uc *PaymentUseCase) GetPaymentByOrderID(orderID string) (*domain.Payment, error) {
	return uc.repo.FindByOrderID(orderID)
}

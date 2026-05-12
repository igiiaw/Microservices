package usecase

import (
	"log"

	"github.com/google/uuid"

	"payment-service/internal/domain"
)

type PaymentUseCase struct {
	repo      domain.PaymentRepository
	publisher domain.EventPublisher // Optional; nil disables events
}

func NewPaymentUseCase(repo domain.PaymentRepository, publisher domain.EventPublisher) *PaymentUseCase {
	return &PaymentUseCase{repo: repo, publisher: publisher}
}

func (uc *PaymentUseCase) ProcessPayment(orderID string, amount int64, customerEmail string) (*domain.Payment, error) {
	payment := domain.NewPayment(orderID, amount)
	payment.ID = uuid.New().String()
	payment.TransactionID = uuid.New().String()

	if err := uc.repo.Save(payment); err != nil {
		return nil, err
	}

	// Best-effort notification. If publishing fails, the payment stays saved.
	if uc.publisher != nil {
		event := domain.PaymentCompletedEvent{
			OrderID:       payment.OrderID,
			Amount:        payment.Amount,
			CustomerEmail: customerEmail,
			Status:        payment.Status,
		}
		if err := uc.publisher.PublishPaymentCompleted(event); err != nil {
			log.Printf("WARNING: failed to publish payment.completed event: %v", err)
		}
	}

	return payment, nil
}

func (uc *PaymentUseCase) GetPaymentByOrderID(orderID string) (*domain.Payment, error) {
	return uc.repo.FindByOrderID(orderID)
}

func (uc *PaymentUseCase) ListPaymentsByStatus(status string) ([]*domain.Payment, error) {
	return uc.repo.ListByStatus(status)
}

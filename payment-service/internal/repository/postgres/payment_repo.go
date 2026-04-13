package postgres

import (
	"database/sql"

	"payment-service/internal/domain"
)

// PaymentRepository is the concrete PostgreSQL adapter for domain.PaymentRepository.
type PaymentRepository struct {
	db *sql.DB
}

func NewPaymentRepository(db *sql.DB) *PaymentRepository {
	return &PaymentRepository{db: db}
}

func (r *PaymentRepository) Save(p *domain.Payment) error {
	const q = `
		INSERT INTO payments (id, order_id, transaction_id, amount, status)
		VALUES ($1, $2, $3, $4, $5)`
	_, err := r.db.Exec(q, p.ID, p.OrderID, p.TransactionID, p.Amount, p.Status)
	return err
}

func (r *PaymentRepository) FindByOrderID(orderID string) (*domain.Payment, error) {
	const q = `
		SELECT id, order_id, transaction_id, amount, status
		FROM payments WHERE order_id = $1`

	var p domain.Payment
	err := r.db.QueryRow(q, orderID).Scan(
		&p.ID,
		&p.OrderID,
		&p.TransactionID,
		&p.Amount,
		&p.Status,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrPaymentNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

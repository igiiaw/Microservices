package postgres

import (
	"database/sql"

	"order-service/internal/domain"
)

// IdempotencyRepository stores Idempotency-Key → Order mappings (Bonus feature).
type IdempotencyRepository struct {
	orderRepo *OrderRepository
	db        *sql.DB
}

func NewIdempotencyRepository(db *sql.DB, orderRepo *OrderRepository) *IdempotencyRepository {
	return &IdempotencyRepository{db: db, orderRepo: orderRepo}
}

func (r *IdempotencyRepository) FindOrderByKey(key string) (*domain.Order, error) {
	const q = `SELECT order_id FROM idempotency_keys WHERE key = $1`
	var orderID string
	err := r.db.QueryRow(q, key).Scan(&orderID)
	if err == sql.ErrNoRows {
		return nil, domain.ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.orderRepo.FindByID(orderID)
}

func (r *IdempotencyRepository) SaveKey(key, orderID string) error {
	const q = `
		INSERT INTO idempotency_keys (key, order_id)
		VALUES ($1, $2)
		ON CONFLICT (key) DO NOTHING`
	_, err := r.db.Exec(q, key, orderID)
	return err
}

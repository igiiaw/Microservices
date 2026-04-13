package postgres

import (
	"database/sql"

	"order-service/internal/domain"
)

// OrderRepository is the concrete PostgreSQL adapter for domain.OrderRepository.
type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Save(order *domain.Order) error {
	const q = `
		INSERT INTO orders (id, customer_id, item_name, amount, status, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)`
	_, err := r.db.Exec(q,
		order.ID,
		order.CustomerID,
		order.ItemName,
		order.Amount,
		order.Status,
		order.CreatedAt,
	)
	return err
}

func (r *OrderRepository) FindByID(id string) (*domain.Order, error) {
	const q = `
		SELECT id, customer_id, item_name, amount, status, created_at
		FROM orders WHERE id = $1`

	var o domain.Order
	err := r.db.QueryRow(q, id).Scan(
		&o.ID,
		&o.CustomerID,
		&o.ItemName,
		&o.Amount,
		&o.Status,
		&o.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, domain.ErrOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func (r *OrderRepository) Update(order *domain.Order) error {
	const q = `UPDATE orders SET status = $1 WHERE id = $2`
	_, err := r.db.Exec(q, order.Status, order.ID)
	return err
}

func (r *OrderRepository) FindByCustomerID(customerID string) ([]*domain.Order, error) {
	const q = `
		SELECT id, customer_id, item_name, amount, status, created_at
		FROM orders WHERE customer_id = $1
		ORDER BY created_at DESC`

	rows, err := r.db.Query(q, customerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*domain.Order
	for rows.Next() {
		var o domain.Order
		if err := rows.Scan(
			&o.ID,
			&o.CustomerID,
			&o.ItemName,
			&o.Amount,
			&o.Status,
			&o.CreatedAt,
		); err != nil {
			return nil, err
		}
		orders = append(orders, &o)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if orders == nil {
		orders = []*domain.Order{}
	}
	return orders, nil
}

package orders

import (
	"context"
	"database/sql"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/domain"
)

type OrderRepository struct {
	db *sql.DB
}

func NewOrderRepository(db *sql.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

func (r *OrderRepository) Create(ctx context.Context, order *domain.Order) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	order.ID = uuid.New().String()

	_, err = tx.ExecContext(ctx, `
		INSERT INTO orders (id, customer_id, status, total, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $5)
	`, order.ID, order.CustomerID, order.Status, order.Total, order.CreatedAt)
	if err != nil {
		return err
	}

	for _, item := range order.Items {
		itemID := uuid.New().String()
		_, err = tx.ExecContext(ctx, `
			INSERT INTO order_items (id, order_id, item_id, quantity, price)
			VALUES ($1, $2, $3, $4, $5)
		`, itemID, order.ID, item.ItemID, item.Quantity, item.Price)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (r *OrderRepository) GetByID(ctx context.Context, id string) (*domain.Order, error) {
	order := &domain.Order{}

	err := r.db.QueryRowContext(ctx, `
		SELECT id, customer_id, status, total, created_at
		FROM orders
		WHERE id = $1
	`, id).Scan(&order.ID, &order.CustomerID, &order.Status, &order.Total, &order.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT item_id, quantity, price
		FROM order_items
		WHERE order_id = $1
	`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var item domain.OrderItem
		if err := rows.Scan(&item.ItemID, &item.Quantity, &item.Price); err != nil {
			return nil, err
		}
		order.Items = append(order.Items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return order, nil
}

func (r *OrderRepository) UpdateStatus(ctx context.Context, id string, status domain.OrderStatus) (*domain.Order, error) {
	result, err := r.db.ExecContext(ctx, `
		UPDATE orders SET status = $1, updated_at = NOW()
		WHERE id = $2
	`, status, id)
	if err != nil {
		return nil, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}

	if rowsAffected == 0 {
		return nil, nil
	}

	return r.GetByID(ctx, id)
}

func (r *OrderRepository) List(ctx context.Context) ([]domain.Order, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, customer_id, status, total, created_at
		FROM orders
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	orderMap := make(map[string]*domain.Order)
	var orderIDs []string

	for rows.Next() {
		var order domain.Order
		if err := rows.Scan(&order.ID, &order.CustomerID, &order.Status, &order.Total, &order.CreatedAt); err != nil {
			return nil, err
		}
		order.Items = []domain.OrderItem{}
		orderMap[order.ID] = &order
		orderIDs = append(orderIDs, order.ID)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(orderIDs) == 0 {
		return []domain.Order{}, nil
	}

	itemRows, err := r.db.QueryContext(ctx, `
		SELECT order_id, item_id, quantity, price
		FROM order_items
		WHERE order_id = ANY($1)
	`, pq.Array(orderIDs))
	if err != nil {
		return nil, err
	}
	defer func() { _ = itemRows.Close() }()

	for itemRows.Next() {
		var orderID string
		var item domain.OrderItem
		if err := itemRows.Scan(&orderID, &item.ItemID, &item.Quantity, &item.Price); err != nil {
			return nil, err
		}
		order := orderMap[orderID]
		order.Items = append(order.Items, item)
	}

	if err := itemRows.Err(); err != nil {
		return nil, err
	}

	orders := make([]domain.Order, 0, len(orderIDs))
	for _, id := range orderIDs {
		orders = append(orders, *orderMap[id])
	}

	return orders, nil
}

func (r *OrderRepository) ListNPlus1(ctx context.Context) ([]domain.Order, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, customer_id, status, total, created_at
		FROM orders
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var orders []domain.Order
	for rows.Next() {
		var order domain.Order
		if err := rows.Scan(&order.ID, &order.CustomerID, &order.Status, &order.Total, &order.CreatedAt); err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range orders {
		itemRows, err := r.db.QueryContext(ctx, `
			SELECT item_id, quantity, price
			FROM order_items
			WHERE order_id = $1
		`, orders[i].ID)
		if err != nil {
			return nil, err
		}

		for itemRows.Next() {
			var item domain.OrderItem
			if err := itemRows.Scan(&item.ItemID, &item.Quantity, &item.Price); err != nil {
				_ = itemRows.Close()
				return nil, err
			}
			orders[i].Items = append(orders[i].Items, item)
		}

		if err := itemRows.Err(); err != nil {
			_ = itemRows.Close()
			return nil, err
		}
		_ = itemRows.Close()
	}

	return orders, nil
}

package inventory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/domain"
)

var ErrInsufficientStock = errors.New("insufficient stock")

type InventoryRepository struct {
	db *sql.DB
}

func NewInventoryRepository(db *sql.DB) *InventoryRepository {
	return &InventoryRepository{db: db}
}

func (r *InventoryRepository) ListAll(ctx context.Context) ([]domain.StockLevel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT item_id, available, reserved
		FROM items
		ORDER BY item_id
	`)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var items []domain.StockLevel
	for rows.Next() {
		var stock domain.StockLevel
		if err := rows.Scan(&stock.ItemID, &stock.Available, &stock.Reserved); err != nil {
			return nil, err
		}
		items = append(items, stock)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return items, nil
}

func (r *InventoryRepository) GetStock(ctx context.Context, itemID string) (*domain.StockLevel, error) {
	stock := &domain.StockLevel{}

	err := r.db.QueryRowContext(ctx, `
		SELECT item_id, available, reserved
		FROM items
		WHERE item_id = $1
	`, itemID).Scan(&stock.ItemID, &stock.Available, &stock.Reserved)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	return stock, nil
}

func (r *InventoryRepository) Reserve(ctx context.Context, itemID string, quantity int) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE items
		SET available = available - $2, reserved = reserved + $2
		WHERE item_id = $1 AND available >= $2
	`, itemID, quantity)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return ErrInsufficientStock
	}

	return nil
}

func (r *InventoryRepository) Release(ctx context.Context, itemID string, quantity int) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE items
		SET available = available + $2, reserved = reserved - $2
		WHERE item_id = $1 AND reserved >= $2
	`, itemID, quantity)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return errors.New("insufficient reserved stock to release")
	}

	return nil
}

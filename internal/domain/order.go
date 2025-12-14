package domain

import "time"

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusShipped   OrderStatus = "shipped"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type OrderItem struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
	Price    int64  `json:"price"`
}

type Order struct {
	ID         string      `json:"id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Total      int64       `json:"total"`
	Status     OrderStatus `json:"status"`
	CreatedAt  time.Time   `json:"created_at"`
}

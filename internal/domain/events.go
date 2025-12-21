package domain

import "time"

type OrderCreatedEvent struct {
	OrderID    string      `json:"order_id"`
	CustomerID string      `json:"customer_id"`
	Items      []OrderItem `json:"items"`
	Timestamp  time.Time   `json:"timestamp"`
}

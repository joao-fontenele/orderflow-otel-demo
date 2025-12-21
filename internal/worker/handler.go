package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/domain"
)

type NotificationHandler struct {
	emailServiceURL     string
	ordersServiceURL    string
	inventoryServiceURL string
	httpClient          *http.Client
	logger              *slog.Logger
}

func NewNotificationHandler(emailServiceURL, ordersServiceURL, inventoryServiceURL string, client *http.Client, logger *slog.Logger) *NotificationHandler {
	return &NotificationHandler{
		emailServiceURL:     emailServiceURL,
		ordersServiceURL:    ordersServiceURL,
		inventoryServiceURL: inventoryServiceURL,
		httpClient:          client,
		logger:              logger,
	}
}

type reservedItem struct {
	ItemID   string
	Quantity int
}

func (h *NotificationHandler) Handle(ctx context.Context, payload []byte) error {
	var event domain.OrderCreatedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return fmt.Errorf("unmarshal order created event: %w", err)
	}

	h.logger.Info("processing order created event", "order_id", event.OrderID, "customer_id", event.CustomerID)

	reserved, err := h.reserveStock(ctx, event)
	if err != nil {
		h.logger.Error("failed to reserve stock", "error", err, "order_id", event.OrderID)

		h.releaseStock(ctx, reserved)

		if err := h.updateOrderStatus(ctx, event.OrderID, domain.OrderStatusCancelled); err != nil {
			h.logger.Error("failed to cancel order", "error", err, "order_id", event.OrderID)
			return fmt.Errorf("cancel order after stock failure: %w", err)
		}

		if err := h.sendCancellationEmail(ctx, event); err != nil {
			h.logger.Error("failed to send cancellation email", "error", err, "order_id", event.OrderID)
			return fmt.Errorf("send cancellation email: %w", err)
		}

		h.logger.Info("order cancelled due to insufficient stock", "order_id", event.OrderID)
		return nil
	}

	if err := h.sendConfirmationEmail(ctx, event); err != nil {
		h.logger.Error("failed to send confirmation email", "error", err, "order_id", event.OrderID)
		return fmt.Errorf("send confirmation email: %w", err)
	}

	if err := h.updateOrderStatus(ctx, event.OrderID, domain.OrderStatusConfirmed); err != nil {
		h.logger.Error("failed to update order status", "error", err, "order_id", event.OrderID)
		return fmt.Errorf("update order status: %w", err)
	}

	h.logger.Info("order processing complete", "order_id", event.OrderID)
	return nil
}

func (h *NotificationHandler) reserveStock(ctx context.Context, event domain.OrderCreatedEvent) ([]reservedItem, error) {
	var reserved []reservedItem

	for _, item := range event.Items {
		body := map[string]int{"quantity": item.Quantity}
		data, err := json.Marshal(body)
		if err != nil {
			return reserved, fmt.Errorf("marshal reserve request: %w", err)
		}

		url := fmt.Sprintf("%s/stock/%s/reserve", h.inventoryServiceURL, item.ItemID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return reserved, fmt.Errorf("create reserve request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := h.httpClient.Do(req)
		if err != nil {
			return reserved, fmt.Errorf("reserve stock for item %s: %w", item.ItemID, err)
		}
		_ = resp.Body.Close()

		if resp.StatusCode == http.StatusConflict {
			return reserved, fmt.Errorf("insufficient stock for item %s", item.ItemID)
		}

		if resp.StatusCode != http.StatusOK {
			return reserved, fmt.Errorf("inventory service returned status %d for item %s", resp.StatusCode, item.ItemID)
		}

		reserved = append(reserved, reservedItem{ItemID: item.ItemID, Quantity: item.Quantity})
	}

	return reserved, nil
}

func (h *NotificationHandler) releaseStock(ctx context.Context, reserved []reservedItem) {
	for _, item := range reserved {
		body := map[string]int{"quantity": item.Quantity}
		data, err := json.Marshal(body)
		if err != nil {
			h.logger.Error("failed to marshal release request", "error", err, "item_id", item.ItemID)
			continue
		}

		url := fmt.Sprintf("%s/stock/%s/release", h.inventoryServiceURL, item.ItemID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			h.logger.Error("failed to create release request", "error", err, "item_id", item.ItemID)
			continue
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := h.httpClient.Do(req)
		if err != nil {
			h.logger.Error("failed to release stock", "error", err, "item_id", item.ItemID)
			continue
		}
		_ = resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			h.logger.Error("failed to release stock", "status", resp.StatusCode, "item_id", item.ItemID)
		}
	}
}

func (h *NotificationHandler) sendConfirmationEmail(ctx context.Context, event domain.OrderCreatedEvent) error {
	body := map[string]string{
		"to":      event.CustomerID + "@example.com",
		"subject": "Order Confirmation: " + event.OrderID,
		"body":    fmt.Sprintf("Your order %s has been confirmed with %d items.", event.OrderID, len(event.Items)),
	}

	return h.sendEmail(ctx, body)
}

func (h *NotificationHandler) sendCancellationEmail(ctx context.Context, event domain.OrderCreatedEvent) error {
	body := map[string]string{
		"to":      event.CustomerID + "@example.com",
		"subject": "Order Cancelled: " + event.OrderID,
		"body":    fmt.Sprintf("Your order %s has been cancelled due to insufficient stock. You will be reimbursed.", event.OrderID),
	}

	return h.sendEmail(ctx, body)
}

func (h *NotificationHandler) sendEmail(ctx context.Context, body map[string]string) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, h.emailServiceURL+"/send", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("email service returned status %d", resp.StatusCode)
	}

	return nil
}

func (h *NotificationHandler) updateOrderStatus(ctx context.Context, orderID string, status domain.OrderStatus) error {
	body := map[string]string{
		"status": string(status),
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/orders/%s/status", h.ordersServiceURL, orderID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("orders service returned status %d", resp.StatusCode)
	}

	return nil
}

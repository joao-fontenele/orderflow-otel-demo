package orders

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/domain"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/messaging"
)

type Handler struct {
	repo     *OrderRepository
	producer *messaging.Producer
	logger   *slog.Logger
}

func NewHandler(repo *OrderRepository, producer *messaging.Producer, logger *slog.Logger) *Handler {
	return &Handler{
		repo:     repo,
		producer: producer,
		logger:   logger,
	}
}

type createOrderRequest struct {
	CustomerID string             `json:"customer_id"`
	Items      []domain.OrderItem `json:"items"`
}

func (h *Handler) HandleCreate(w http.ResponseWriter, r *http.Request) {
	var req createOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var total int64
	for _, item := range req.Items {
		total += int64(item.Quantity) * item.Price
	}

	order := &domain.Order{
		CustomerID: req.CustomerID,
		Items:      req.Items,
		Total:      total,
		Status:     domain.OrderStatusPending,
		CreatedAt:  time.Now().UTC(),
	}

	if err := h.repo.Create(r.Context(), order); err != nil {
		h.logger.Error("failed to create order", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if h.producer != nil {
		event := domain.OrderCreatedEvent{
			OrderID:    order.ID,
			CustomerID: order.CustomerID,
			Items:      order.Items,
			Timestamp:  order.CreatedAt,
		}
		if err := h.producer.Publish(r.Context(), order.ID, event); err != nil {
			h.logger.Error("failed to publish order created event", "error", err, "order_id", order.ID)
		}
	}

	h.logger.Info("order created", "order_id", order.ID, "customer_id", order.CustomerID)
	h.writeJSON(w, http.StatusCreated, order)
}

func (h *Handler) HandleGet(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.writeError(w, http.StatusBadRequest, "missing order id")
		return
	}

	order, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		h.logger.Error("failed to get order", "error", err, "id", id)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if order == nil {
		h.writeError(w, http.StatusNotFound, "order not found")
		return
	}

	h.logger.Info("order retrieved", "order_id", order.ID)
	h.writeJSON(w, http.StatusOK, order)
}

type updateStatusRequest struct {
	Status domain.OrderStatus `json:"status"`
}

func (h *Handler) HandleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.writeError(w, http.StatusBadRequest, "missing order id")
		return
	}

	var req updateStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	order, err := h.repo.UpdateStatus(r.Context(), id, req.Status)
	if err != nil {
		h.logger.Error("failed to update order status", "error", err, "id", id)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if order == nil {
		h.writeError(w, http.StatusNotFound, "order not found")
		return
	}

	h.logger.Info("order status updated", "order_id", order.ID, "status", order.Status)
	h.writeJSON(w, http.StatusOK, order)
}

func (h *Handler) HandleList(w http.ResponseWriter, r *http.Request) {
	orders, err := h.repo.List(r.Context())
	if err != nil {
		h.logger.Error("failed to list orders", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.logger.Info("orders listed", "count", len(orders))
	h.writeJSON(w, http.StatusOK, orders)
}

func (h *Handler) HandleListNPlus1(w http.ResponseWriter, r *http.Request) {
	orders, err := h.repo.ListNPlus1(r.Context())
	if err != nil {
		h.logger.Error("failed to list orders (n+1)", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.logger.Info("orders listed (n+1)", "count", len(orders))
	h.writeJSON(w, http.StatusOK, orders)
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("failed to encode response", "error", err)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}

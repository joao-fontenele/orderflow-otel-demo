package inventory

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

type Handler struct {
	repo   *InventoryRepository
	logger *slog.Logger
}

func NewHandler(repo *InventoryRepository, logger *slog.Logger) *Handler {
	return &Handler{
		repo:   repo,
		logger: logger,
	}
}

func (h *Handler) HandleListStock(w http.ResponseWriter, r *http.Request) {
	items, err := h.repo.ListAll(r.Context())
	if err != nil {
		h.logger.Error("failed to list stock", "error", err)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.logger.Info("stock listed", "count", len(items))
	h.writeJSON(w, http.StatusOK, items)
}

func (h *Handler) HandleGetStock(w http.ResponseWriter, r *http.Request) {
	itemID := r.PathValue("itemId")
	if itemID == "" {
		h.writeError(w, http.StatusBadRequest, "missing item id")
		return
	}

	stock, err := h.repo.GetStock(r.Context(), itemID)
	if err != nil {
		h.logger.Error("failed to get stock", "error", err, "item_id", itemID)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if stock == nil {
		h.writeError(w, http.StatusNotFound, "item not found")
		return
	}

	h.logger.Info("stock retrieved", "item_id", itemID)
	h.writeJSON(w, http.StatusOK, stock)
}

type reserveRequest struct {
	Quantity int `json:"quantity"`
}

func (h *Handler) HandleReserve(w http.ResponseWriter, r *http.Request) {
	itemID := r.PathValue("itemId")
	if itemID == "" {
		h.writeError(w, http.StatusBadRequest, "missing item id")
		return
	}

	var req reserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	stock, err := h.repo.GetStock(r.Context(), itemID)
	if err != nil {
		h.logger.Error("failed to get stock", "error", err, "item_id", itemID)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	if stock == nil {
		h.writeError(w, http.StatusNotFound, "item not found")
		return
	}

	if err := h.repo.Reserve(r.Context(), itemID, req.Quantity); err != nil {
		if errors.Is(err, ErrInsufficientStock) {
			h.writeError(w, http.StatusConflict, "insufficient stock")
			return
		}
		h.logger.Error("failed to reserve stock", "error", err, "item_id", itemID, "quantity", req.Quantity)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	stock, err = h.repo.GetStock(r.Context(), itemID)
	if err != nil {
		h.logger.Error("failed to get updated stock", "error", err, "item_id", itemID)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.logger.Info("stock reserved", "item_id", itemID, "quantity", req.Quantity)
	h.writeJSON(w, http.StatusOK, stock)
}

type releaseRequest struct {
	Quantity int `json:"quantity"`
}

func (h *Handler) HandleRelease(w http.ResponseWriter, r *http.Request) {
	itemID := r.PathValue("itemId")
	if itemID == "" {
		h.writeError(w, http.StatusBadRequest, "missing item id")
		return
	}

	var req releaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if err := h.repo.Release(r.Context(), itemID, req.Quantity); err != nil {
		h.logger.Error("failed to release stock", "error", err, "item_id", itemID, "quantity", req.Quantity)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	stock, err := h.repo.GetStock(r.Context(), itemID)
	if err != nil {
		h.logger.Error("failed to get updated stock", "error", err, "item_id", itemID)
		h.writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}

	h.logger.Info("stock released", "item_id", itemID, "quantity", req.Quantity)
	h.writeJSON(w, http.StatusOK, stock)
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

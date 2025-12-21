package gateway

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type Handler struct {
	ordersProxy    *ServiceProxy
	inventoryProxy *ServiceProxy
	logger         *slog.Logger
}

func NewHandler(ordersProxy, inventoryProxy *ServiceProxy, logger *slog.Logger) *Handler {
	return &Handler{
		ordersProxy:    ordersProxy,
		inventoryProxy: inventoryProxy,
		logger:         logger,
	}
}

func (h *Handler) HandleOrders(w http.ResponseWriter, r *http.Request) {
	h.proxyRequest(w, r, h.ordersProxy, r.URL.Path)
}

func (h *Handler) HandleInventory(w http.ResponseWriter, r *http.Request) {
	path := strings.Replace(r.URL.Path, "/inventory/", "/stock/", 1)
	h.proxyRequest(w, r, h.inventoryProxy, path)
}

func (h *Handler) proxyRequest(w http.ResponseWriter, r *http.Request, proxy *ServiceProxy, path string) {
	resp, err := proxy.ForwardRequest(r.Context(), r, path)
	if err != nil {
		h.logger.Error("failed to forward request", "error", err, "path", path)
		h.writeError(w, http.StatusBadGateway, "service unavailable")
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if contentType := resp.Header.Get("Content-Type"); contentType != "" {
		w.Header().Set("Content-Type", contentType)
	}

	w.WriteHeader(resp.StatusCode)

	h.logger.Info("request proxied", "method", r.Method, "path", path, "status", resp.StatusCode)

	if _, err := io.Copy(w, resp.Body); err != nil {
		h.logger.Error("failed to copy response body", "error", err)
	}
}

func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"error": message}); err != nil {
		h.logger.Error("failed to encode error response", "error", err)
	}
}

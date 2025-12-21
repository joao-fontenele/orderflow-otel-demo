package email

import (
	"encoding/json"
	"log/slog"
	"math/rand"
	"net/http"
	"time"
)

type Handler struct {
	logger *slog.Logger
}

func NewHandler(logger *slog.Logger) *Handler {
	return &Handler{
		logger: logger,
	}
}

type sendRequest struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
	Body    string `json:"body"`
}

type sendResponse struct {
	Status string `json:"status"`
}

func (h *Handler) HandleSend(w http.ResponseWriter, r *http.Request) {
	var req sendRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	delay := time.Duration(50+rand.Intn(151)) * time.Millisecond
	time.Sleep(delay)

	h.logger.Info("email sent", "to", req.To, "subject", req.Subject)

	h.writeJSON(w, http.StatusOK, sendResponse{Status: "sent"})
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

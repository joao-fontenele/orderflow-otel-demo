package gateway

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandler_HandleOrders(t *testing.T) {
	t.Run("proxies GET /orders", func(t *testing.T) {
		ordersServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/orders" {
				t.Errorf("expected /orders, got %s", r.URL.Path)
			}
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`[{"id":"1"}]`))
		}))
		defer ordersServer.Close()

		handler := NewHandler(
			NewServiceProxy(ordersServer.URL, ordersServer.Client()),
			NewServiceProxy("http://unused", http.DefaultClient),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		req := httptest.NewRequest(http.MethodGet, "/orders", nil)
		rec := httptest.NewRecorder()

		handler.HandleOrders(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
		if rec.Header().Get("Content-Type") != "application/json" {
			t.Errorf("expected application/json, got %s", rec.Header().Get("Content-Type"))
		}
		if rec.Body.String() != `[{"id":"1"}]` {
			t.Errorf("unexpected body: %s", rec.Body.String())
		}
	})

	t.Run("proxies POST /orders with body", func(t *testing.T) {
		ordersServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"customer_id":"123"}` {
				t.Errorf("unexpected body: %s", body)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":"new-id"}`))
		}))
		defer ordersServer.Close()

		handler := NewHandler(
			NewServiceProxy(ordersServer.URL, ordersServer.Client()),
			NewServiceProxy("http://unused", http.DefaultClient),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(`{"customer_id":"123"}`))
		rec := httptest.NewRecorder()

		handler.HandleOrders(rec, req)

		if rec.Code != http.StatusCreated {
			t.Errorf("expected status 201, got %d", rec.Code)
		}
	})

	t.Run("returns 502 when orders service unavailable", func(t *testing.T) {
		handler := NewHandler(
			NewServiceProxy("http://localhost:99999", &http.Client{}),
			NewServiceProxy("http://unused", http.DefaultClient),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		req := httptest.NewRequest(http.MethodGet, "/orders", nil)
		rec := httptest.NewRecorder()

		handler.HandleOrders(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Errorf("expected status 502, got %d", rec.Code)
		}

		var resp map[string]string
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["error"] != "service unavailable" {
			t.Errorf("expected 'service unavailable', got %s", resp["error"])
		}
	})
}

func TestHandler_HandleInventory(t *testing.T) {
	t.Run("strips /inventory prefix and forwards to inventory service", func(t *testing.T) {
		inventoryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/stock/item-123" {
				t.Errorf("expected /stock/item-123, got %s", r.URL.Path)
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"item_id":"item-123","quantity":10}`))
		}))
		defer inventoryServer.Close()

		handler := NewHandler(
			NewServiceProxy("http://unused", http.DefaultClient),
			NewServiceProxy(inventoryServer.URL, inventoryServer.Client()),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		req := httptest.NewRequest(http.MethodGet, "/inventory/stock/item-123", nil)
		rec := httptest.NewRecorder()

		handler.HandleInventory(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}
	})

	t.Run("preserves downstream error status", func(t *testing.T) {
		inventoryServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":"item not found"}`))
		}))
		defer inventoryServer.Close()

		handler := NewHandler(
			NewServiceProxy("http://unused", http.DefaultClient),
			NewServiceProxy(inventoryServer.URL, inventoryServer.Client()),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		req := httptest.NewRequest(http.MethodGet, "/inventory/stock/unknown", nil)
		rec := httptest.NewRecorder()

		handler.HandleInventory(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("expected status 404, got %d", rec.Code)
		}
	})

	t.Run("returns 502 when inventory service unavailable", func(t *testing.T) {
		handler := NewHandler(
			NewServiceProxy("http://unused", http.DefaultClient),
			NewServiceProxy("http://localhost:99999", &http.Client{}),
			slog.New(slog.NewTextHandler(io.Discard, nil)),
		)

		req := httptest.NewRequest(http.MethodGet, "/inventory/stock/item-123", nil)
		rec := httptest.NewRecorder()

		handler.HandleInventory(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Errorf("expected status 502, got %d", rec.Code)
		}
	})
}

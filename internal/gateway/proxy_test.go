package gateway

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestServiceProxy_ForwardRequest(t *testing.T) {
	t.Run("forwards GET request", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}
			if r.URL.Path != "/test" {
				t.Errorf("expected /test, got %s", r.URL.Path)
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		}))
		defer server.Close()

		proxy := NewServiceProxy(server.URL, server.Client())
		req := httptest.NewRequest(http.MethodGet, "/original", nil)
		resp, err := proxy.ForwardRequest(context.Background(), req, "/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
	})

	t.Run("forwards POST request with body and content-type", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}
			if r.Header.Get("Content-Type") != "application/json" {
				t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"data":"test"}` {
				t.Errorf("unexpected body: %s", body)
			}
			w.WriteHeader(http.StatusCreated)
		}))
		defer server.Close()

		proxy := NewServiceProxy(server.URL, server.Client())
		req := httptest.NewRequest(http.MethodPost, "/original", strings.NewReader(`{"data":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		resp, err := proxy.ForwardRequest(context.Background(), req, "/create")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected status 201, got %d", resp.StatusCode)
		}
	})

	t.Run("respects context cancellation", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		proxy := NewServiceProxy(server.URL, server.Client())
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		req := httptest.NewRequest(http.MethodGet, "/original", nil)
		_, err := proxy.ForwardRequest(ctx, req, "/test")
		if err == nil {
			t.Error("expected error for cancelled context")
		}
	})
}

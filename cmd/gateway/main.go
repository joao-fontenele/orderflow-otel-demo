package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/gateway"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	ordersServiceURL := os.Getenv("ORDERS_SERVICE_URL")
	if ordersServiceURL == "" {
		logger.Error("ORDERS_SERVICE_URL is required")
		os.Exit(1)
	}

	inventoryServiceURL := os.Getenv("INVENTORY_SERVICE_URL")
	if inventoryServiceURL == "" {
		logger.Error("INVENTORY_SERVICE_URL is required")
		os.Exit(1)
	}

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	ordersProxy := gateway.NewServiceProxy(ordersServiceURL, httpClient)
	inventoryProxy := gateway.NewServiceProxy(inventoryServiceURL, httpClient)
	handler := gateway.NewHandler(ordersProxy, inventoryProxy, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /orders", handler.HandleOrders)
	mux.HandleFunc("POST /orders", handler.HandleOrders)
	mux.HandleFunc("GET /orders/{id}", handler.HandleOrders)
	mux.HandleFunc("PATCH /orders/{id}/status", handler.HandleOrders)
	mux.HandleFunc("GET /inventory/stock", handler.HandleInventory)
	mux.HandleFunc("GET /inventory/stock/{itemId}", handler.HandleInventory)
	mux.HandleFunc("POST /inventory/stock/{itemId}/reserve", handler.HandleInventory)
	mux.HandleFunc("POST /inventory/stock/{itemId}/release", handler.HandleInventory)

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("starting gateway service", "port", port)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	logger.Info("shutting down")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", "error", err)
		os.Exit(1)
	}
}

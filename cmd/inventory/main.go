package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/inventory"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		logger.Error("POSTGRES_URL environment variable is required")
		os.Exit(1)
	}

	db, err := sql.Open("postgres", postgresURL)
	if err != nil {
		logger.Error("failed to open database connection", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	if _, err := db.Exec("SET search_path TO inventory"); err != nil {
		logger.Error("failed to set search_path", "error", err)
		os.Exit(1)
	}

	repo := inventory.NewInventoryRepository(db)
	handler := inventory.NewHandler(repo, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /stock", handler.HandleListStock)
	mux.HandleFunc("GET /stock/{itemId}", handler.HandleGetStock)
	mux.HandleFunc("POST /stock/{itemId}/reserve", handler.HandleReserve)
	mux.HandleFunc("POST /stock/{itemId}/release", handler.HandleRelease)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("starting inventory service", "port", port)
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

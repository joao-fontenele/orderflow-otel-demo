package main

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	_ "github.com/lib/pq"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/messaging"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/orders"
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
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	if err := db.Ping(); err != nil {
		logger.Error("failed to connect to database", "error", err)
		os.Exit(1)
	}

	if _, err := db.Exec("SET search_path TO orders"); err != nil {
		logger.Error("failed to set search_path", "error", err)
		os.Exit(1)
	}

	var producer *messaging.Producer
	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers != "" {
		brokers := strings.Split(kafkaBrokers, ",")
		producer = messaging.NewProducer(brokers, "order.created")
		defer func() { _ = producer.Close() }()
	}

	repo := orders.NewOrderRepository(db)
	handler := orders.NewHandler(repo, producer, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/orders", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.HandleList(w, r)
		case http.MethodPost:
			handler.HandleCreate(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/orders/", func(w http.ResponseWriter, r *http.Request) {
		pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		hasStatusSuffix := len(pathParts) == 3 && pathParts[2] == "status"

		switch {
		case r.Method == http.MethodGet && !hasStatusSuffix:
			handler.HandleGet(w, r)
		case r.Method == http.MethodPatch && hasStatusSuffix:
			handler.HandleUpdateStatus(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	server := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("starting orders service", "port", port)
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

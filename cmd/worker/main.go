package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/messaging"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/worker"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	kafkaBrokers := os.Getenv("KAFKA_BROKERS")
	if kafkaBrokers == "" {
		logger.Error("KAFKA_BROKERS environment variable is required")
		os.Exit(1)
	}

	emailServiceURL := os.Getenv("EMAIL_SERVICE_URL")
	if emailServiceURL == "" {
		logger.Error("EMAIL_SERVICE_URL environment variable is required")
		os.Exit(1)
	}

	ordersServiceURL := os.Getenv("ORDERS_SERVICE_URL")
	if ordersServiceURL == "" {
		logger.Error("ORDERS_SERVICE_URL environment variable is required")
		os.Exit(1)
	}

	inventoryServiceURL := os.Getenv("INVENTORY_SERVICE_URL")
	if inventoryServiceURL == "" {
		logger.Error("INVENTORY_SERVICE_URL environment variable is required")
		os.Exit(1)
	}

	brokers := strings.Split(kafkaBrokers, ",")
	consumer := messaging.NewConsumer(brokers, "order.created", "notification-worker")
	defer func() { _ = consumer.Close() }()

	httpClient := &http.Client{
		Timeout: 10 * time.Second,
	}

	notificationHandler := worker.NewNotificationHandler(emailServiceURL, ordersServiceURL, inventoryServiceURL, httpClient, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		logger.Info("shutting down")
		cancel()
	}()

	logger.Info("starting notification worker", "brokers", brokers)

	if err := consumer.Consume(ctx, notificationHandler.Handle); err != nil {
		if ctx.Err() == context.Canceled {
			logger.Info("consumer stopped")
			return
		}
		logger.Error("consumer error", "error", err)
		os.Exit(1)
	}
}

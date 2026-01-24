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

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/messaging"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/telemetry"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/worker"
)

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	shutdownTracer, err := telemetry.InitTracerProvider(ctx, "worker", "0.1.0")
	if err != nil {
		logger.Error("failed to initialize tracer", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	metricsHandler, shutdownMeter, err := telemetry.InitMeterProvider("worker", "0.1.0")
	if err != nil {
		logger.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownMeter(ctx) }()

	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		logger.Error("failed to start runtime metrics", "error", err)
	}

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
		Timeout:   10 * time.Second,
		Transport: otelhttp.NewTransport(http.DefaultTransport),
	}

	notificationHandler := worker.NewNotificationHandler(emailServiceURL, ordersServiceURL, inventoryServiceURL, httpClient, logger)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8083"
	}
	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metricsHandler)

	metricsServer := &http.Server{
		Addr:         ":" + port,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("starting metrics server", "port", port)
		if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("metrics server error", "error", err)
		}
	}()

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		stop := make(chan os.Signal, 1)
		signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
		<-stop
		logger.Info("shutting down")

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("metrics server shutdown error", "error", err)
		}

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

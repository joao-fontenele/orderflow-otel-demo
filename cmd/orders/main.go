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

	_ "github.com/lib/pq"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/contrib/instrumentation/runtime"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/messaging"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/orders"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/telemetry"
)

func main() {
	ctx := context.Background()
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	shutdownTracer, err := telemetry.InitTracerProvider(ctx, "orders", "0.1.0")
	if err != nil {
		logger.Error("failed to initialize tracer", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownTracer(ctx) }()

	metricsHandler, shutdownMeter, err := telemetry.InitMeterProvider("orders", "0.1.0")
	if err != nil {
		logger.Error("failed to initialize metrics", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdownMeter(ctx) }()

	if err := runtime.Start(runtime.WithMinimumReadMemStatsInterval(time.Second)); err != nil {
		logger.Error("failed to start runtime metrics", "error", err)
	}

	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		logger.Error("POSTGRES_URL environment variable is required")
		os.Exit(1)
	}

	db, err := telemetry.OpenDB("postgres", postgresURL)
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
	handler, err := orders.NewHandler(repo, producer, logger)
	if err != nil {
		logger.Error("failed to create handler", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()
	mux.Handle("GET /metrics", metricsHandler)

	mux.HandleFunc("GET /orders", telemetry.WithHTTPRoute(handler.HandleList))
	mux.HandleFunc("GET /orders-nplus1", telemetry.WithHTTPRoute(handler.HandleListNPlus1))
	mux.HandleFunc("POST /orders", telemetry.WithHTTPRoute(handler.HandleCreate))
	mux.HandleFunc("GET /orders/{id}", telemetry.WithHTTPRoute(handler.HandleGet))
	mux.HandleFunc("PATCH /orders/{id}/status", telemetry.WithHTTPRoute(handler.HandleUpdateStatus))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	server := &http.Server{
		Addr: ":" + port,
		Handler: otelhttp.NewHandler(mux, "orders",
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				if r.Pattern != "" {
					return r.Pattern
				}
				return r.Method + " " + r.URL.Path
			}),
		),
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

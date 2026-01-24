package telemetry

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/prometheus"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

// InitMeterProvider initializes the Prometheus exporter and MeterProvider.
// It returns an http.Handler for the /metrics endpoint and a shutdown function.
func InitMeterProvider(serviceName, serviceVersion string) (http.Handler, func(context.Context) error, error) {
	// 1. Create the Prometheus exporter
	exporter, err := prometheus.New()
	if err != nil {
		return nil, nil, err
	}

	// 2. Define the Resource (same as TracerProvider)
	res := resource.NewWithAttributes(
		semconv.SchemaURL,
		semconv.ServiceName(serviceName),
		semconv.ServiceVersion(serviceVersion),
	)

	// 3. Create the MeterProvider
	mp := metric.NewMeterProvider(
		metric.WithReader(exporter),
		metric.WithResource(res),
	)

	// 4. Set Global MeterProvider
	otel.SetMeterProvider(mp)

	return promhttp.Handler(), mp.Shutdown, nil
}

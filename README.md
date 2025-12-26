# orderflow-otel-demo

A sample microservices application demonstrating OpenTelemetry instrumentation patterns. This project serves as a learning resource for an OpenTelemetry tutorial series.

## Architecture

```
┌──────────┐     ┌──────────┐     ┌───────────┐
│ Gateway  │────▶│ Orders   │────▶│ Inventory │
│ :8080    │     │ :8081    │     │ :8082     │
└──────────┘     └──────────┘     └───────────┘
                      │                 │
                      └────────┬────────┘
                               ▼
                          PostgreSQL
                               │
                               ▼
                            Kafka
                               │
                               ▼
                   ┌───────────────────────┐
                   │  Notification Worker  │
                   └───────────────────────┘
                         │           │
                         ▼           ▼
              ┌──────────────┐  ┌─────────┐
              │ Email Service│  │ Orders  │
              │ :8084        │  │ (status)│
              └──────────────┘  └─────────┘
```

### Services

| Service   | Port | Description                                     |
|-----------|------|-------------------------------------------------|
| gateway   | 8080 | API gateway routing requests to backend services|
| orders    | 8081 | Manages order creation and lifecycle            |
| inventory | 8082 | Handles inventory and stock reservations        |
| worker    | 8083 | Background worker for async event processing    |
| email     | 8084 | Email notification service (simulated)          |

## Quick Start

### Prerequisites

- Go 1.25+
- Docker and Docker Compose
- Make

### Running with Docker Compose

Start all infrastructure and services:

```bash
make all-up
```

This starts PostgreSQL, Kafka, all microservices, and observability stack (Jaeger, Prometheus, Grafana).

For development (infrastructure only):

```bash
make docker-up
```

Then run migrations and services locally:

```bash
make migrate-up
make run-gateway   # Terminal 1
make run-orders    # Terminal 2
make run-inventory # Terminal 3
make run-worker    # Terminal 4
make run-email     # Terminal 5
```

### Generate Traffic

```bash
./scripts/generate-traffic.sh
```

Options:

```bash
GATEWAY_URL=http://localhost:8080 ORDER_COUNT=20 SLEEP_SECONDS=2 ./scripts/generate-traffic.sh
```

## API Reference

### Gateway Endpoints (Public)

| Method | Endpoint              | Description                              |
|--------|-----------------------|------------------------------------------|
| GET    | /orders               | List all orders                          |
| GET    | /orders-nplus1        | List all orders (N+1 query demo)         |
| GET    | /orders/{id}          | Get order by ID                          |
| POST   | /orders               | Create a new order                       |
| GET    | /inventory/{itemId}   | Get inventory level                      |

### Orders Service (Internal)

| Method | Endpoint              | Description                              |
|--------|-----------------------|------------------------------------------|
| GET    | /orders               | List all orders                          |
| GET    | /orders-nplus1        | List all orders (N+1 query for tracing)  |
| GET    | /orders/{id}          | Get order by ID                          |
| POST   | /orders               | Create order (publishes to Kafka)        |
| PATCH  | /orders/{id}/status   | Update order status                      |

### Inventory Service (Internal)

| Method | Endpoint                   | Description        |
|--------|----------------------------|--------------------|
| GET    | /stock                     | List all stock     |
| GET    | /stock/{itemId}            | Get stock level    |
| POST   | /stock/{itemId}/reserve    | Reserve stock      |
| POST   | /stock/{itemId}/release    | Release reservation|

### Example Requests

Create an order:

```bash
curl -X POST http://localhost:8080/orders \
  -H "Content-Type: application/json" \
  -d '{
    "customer_id": "customer-123",
    "items": [
      {"item_id": "ITEM-001", "quantity": 2, "price": 2999}
    ]
  }'
```

Check inventory:

```bash
curl http://localhost:8080/inventory/ITEM-001
```

Reserve stock:

```bash
curl -X POST http://localhost:8082/stock/ITEM-001/reserve \
  -H "Content-Type: application/json" \
  -d '{"quantity": 5}'
```

## Environment Variables

### All Services

| Variable       | Description                | Default |
|----------------|----------------------------|---------|
| PORT           | HTTP listen port           | varies  |

### Orders Service

| Variable       | Description                | Default |
|----------------|----------------------------|---------|
| POSTGRES_URL   | PostgreSQL connection URL  | -       |
| KAFKA_BROKERS  | Comma-separated broker list| -       |

### Inventory Service

| Variable       | Description                | Default |
|----------------|----------------------------|---------|
| POSTGRES_URL   | PostgreSQL connection URL  | -       |

### Gateway Service

| Variable              | Description             | Default |
|-----------------------|-------------------------|---------|
| ORDERS_SERVICE_URL    | Orders service base URL | -       |
| INVENTORY_SERVICE_URL | Inventory service URL   | -       |

### Worker Service

| Variable              | Description                | Default |
|-----------------------|----------------------------|---------|
| KAFKA_BROKERS         | Comma-separated broker list| -       |
| EMAIL_SERVICE_URL     | Email service base URL     | -       |
| ORDERS_SERVICE_URL    | Orders service base URL    | -       |
| INVENTORY_SERVICE_URL | Inventory service URL      | -       |

### Migration Tool

| Variable         | Description                | Default            |
|------------------|----------------------------|--------------------|
| POSTGRES_URL     | PostgreSQL connection URL  | -                  |
| MIGRATIONS_PATH  | Path to migration files    | file://migrations  |

## Development

### Build

```bash
make build
```

### Run Tests

Unit tests:

```bash
make test
```

Integration tests (requires Docker):

```bash
go test -tags=integration -v ./test/...
```

### Lint and Format

```bash
make lint
make format
```

### Database Migrations

Run migrations:

```bash
make migrate-up
```

Rollback:

```bash
make migrate-down
```

Check current version:

```bash
make migrate-version
```

Create new migration:

```bash
make migrate-create name=add_new_table
```

## Docker Compose Profiles

| Command           | Description                                      |
|-------------------|--------------------------------------------------|
| make docker-up    | Infrastructure only (Postgres, Kafka)            |
| make docker-up-all| Infrastructure + all services                    |
| make otel-up      | Infrastructure + observability (Jaeger, etc.)    |
| make all-up       | Everything: infra + services + observability     |

## Observability Stack

When running with `make all-up` or `make otel-up`:

| Service    | URL                      | Description         |
|------------|--------------------------|---------------------|
| Jaeger     | http://localhost:16686   | Distributed tracing |
| Prometheus | http://localhost:9090    | Metrics             |
| Grafana    | http://localhost:3000    | Dashboards          |

## Project Structure

```
orderflow-otel-demo/
├── cmd/
│   ├── gateway/       # API gateway entry point
│   ├── orders/        # Orders service entry point
│   ├── inventory/     # Inventory service entry point
│   ├── worker/        # Background worker entry point
│   ├── email/         # Email service entry point
│   └── migrate/       # Database migration tool
├── internal/
│   ├── domain/        # Shared domain types
│   ├── gateway/       # Gateway handlers and proxy
│   ├── orders/        # Orders business logic
│   ├── inventory/     # Inventory business logic
│   ├── worker/        # Worker event handlers
│   ├── email/         # Email service handlers
│   └── messaging/     # Kafka producer/consumer
├── migrations/        # SQL migration files
├── scripts/           # Utility scripts
├── test/              # Integration tests
└── docker-compose*.yml
```

## License

MIT

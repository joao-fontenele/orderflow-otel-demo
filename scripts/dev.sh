#!/bin/bash
set -euo pipefail

cleanup() {
  echo ""
  echo "Shutting down services..."
  kill 0
}

trap cleanup SIGINT SIGTERM

echo "Starting services with hot-reload..."
echo "  Gateway:   http://localhost:8080"
echo "  Orders:    http://localhost:8081"
echo "  Inventory: http://localhost:8082"
echo "  Email:     http://localhost:8084"
echo ""
echo "Press Ctrl+C to stop all services"
echo ""

PORT=8084 go tool reflex -r '\.go$' -s -- go run ./cmd/email &
PORT=8082 go tool reflex -r '\.go$' -s -- go run ./cmd/inventory &
PORT=8081 go tool reflex -r '\.go$' -s -- go run ./cmd/orders &
PORT=8080 go tool reflex -r '\.go$' -s -- go run ./cmd/gateway &

wait

.PHONY: build dev run-gateway run-orders run-inventory run-worker run-email test test-integration lint format vulncheck clean migrate-up migrate-down migrate-version migrate-create docker-up docker-up-all docker-down docker-logs docker-build otel-up otel-down all-up all-down

GOLANGCI_LINT_VERSION := v2.7.2
GOLANGCI_LINT := ./bin/golangci-lint



build:
	@mkdir -p ./bin
	go build -o ./bin/gateway ./cmd/gateway
	go build -o ./bin/orders ./cmd/orders
	go build -o ./bin/inventory ./cmd/inventory
	go build -o ./bin/worker ./cmd/worker
	go build -o ./bin/email ./cmd/email
	go build -o ./bin/migrate ./cmd/migrate

dev:
	./scripts/dev.sh

run-gateway:
	go tool reflex -r '\.go$$' -s -- go run ./cmd/gateway

run-orders:
	go tool reflex -r '\.go$$' -s -- go run ./cmd/orders

run-inventory:
	go tool reflex -r '\.go$$' -s -- go run ./cmd/inventory

run-worker:
	go tool reflex -r '\.go$$' -s -- go run ./cmd/worker

run-email:
	go tool reflex -r '\.go$$' -s -- go run ./cmd/email

test:
	go test -v -race -shuffle=on ./...

test-integration:
	go test -tags=integration -v -race -shuffle=on ./...

$(GOLANGCI_LINT):
	@mkdir -p ./bin
	curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b ./bin $(GOLANGCI_LINT_VERSION)

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run ./...

vulncheck:
	go tool govulncheck ./...

format:
	go mod tidy
	go tool goimports -w .

clean:
	rm -rf ./bin

POSTGRES_URL ?= postgres://orderflow:orderflow@localhost:5432/orderflow?sslmode=disable

migrate-up:
	POSTGRES_URL=$(POSTGRES_URL) go run ./cmd/migrate up

migrate-down:
	POSTGRES_URL=$(POSTGRES_URL) go run ./cmd/migrate down

migrate-version:
	POSTGRES_URL=$(POSTGRES_URL) go run ./cmd/migrate version

migrate-create:
ifndef name
	$(error name is required: make migrate-create name=xxx)
endif
	@touch migrations/$$(printf "%06d" $$(($$(ls -1 migrations/*.up.sql 2>/dev/null | wc -l) + 1)))_$(name).up.sql
	@touch migrations/$$(printf "%06d" $$(($$(ls -1 migrations/*.up.sql 2>/dev/null | wc -l))))_$(name).down.sql
	@echo "Created migration files for $(name)"

docker-up:
	docker compose up -d

docker-up-all:
	docker compose -f docker-compose.yml -f docker-compose.services.yml up -d

docker-down:
	docker compose -f docker-compose.yml -f docker-compose.services.yml down

docker-logs:
	docker compose -f docker-compose.yml -f docker-compose.services.yml logs -f

docker-build:
	docker compose -f docker-compose.yml -f docker-compose.services.yml build

otel-up:
	docker compose -f docker-compose.yml -f docker-compose.otel.yml up -d

otel-down:
	docker compose -f docker-compose.yml -f docker-compose.otel.yml down

all-up:
	docker compose -f docker-compose.yml -f docker-compose.services.yml -f docker-compose.otel.yml up -d

all-down:
	docker compose -f docker-compose.yml -f docker-compose.services.yml -f docker-compose.otel.yml down

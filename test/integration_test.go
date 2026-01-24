//go:build integration

package test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/joao-fontenele/orderflow-otel-demo/internal/domain"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/inventory"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/orders"
	"github.com/joao-fontenele/orderflow-otel-demo/internal/worker"
)

func TestOrderCreationFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := SetupPostgres(ctx, t)
	defer pg.Cleanup()

	ordersDB, err := DBWithSchema(pg.ConnStr, "orders")
	if err != nil {
		t.Fatalf("failed to create orders DB: %v", err)
	}
	defer func() { _ = ordersDB.Close() }()

	repo := orders.NewOrderRepository(ordersDB)
	logger := slog.Default()
	handler, err := orders.NewHandler(repo, nil, logger)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	reqBody := `{"customer_id": "test-customer-1", "items": [{"item_id": "ITEM-001", "quantity": 2, "price": 1000}]}`
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var createdOrder domain.Order
	if err := json.NewDecoder(rec.Body).Decode(&createdOrder); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if createdOrder.ID == "" {
		t.Fatal("expected order ID to be set")
	}
	if createdOrder.CustomerID != "test-customer-1" {
		t.Fatalf("expected customer_id 'test-customer-1', got '%s'", createdOrder.CustomerID)
	}
	if createdOrder.Status != domain.OrderStatusPending {
		t.Fatalf("expected status '%s', got '%s'", domain.OrderStatusPending, createdOrder.Status)
	}
	if createdOrder.Total != 2000 {
		t.Fatalf("expected total 2000, got %d", createdOrder.Total)
	}

	fetchedOrder, err := repo.GetByID(ctx, createdOrder.ID)
	if err != nil {
		t.Fatalf("failed to fetch order from DB: %v", err)
	}
	if fetchedOrder == nil {
		t.Fatal("order not found in database")
	}
	if fetchedOrder.CustomerID != createdOrder.CustomerID {
		t.Fatalf("DB order customer_id mismatch: expected '%s', got '%s'", createdOrder.CustomerID, fetchedOrder.CustomerID)
	}
}

func TestInventoryReserve(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := SetupPostgres(ctx, t)
	defer pg.Cleanup()

	inventoryDB, err := DBWithSchema(pg.ConnStr, "inventory")
	if err != nil {
		t.Fatalf("failed to create inventory DB: %v", err)
	}
	defer func() { _ = inventoryDB.Close() }()

	repo := inventory.NewInventoryRepository(inventoryDB)
	logger := slog.Default()
	handler := inventory.NewHandler(repo, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /stock/{itemId}", handler.HandleGetStock)
	mux.HandleFunc("POST /stock/{itemId}/reserve", handler.HandleReserve)

	req := httptest.NewRequest(http.MethodGet, "/stock/ITEM-001", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var initialStock domain.StockLevel
	if err := json.NewDecoder(rec.Body).Decode(&initialStock); err != nil {
		t.Fatalf("failed to decode initial stock: %v", err)
	}

	if initialStock.Available != 100 {
		t.Fatalf("expected initial available stock 100, got %d", initialStock.Available)
	}
	if initialStock.Reserved != 0 {
		t.Fatalf("expected initial reserved stock 0, got %d", initialStock.Reserved)
	}

	reserveBody := `{"quantity": 10}`
	req = httptest.NewRequest(http.MethodPost, "/stock/ITEM-001/reserve", strings.NewReader(reserveBody))
	req.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var updatedStock domain.StockLevel
	if err := json.NewDecoder(rec.Body).Decode(&updatedStock); err != nil {
		t.Fatalf("failed to decode updated stock: %v", err)
	}

	if updatedStock.Available != 90 {
		t.Fatalf("expected available stock 90 after reserve, got %d", updatedStock.Available)
	}
	if updatedStock.Reserved != 10 {
		t.Fatalf("expected reserved stock 10 after reserve, got %d", updatedStock.Reserved)
	}
}

func TestListOrders(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pg := SetupPostgres(ctx, t)
	defer pg.Cleanup()

	ordersDB, err := DBWithSchema(pg.ConnStr, "orders")
	if err != nil {
		t.Fatalf("failed to create orders DB: %v", err)
	}
	defer func() { _ = ordersDB.Close() }()

	repo := orders.NewOrderRepository(ordersDB)
	logger := slog.Default()
	handler, err := orders.NewHandler(repo, nil, logger)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}

	for i := 1; i <= 3; i++ {
		order := &domain.Order{
			CustomerID: "list-test-customer",
			Items: []domain.OrderItem{
				{ItemID: "ITEM-001", Quantity: 1, Price: 1000},
			},
			Total:     1000,
			Status:    domain.OrderStatusPending,
			CreatedAt: time.Now().UTC(),
		}
		if err := repo.Create(ctx, order); err != nil {
			t.Fatalf("failed to create order %d: %v", i, err)
		}
	}

	req := httptest.NewRequest(http.MethodGet, "/orders", nil)
	rec := httptest.NewRecorder()

	handler.HandleList(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, rec.Code, rec.Body.String())
	}

	var orderList []domain.Order
	if err := json.NewDecoder(rec.Body).Decode(&orderList); err != nil {
		t.Fatalf("failed to decode order list: %v", err)
	}

	if len(orderList) != 3 {
		t.Fatalf("expected 3 orders, got %d", len(orderList))
	}

	for _, order := range orderList {
		if order.CustomerID != "list-test-customer" {
			t.Fatalf("unexpected customer_id: %s", order.CustomerID)
		}
	}
}

func TestKafkaConnection(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	brokers, cleanup := SetupKafka(ctx, t)
	defer cleanup()

	if len(brokers) == 0 {
		t.Fatal("expected at least one broker")
	}

	t.Logf("kafka brokers: %v", brokers)
}

type emailCapture struct {
	mu     sync.Mutex
	emails []map[string]string
}

func (e *emailCapture) handler(w http.ResponseWriter, r *http.Request) {
	var req map[string]string
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	e.mu.Lock()
	e.emails = append(e.emails, req)
	e.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, `{"status":"sent"}`)
}

func (e *emailCapture) getEmails() []map[string]string {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]map[string]string, len(e.emails))
	copy(result, e.emails)
	return result
}

func TestOrderFlowWithSufficientStock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	pg := SetupPostgres(ctx, t)
	defer pg.Cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ordersDB, err := DBWithSchema(pg.ConnStr, "orders")
	if err != nil {
		t.Fatalf("failed to create orders DB: %v", err)
	}
	defer func() { _ = ordersDB.Close() }()

	ordersRepo := orders.NewOrderRepository(ordersDB)
	ordersHandler, err := orders.NewHandler(ordersRepo, nil, logger)
	if err != nil {
		t.Fatalf("failed to create orders handler: %v", err)
	}
	ordersMux := http.NewServeMux()
	ordersMux.HandleFunc("POST /orders", ordersHandler.HandleCreate)
	ordersMux.HandleFunc("GET /orders/{id}", ordersHandler.HandleGet)
	ordersMux.HandleFunc("PATCH /orders/{id}/status", ordersHandler.HandleUpdateStatus)
	ordersServer := httptest.NewServer(ordersMux)
	defer ordersServer.Close()

	inventoryDB, err := DBWithSchema(pg.ConnStr, "inventory")
	if err != nil {
		t.Fatalf("failed to create inventory DB: %v", err)
	}
	defer func() { _ = inventoryDB.Close() }()

	inventoryRepo := inventory.NewInventoryRepository(inventoryDB)
	inventoryHandler := inventory.NewHandler(inventoryRepo, logger)
	inventoryMux := http.NewServeMux()
	inventoryMux.HandleFunc("GET /stock/{itemId}", inventoryHandler.HandleGetStock)
	inventoryMux.HandleFunc("POST /stock/{itemId}/reserve", inventoryHandler.HandleReserve)
	inventoryMux.HandleFunc("POST /stock/{itemId}/release", inventoryHandler.HandleRelease)
	inventoryServer := httptest.NewServer(inventoryMux)
	defer inventoryServer.Close()

	emailCap := &emailCapture{}
	emailMux := http.NewServeMux()
	emailMux.HandleFunc("POST /send", emailCap.handler)
	emailServer := httptest.NewServer(emailMux)
	defer emailServer.Close()

	httpClient := &http.Client{Timeout: 10 * time.Second}
	notificationHandler := worker.NewNotificationHandler(
		emailServer.URL,
		ordersServer.URL,
		inventoryServer.URL,
		httpClient,
		logger,
	)

	initialStock, err := inventoryRepo.GetStock(ctx, "ITEM-001")
	if err != nil {
		t.Fatalf("failed to get initial stock: %v", err)
	}
	initialAvailable := initialStock.Available

	reqBody := `{"customer_id": "cust-123", "items": [{"item_id": "ITEM-001", "quantity": 5, "price": 1000}]}`
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ordersHandler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var createdOrder domain.Order
	if err := json.NewDecoder(rec.Body).Decode(&createdOrder); err != nil {
		t.Fatalf("failed to decode order: %v", err)
	}

	event := domain.OrderCreatedEvent{
		OrderID:    createdOrder.ID,
		CustomerID: createdOrder.CustomerID,
		Items:      createdOrder.Items,
		Timestamp:  createdOrder.CreatedAt,
	}
	eventPayload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	if err := notificationHandler.Handle(ctx, eventPayload); err != nil {
		t.Fatalf("worker handler failed: %v", err)
	}

	finalOrder, err := ordersRepo.GetByID(ctx, createdOrder.ID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}

	if finalOrder.Status != domain.OrderStatusConfirmed {
		t.Fatalf("expected order status %s, got %s", domain.OrderStatusConfirmed, finalOrder.Status)
	}

	finalStock, err := inventoryRepo.GetStock(ctx, "ITEM-001")
	if err != nil {
		t.Fatalf("failed to get final stock: %v", err)
	}

	expectedAvailable := initialAvailable - 5
	if finalStock.Available != expectedAvailable {
		t.Fatalf("expected available stock %d, got %d", expectedAvailable, finalStock.Available)
	}
	if finalStock.Reserved != 5 {
		t.Fatalf("expected reserved stock 5, got %d", finalStock.Reserved)
	}

	emails := emailCap.getEmails()
	if len(emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(emails))
	}

	email := emails[0]
	if !strings.Contains(email["subject"], "Confirmation") {
		t.Fatalf("expected confirmation email, got subject: %s", email["subject"])
	}
	if !strings.Contains(email["subject"], createdOrder.ID) {
		t.Fatalf("expected email subject to contain order ID %s, got: %s", createdOrder.ID, email["subject"])
	}
}

func TestOrderFlowWithInsufficientStock(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	pg := SetupPostgres(ctx, t)
	defer pg.Cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ordersDB, err := DBWithSchema(pg.ConnStr, "orders")
	if err != nil {
		t.Fatalf("failed to create orders DB: %v", err)
	}
	defer func() { _ = ordersDB.Close() }()

	ordersRepo := orders.NewOrderRepository(ordersDB)
	ordersHandler, err := orders.NewHandler(ordersRepo, nil, logger)
	if err != nil {
		t.Fatalf("failed to create orders handler: %v", err)
	}
	ordersMux := http.NewServeMux()
	ordersMux.HandleFunc("POST /orders", ordersHandler.HandleCreate)
	ordersMux.HandleFunc("GET /orders/{id}", ordersHandler.HandleGet)
	ordersMux.HandleFunc("PATCH /orders/{id}/status", ordersHandler.HandleUpdateStatus)
	ordersServer := httptest.NewServer(ordersMux)
	defer ordersServer.Close()

	inventoryDB, err := DBWithSchema(pg.ConnStr, "inventory")
	if err != nil {
		t.Fatalf("failed to create inventory DB: %v", err)
	}
	defer func() { _ = inventoryDB.Close() }()

	inventoryRepo := inventory.NewInventoryRepository(inventoryDB)
	inventoryHandler := inventory.NewHandler(inventoryRepo, logger)
	inventoryMux := http.NewServeMux()
	inventoryMux.HandleFunc("GET /stock/{itemId}", inventoryHandler.HandleGetStock)
	inventoryMux.HandleFunc("POST /stock/{itemId}/reserve", inventoryHandler.HandleReserve)
	inventoryMux.HandleFunc("POST /stock/{itemId}/release", inventoryHandler.HandleRelease)
	inventoryServer := httptest.NewServer(inventoryMux)
	defer inventoryServer.Close()

	emailCap := &emailCapture{}
	emailMux := http.NewServeMux()
	emailMux.HandleFunc("POST /send", emailCap.handler)
	emailServer := httptest.NewServer(emailMux)
	defer emailServer.Close()

	httpClient := &http.Client{Timeout: 10 * time.Second}
	notificationHandler := worker.NewNotificationHandler(
		emailServer.URL,
		ordersServer.URL,
		inventoryServer.URL,
		httpClient,
		logger,
	)

	initialStock, err := inventoryRepo.GetStock(ctx, "ITEM-001")
	if err != nil {
		t.Fatalf("failed to get initial stock: %v", err)
	}
	initialAvailable := initialStock.Available
	initialReserved := initialStock.Reserved

	reqBody := `{"customer_id": "cust-456", "items": [{"item_id": "ITEM-001", "quantity": 9999, "price": 1000}]}`
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ordersHandler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var createdOrder domain.Order
	if err := json.NewDecoder(rec.Body).Decode(&createdOrder); err != nil {
		t.Fatalf("failed to decode order: %v", err)
	}

	event := domain.OrderCreatedEvent{
		OrderID:    createdOrder.ID,
		CustomerID: createdOrder.CustomerID,
		Items:      createdOrder.Items,
		Timestamp:  createdOrder.CreatedAt,
	}
	eventPayload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	if err := notificationHandler.Handle(ctx, eventPayload); err != nil {
		t.Fatalf("worker handler failed: %v", err)
	}

	finalOrder, err := ordersRepo.GetByID(ctx, createdOrder.ID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}

	if finalOrder.Status != domain.OrderStatusCancelled {
		t.Fatalf("expected order status %s, got %s", domain.OrderStatusCancelled, finalOrder.Status)
	}

	finalStock, err := inventoryRepo.GetStock(ctx, "ITEM-001")
	if err != nil {
		t.Fatalf("failed to get final stock: %v", err)
	}

	if finalStock.Available != initialAvailable {
		t.Fatalf("expected available stock unchanged at %d, got %d", initialAvailable, finalStock.Available)
	}
	if finalStock.Reserved != initialReserved {
		t.Fatalf("expected reserved stock unchanged at %d, got %d", initialReserved, finalStock.Reserved)
	}

	emails := emailCap.getEmails()
	if len(emails) != 1 {
		t.Fatalf("expected 1 email, got %d", len(emails))
	}

	email := emails[0]
	if !strings.Contains(email["subject"], "Cancelled") {
		t.Fatalf("expected cancellation email, got subject: %s", email["subject"])
	}
	if !strings.Contains(email["body"], "reimbursed") {
		t.Fatalf("expected email body to mention reimbursement, got: %s", email["body"])
	}
}

func TestOrderFlowWithPartialStockRollback(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	pg := SetupPostgres(ctx, t)
	defer pg.Cleanup()

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	ordersDB, err := DBWithSchema(pg.ConnStr, "orders")
	if err != nil {
		t.Fatalf("failed to create orders DB: %v", err)
	}
	defer func() { _ = ordersDB.Close() }()

	ordersRepo := orders.NewOrderRepository(ordersDB)
	ordersHandler, err := orders.NewHandler(ordersRepo, nil, logger)
	if err != nil {
		t.Fatalf("failed to create orders handler: %v", err)
	}
	ordersMux := http.NewServeMux()
	ordersMux.HandleFunc("POST /orders", ordersHandler.HandleCreate)
	ordersMux.HandleFunc("GET /orders/{id}", ordersHandler.HandleGet)
	ordersMux.HandleFunc("PATCH /orders/{id}/status", ordersHandler.HandleUpdateStatus)
	ordersServer := httptest.NewServer(ordersMux)
	defer ordersServer.Close()

	inventoryDB, err := DBWithSchema(pg.ConnStr, "inventory")
	if err != nil {
		t.Fatalf("failed to create inventory DB: %v", err)
	}
	defer func() { _ = inventoryDB.Close() }()

	inventoryRepo := inventory.NewInventoryRepository(inventoryDB)
	inventoryHandler := inventory.NewHandler(inventoryRepo, logger)
	inventoryMux := http.NewServeMux()
	inventoryMux.HandleFunc("GET /stock/{itemId}", inventoryHandler.HandleGetStock)
	inventoryMux.HandleFunc("POST /stock/{itemId}/reserve", inventoryHandler.HandleReserve)
	inventoryMux.HandleFunc("POST /stock/{itemId}/release", inventoryHandler.HandleRelease)
	inventoryServer := httptest.NewServer(inventoryMux)
	defer inventoryServer.Close()

	emailCap := &emailCapture{}
	emailMux := http.NewServeMux()
	emailMux.HandleFunc("POST /send", emailCap.handler)
	emailServer := httptest.NewServer(emailMux)
	defer emailServer.Close()

	httpClient := &http.Client{Timeout: 10 * time.Second}
	notificationHandler := worker.NewNotificationHandler(
		emailServer.URL,
		ordersServer.URL,
		inventoryServer.URL,
		httpClient,
		logger,
	)

	initialStock1, err := inventoryRepo.GetStock(ctx, "ITEM-001")
	if err != nil {
		t.Fatalf("failed to get initial stock for ITEM-001: %v", err)
	}
	initialStock2, err := inventoryRepo.GetStock(ctx, "ITEM-002")
	if err != nil {
		t.Fatalf("failed to get initial stock for ITEM-002: %v", err)
	}

	reqBody := `{
		"customer_id": "cust-789",
		"items": [
			{"item_id": "ITEM-001", "quantity": 5, "price": 1000},
			{"item_id": "ITEM-002", "quantity": 9999, "price": 2000}
		]
	}`
	req := httptest.NewRequest(http.MethodPost, "/orders", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	ordersHandler.HandleCreate(rec, req)

	if rec.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d: %s", http.StatusCreated, rec.Code, rec.Body.String())
	}

	var createdOrder domain.Order
	if err := json.NewDecoder(rec.Body).Decode(&createdOrder); err != nil {
		t.Fatalf("failed to decode order: %v", err)
	}

	event := domain.OrderCreatedEvent{
		OrderID:    createdOrder.ID,
		CustomerID: createdOrder.CustomerID,
		Items:      createdOrder.Items,
		Timestamp:  createdOrder.CreatedAt,
	}
	eventPayload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("failed to marshal event: %v", err)
	}

	if err := notificationHandler.Handle(ctx, eventPayload); err != nil {
		t.Fatalf("worker handler failed: %v", err)
	}

	finalOrder, err := ordersRepo.GetByID(ctx, createdOrder.ID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}

	if finalOrder.Status != domain.OrderStatusCancelled {
		t.Fatalf("expected order status %s, got %s", domain.OrderStatusCancelled, finalOrder.Status)
	}

	finalStock1, err := inventoryRepo.GetStock(ctx, "ITEM-001")
	if err != nil {
		t.Fatalf("failed to get final stock for ITEM-001: %v", err)
	}
	finalStock2, err := inventoryRepo.GetStock(ctx, "ITEM-002")
	if err != nil {
		t.Fatalf("failed to get final stock for ITEM-002: %v", err)
	}

	if finalStock1.Available != initialStock1.Available {
		t.Fatalf("ITEM-001: expected available stock rolled back to %d, got %d", initialStock1.Available, finalStock1.Available)
	}
	if finalStock1.Reserved != initialStock1.Reserved {
		t.Fatalf("ITEM-001: expected reserved stock rolled back to %d, got %d", initialStock1.Reserved, finalStock1.Reserved)
	}

	if finalStock2.Available != initialStock2.Available {
		t.Fatalf("ITEM-002: expected available stock unchanged at %d, got %d", initialStock2.Available, finalStock2.Available)
	}
	if finalStock2.Reserved != initialStock2.Reserved {
		t.Fatalf("ITEM-002: expected reserved stock unchanged at %d, got %d", initialStock2.Reserved, finalStock2.Reserved)
	}
	emails := emailCap.getEmails()
	if len(emails) != 1 {
		t.Fatalf("expected 1 cancellation email, got %d", len(emails))
	}
}

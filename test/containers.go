package test

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/kafka"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

type PostgresSetup struct {
	ConnStr string
	cleanup func()
}

func (p *PostgresSetup) Cleanup() {
	p.cleanup()
}

func SetupPostgres(ctx context.Context, t *testing.T) *PostgresSetup {
	t.Helper()

	container, err := postgres.Run(ctx,
		"postgres:18-alpine",
		postgres.WithDatabase("orderflow"),
		postgres.WithUsername("orderflow"),
		postgres.WithPassword("orderflow"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("failed to start postgres container: %v", err)
	}

	connStr, err := container.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("failed to get connection string: %v", err)
	}

	if err := runMigrations(connStr); err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("failed to run migrations: %v", err)
	}

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate postgres container: %v", err)
		}
	}

	return &PostgresSetup{ConnStr: connStr, cleanup: cleanup}
}

func runMigrations(connStr string) error {
	migrationsPath := getMigrationsPath()

	m, err := migrate.New(migrationsPath, connStr)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}
	defer func() { _, _ = m.Close() }()

	if err := m.Up(); err != nil && err != migrate.ErrNoChange {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

func getMigrationsPath() string {
	_, filename, _, _ := runtime.Caller(0)
	testDir := filepath.Dir(filename)
	projectRoot := filepath.Dir(testDir)
	migrationsDir := filepath.Join(projectRoot, "migrations")
	return "file://" + migrationsDir
}

func SetupKafka(ctx context.Context, t *testing.T) ([]string, func()) {
	t.Helper()

	container, err := kafka.Run(ctx,
		"confluentinc/confluent-local:7.8.0",
		kafka.WithClusterID("test-cluster"),
	)
	if err != nil {
		t.Fatalf("failed to start kafka container: %v", err)
	}

	brokers, err := container.Brokers(ctx)
	if err != nil {
		_ = container.Terminate(ctx)
		t.Fatalf("failed to get kafka brokers: %v", err)
	}

	cleanup := func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("failed to terminate kafka container: %v", err)
		}
	}

	return brokers, cleanup
}

func DBWithSchema(connStr, schema string) (*sql.DB, error) {
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open database connection: %w", err)
	}

	if _, err := db.Exec(fmt.Sprintf("SET search_path TO %s", schema)); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to set search_path: %w", err)
	}

	return db, nil
}

package main

import (
	"errors"
	"flag"
	"log/slog"
	"os"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	flag.Parse()
	args := flag.Args()

	if len(args) < 1 {
		logger.Error("usage: migrate <up|down|version>")
		os.Exit(1)
	}

	postgresURL := os.Getenv("POSTGRES_URL")
	if postgresURL == "" {
		logger.Error("POSTGRES_URL environment variable is required")
		os.Exit(1)
	}

	migrationsPath := os.Getenv("MIGRATIONS_PATH")
	if migrationsPath == "" {
		migrationsPath = "file://migrations"
	}

	m, err := migrate.New(migrationsPath, postgresURL)
	if err != nil {
		logger.Error("failed to create migrate instance", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer func() { _, _ = m.Close() }()

	command := args[0]

	switch command {
	case "up":
		err = m.Up()
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("no pending migrations")
			return
		}
		if err != nil {
			logger.Error("migration up failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("migrations applied successfully")

	case "down":
		err = m.Steps(-1)
		if errors.Is(err, migrate.ErrNoChange) {
			logger.Info("no migrations to rollback")
			return
		}
		if err != nil {
			logger.Error("migration down failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("migration rolled back successfully")

	case "version":
		version, dirty, err := m.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			logger.Info("no migrations applied yet")
			return
		}
		if err != nil {
			logger.Error("failed to get version", slog.String("error", err.Error()))
			os.Exit(1)
		}
		logger.Info("current migration version", slog.Uint64("version", uint64(version)), slog.Bool("dirty", dirty))

	default:
		logger.Error("unknown command", slog.String("command", command))
		os.Exit(1)
	}
}

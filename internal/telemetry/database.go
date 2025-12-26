package telemetry

import (
	"database/sql"

	"github.com/XSAM/otelsql"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func OpenDB(driverName, dsn string) (*sql.DB, error) {
	return otelsql.Open(driverName, dsn,
		otelsql.WithAttributes(semconv.DBSystemPostgreSQL),
	)
}

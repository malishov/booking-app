package db

import (
	"context"
	"database/sql"
	"fmt"

	_ "github.com/jackc/pgx/v5/stdlib" // register the "pgx" database/sql driver
	"github.com/pressly/goose/v3"

	"github.com/muradalishov/booking-app/backend/migrations"
)

// Migrate applies all pending goose migrations embedded in the binary.
//
// goose runs over the standard database/sql interface, so we open a short-lived
// connection using the pgx stdlib driver purely for the migration step; the
// application itself uses a pgxpool.Pool for queries.
func Migrate(ctx context.Context, databaseURL string) error {
	sqlDB, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return fmt.Errorf("db: open for migrate: %w", err)
	}
	defer func() { _ = sqlDB.Close() }()

	goose.SetBaseFS(migrations.FS)
	if err := goose.SetDialect("postgres"); err != nil {
		return fmt.Errorf("db: set dialect: %w", err)
	}

	if err := goose.UpContext(ctx, sqlDB, "."); err != nil {
		return fmt.Errorf("db: apply migrations: %w", err)
	}

	return nil
}

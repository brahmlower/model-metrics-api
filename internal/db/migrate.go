package db

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)

//go:embed schema.sql
var schemaSQL string

// Migrate applies the schema migrations to the database.
// It is idempotent: all DDL statements use IF NOT EXISTS.
func Migrate(sqlDB *sql.DB) error {
	_, err := sqlDB.ExecContext(context.Background(), schemaSQL)
	if err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	return nil
}

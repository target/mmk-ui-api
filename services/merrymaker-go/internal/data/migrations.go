package data

import (
	"context"
	"database/sql"

	"github.com/target/mmk-ui-api/internal/migrate"
)

// RunMigrations executes database migrations to set up the required schema by delegating to the migrate package.
func RunMigrations(ctx context.Context, db *sql.DB) error {
	return migrate.Run(ctx, db)
}

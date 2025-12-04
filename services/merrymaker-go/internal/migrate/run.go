package migrate

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Run applies all SQL migrations embedded in this package. It is safe to call multiple times.
func Run(ctx context.Context, db *sql.DB) error {
	// Ensure schema_migrations table exists (TEXT schema by default).
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`); err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	for _, f := range files {
		info := migrationInfo{
			versionStr: strings.TrimSuffix(f, ".sql"),
			file:       f,
		}
		if applyErr := applyMigration(ctx, db, info); applyErr != nil {
			return applyErr
		}
	}
	return nil
}

// migrationInfo holds information about a migration for processing.
type migrationInfo struct {
	versionStr string
	file       string
}

func migrationExists(ctx context.Context, db *sql.DB, info migrationInfo) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)`
	if err := db.QueryRowContext(ctx, query, info.versionStr).Scan(&exists); err != nil {
		return false, fmt.Errorf("check migration %s: %w", info.file, err)
	}
	return exists, nil
}

func insertMigration(ctx context.Context, tx *sql.Tx, info migrationInfo) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, info.versionStr); err != nil {
		return fmt.Errorf("record migration %s: %w", info.file, err)
	}
	return nil
}

func applyMigration(ctx context.Context, db *sql.DB, info migrationInfo) error {
	exists, err := migrationExists(ctx, db, info)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	sqlBytes, err := migrationsFS.ReadFile("migrations/" + info.file)
	if err != nil {
		return fmt.Errorf("read migration %s: %w", info.file, err)
	}

	logger := slog.Default().With("component", "migrations")
	logger.InfoContext(ctx, "applying migration", "version", info.versionStr)

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if rollbackErr := tx.Rollback(); rollbackErr != nil && !errors.Is(rollbackErr, sql.ErrTxDone) {
			logger.ErrorContext(
				ctx,
				"failed to rollback transaction",
				"err",
				rollbackErr,
				"migration_file",
				info.file,
			)
		}
	}()

	if _, execErr := tx.ExecContext(ctx, string(sqlBytes)); execErr != nil {
		return fmt.Errorf("exec migration %s: %w", info.file, execErr)
	}
	if insertErr := insertMigration(ctx, tx, info); insertErr != nil {
		return insertErr
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("commit migration %s: %w", info.file, commitErr)
	}

	return nil
}

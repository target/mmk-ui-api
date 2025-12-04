package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrSourceNotFound is returned when a source is not found.
	ErrSourceNotFound = errors.New("source not found")
	// ErrSourceNameExists is returned when attempting to create a source with a name that already exists.
	ErrSourceNameExists = errors.New("source name already exists")
)

// SourceRepo provides database operations for source management.
type SourceRepo struct {
	DB           *sql.DB
	timeProvider TimeProvider
}

// NewSourceRepo creates a new SourceRepo instance with the given database connection.
func NewSourceRepo(db *sql.DB) *SourceRepo {
	return &SourceRepo{
		DB:           db,
		timeProvider: &RealTimeProvider{},
	}
}

// NewSourceRepoWithTimeProvider creates a SourceRepo with a custom TimeProvider (useful for testing).
func NewSourceRepoWithTimeProvider(db *sql.DB, timeProvider TimeProvider) *SourceRepo {
	return &SourceRepo{
		DB:           db,
		timeProvider: timeProvider,
	}
}

// Create creates a new source and associates any provided secret names.
func (r *SourceRepo) Create(ctx context.Context, req *model.CreateSourceRequest) (*model.Source, error) {
	if req == nil {
		return nil, errors.New("create source request is required")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	createdAt := r.timeProvider.Now()

	var out model.Source
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) (err error) {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() {
			if rerr := tx.Rollback(ctx); rerr != nil && !errors.Is(rerr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rerr))
			}
		}()

		var sourceID string
		sourceID, err = r.insertSourceTx(ctx, tx, req, createdAt)
		if err != nil {
			return err
		}

		if err = r.ensureSecretsExistTx(ctx, tx, req.Secrets); err != nil {
			return err
		}
		if err = r.associateSecretsByNamesTx(ctx, tx, sourceID, req.Secrets); err != nil {
			return err
		}

		out, err = r.loadSourceByIDTx(ctx, tx, sourceID)
		if err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create source: %w", r.mapSourceWriteErr(err, false))
	}

	return &out, nil
}

// getSourceByQuery is a helper function to execute a query and return a single source.
// Uses variadic args to avoid slice allocation at call sites.
func (r *SourceRepo) getSourceByQuery(
	ctx context.Context,
	q string,
	errMsg string,
	args ...any,
) (*model.Source, error) {
	var source model.Source
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		source, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Source])
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSourceNotFound
		}
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	return &source, nil
}

// GetByID retrieves a source by its ID.
func (r *SourceRepo) GetByID(ctx context.Context, id string) (*model.Source, error) {
	return r.getSourceByQuery(ctx, sourceGetByIDQuery, "failed to get source by ID", id)
}

// GetByName retrieves a source by its name.
func (r *SourceRepo) GetByName(ctx context.Context, name string) (*model.Source, error) {
	return r.getSourceByQuery(ctx, sourceGetByNameQuery, "failed to get source by name", name)
}

// List retrieves a list of sources with pagination.
func (r *SourceRepo) List(ctx context.Context, limit, offset int) ([]*model.Source, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var sources []model.Source
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, sourceListQuery, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		sources, err = pgx.CollectRows(rows, pgx.RowToStructByName[model.Source])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sources: %w", err)
	}

	result := make([]*model.Source, len(sources))
	for i := range sources {
		result[i] = &sources[i]
	}

	return result, nil
}

// ListByNameContains retrieves sources filtered by name substring with pagination.
func (r *SourceRepo) ListByNameContains(ctx context.Context, q string, limit, offset int) ([]*model.Source, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	// Use ILIKE with wildcards for case-insensitive substring search
	// Note: We do NOT trim the query string to preserve original behavior
	// and allow searching for names with leading/trailing spaces if needed.
	searchPattern := "%" + q + "%"

	var sources []model.Source
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, sourceListByNameQuery, searchPattern, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		sources, err = pgx.CollectRows(rows, pgx.RowToStructByName[model.Source])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list sources by name: %w", err)
	}
	result := make([]*model.Source, len(sources))
	for i := range sources {
		result[i] = &sources[i]
	}
	return result, nil
}

// --- helpers to reduce complexity in Create/Update ---

func (r *SourceRepo) insertSourceTx(
	ctx context.Context,
	tx pgx.Tx,
	req *model.CreateSourceRequest,
	createdAt time.Time,
) (string, error) {
	if req == nil {
		return "", errors.New("create source request is required")
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO sources (name, value, test, created_at)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`, req.Name, req.Value, req.Test, createdAt)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (r *SourceRepo) ensureSecretsExistTx(ctx context.Context, tx pgx.Tx, names []string) error {
	if len(names) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO secrets (name, value)
		SELECT unnest($1::text[]), ''
		ON CONFLICT (name) DO NOTHING
	`, names)
	return err
}

func (r *SourceRepo) associateSecretsByNamesTx(ctx context.Context, tx pgx.Tx, sourceID string, names []string) error {
	if len(names) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO source_secrets (source_id, secret_id)
		SELECT $1, s.id FROM secrets s WHERE s.name = ANY($2)
		ON CONFLICT DO NOTHING
	`, sourceID, names)
	return err
}

func (r *SourceRepo) loadSourceByIDTx(ctx context.Context, tx pgx.Tx, id string) (model.Source, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.id, s.name, s.value, s.test, s.created_at,
		       COALESCE(array_agg(sec.name ORDER BY sec.name)
		                FILTER (WHERE sec.name IS NOT NULL), '{}') AS secrets
		FROM sources s
		LEFT JOIN source_secrets ss ON ss.source_id = s.id
		LEFT JOIN secrets sec ON sec.id = ss.secret_id
		WHERE s.id = $1
		GROUP BY s.id
	`, id)
	if err != nil {
		return model.Source{}, err
	}
	defer rows.Close()
	return pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Source])
}

func (r *SourceRepo) updateSourceFieldsTx(
	ctx context.Context,
	tx pgx.Tx,
	id string,
	req model.UpdateSourceRequest,
) error {
	setParts := make([]string, 0, 3)
	args := make([]any, 0, 4)
	argIdx := 1
	if req.Name != nil {
		setParts = append(setParts, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if req.Value != nil {
		setParts = append(setParts, fmt.Sprintf("value = $%d", argIdx))
		args = append(args, *req.Value)
		argIdx++
	}
	if req.Test != nil {
		setParts = append(setParts, fmt.Sprintf("test = $%d", argIdx))
		args = append(args, *req.Test)
		argIdx++
	}
	if len(setParts) == 0 {
		return nil
	}
	args = append(args, id)
	ct, err := tx.Exec(
		ctx,
		"UPDATE sources SET "+strings.Join(setParts, ", ")+fmt.Sprintf(" WHERE id = $%d", argIdx),
		args...)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *SourceRepo) replaceAssociationsTx(ctx context.Context, tx pgx.Tx, id string, names []string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM source_secrets WHERE source_id = $1`, id); err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	if err := r.ensureSecretsExistTx(ctx, tx, names); err != nil {
		return err
	}
	return r.associateSecretsByNamesTx(ctx, tx, id, names)
}

// Update updates an existing source and optionally replaces its secret associations.

func (r *SourceRepo) Update(ctx context.Context, id string, req model.UpdateSourceRequest) (*model.Source, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var out model.Source
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) (err error) {
		tx, err := conn.Begin(ctx)
		if err != nil {
			return err
		}
		defer func() {
			if rerr := tx.Rollback(ctx); rerr != nil && !errors.Is(rerr, pgx.ErrTxClosed) {
				err = errors.Join(err, fmt.Errorf("rollback: %w", rerr))
			}
		}()

		// Update basic fields if any
		if err = r.updateSourceFieldsTx(ctx, tx, id, req); err != nil {
			return err
		}

		// Replace secret associations if provided
		if req.Secrets != nil {
			if err = r.replaceAssociationsTx(ctx, tx, id, req.Secrets); err != nil {
				return err
			}
		}

		// Read back and commit
		out, err = r.loadSourceByIDTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if err = tx.Commit(ctx); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update source: %w", r.mapSourceWriteErr(err, true))
	}

	return &out, nil
}

// Delete deletes a source by its ID.
func (r *SourceRepo) mapSourceWriteErr(err error, includeNotFound bool) error {
	if err == nil {
		return nil
	}
	if includeNotFound && errors.Is(err, pgx.ErrNoRows) {
		return ErrSourceNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrSourceNameExists
	}
	return err
}

func (r *SourceRepo) Delete(ctx context.Context, id string) (bool, error) {
	query := `DELETE FROM sources WHERE id = $1`

	result, err := r.DB.ExecContext(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("failed to delete source: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}

// --- helpers ---

// SQL query constants for static queries (no dynamic WHERE/ORDER BY).
// Using constants avoids runtime query building overhead for hot paths.
const (
	sourceGetByIDQuery = `
		SELECT s.id, s.name, s.value, s.test, s.created_at,
		       COALESCE(array_agg(sec.name ORDER BY sec.name)
		                FILTER (WHERE sec.name IS NOT NULL), '{}') AS secrets
		FROM sources s
		LEFT JOIN source_secrets ss ON ss.source_id = s.id
		LEFT JOIN secrets sec ON sec.id = ss.secret_id
		WHERE s.id = $1
		GROUP BY s.id`

	sourceGetByNameQuery = `
		SELECT s.id, s.name, s.value, s.test, s.created_at,
		       COALESCE(array_agg(sec.name ORDER BY sec.name)
		                FILTER (WHERE sec.name IS NOT NULL), '{}') AS secrets
		FROM sources s
		LEFT JOIN source_secrets ss ON ss.source_id = s.id
		LEFT JOIN secrets sec ON sec.id = ss.secret_id
		WHERE s.name = $1
		GROUP BY s.id`

	sourceListQuery = `
		SELECT s.id, s.name, s.value, s.test, s.created_at,
		       COALESCE(array_agg(sec.name ORDER BY sec.name)
		                FILTER (WHERE sec.name IS NOT NULL), '{}') AS secrets
		FROM sources s
		LEFT JOIN source_secrets ss ON ss.source_id = s.id
		LEFT JOIN secrets sec ON sec.id = ss.secret_id
		GROUP BY s.id
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $1 OFFSET $2`

	sourceListByNameQuery = `
		SELECT s.id, s.name, s.value, s.test, s.created_at,
		       COALESCE(array_agg(sec.name ORDER BY sec.name)
		                FILTER (WHERE sec.name IS NOT NULL), '{}') AS secrets
		FROM sources s
		LEFT JOIN source_secrets ss ON ss.source_id = s.id
		LEFT JOIN secrets sec ON sec.id = ss.secret_id
		WHERE s.name ILIKE $1
		GROUP BY s.id
		ORDER BY s.created_at DESC, s.id DESC
		LIMIT $2 OFFSET $3`
)

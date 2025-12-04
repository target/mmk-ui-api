package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

var (
	// ErrHTTPAlertSinkNotFound is returned when an HTTP alert sink is not found.
	ErrHTTPAlertSinkNotFound = errors.New("http alert sink not found")
	// ErrHTTPAlertSinkNameExists is returned when attempting to create an HTTP alert sink with a name that already exists.
	ErrHTTPAlertSinkNameExists = errors.New("http alert sink name already exists")
)

// HTTPAlertSinkRepo provides database operations for HTTP alert sink management.
type HTTPAlertSinkRepo struct {
	DB           *sql.DB
	timeProvider TimeProvider
}

// NewHTTPAlertSinkRepo creates a new HTTPAlertSinkRepo.
func NewHTTPAlertSinkRepo(db *sql.DB) *HTTPAlertSinkRepo {
	return &HTTPAlertSinkRepo{
		DB:           db,
		timeProvider: &RealTimeProvider{},
	}
}

// NewHTTPAlertSinkRepoWithTimeProvider creates a new HTTPAlertSinkRepo with a custom time provider.
func NewHTTPAlertSinkRepoWithTimeProvider(db *sql.DB, tp TimeProvider) *HTTPAlertSinkRepo {
	return &HTTPAlertSinkRepo{
		DB:           db,
		timeProvider: tp,
	}
}

// httpAlertSinkQueryParams holds parameters for query operations.
type httpAlertSinkQueryParams struct {
	query    string
	arg      any
	errorMsg string
}

// --- compact param structs to enforce <=3 function params ---.
type httpAlertSinkInsertParams struct {
	req       *model.CreateHTTPAlertSinkRequest
	createdAt time.Time
}

type httpAlertSinkAssocParams struct {
	sinkID string
	names  []string
}

type httpAlertSinkUpdateFieldsParams struct {
	id  string
	req *model.UpdateHTTPAlertSinkRequest
}

// Create creates a new HTTP alert sink with the given request parameters.
func (r *HTTPAlertSinkRepo) Create(
	ctx context.Context,
	req *model.CreateHTTPAlertSinkRequest,
) (*model.HTTPAlertSink, error) {
	if req == nil {
		return nil, errors.New("create http alert sink request is required")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Normalize the request
	req.Normalize()

	createdAt := r.timeProvider.Now()

	var out model.HTTPAlertSink
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

		sinkID, err := r.insertHTTPAlertSinkTx(ctx, tx, &httpAlertSinkInsertParams{req: req, createdAt: createdAt})
		if err != nil {
			return err
		}

		if ensureErr := r.ensureSecretsExistTx(ctx, tx, req.Secrets); ensureErr != nil {
			return ensureErr
		}
		if assocErr := r.associateSecretsByNamesTx(ctx, tx, httpAlertSinkAssocParams{sinkID: sinkID, names: req.Secrets}); assocErr != nil {
			return assocErr
		}

		out, err = r.loadHTTPAlertSinkByIDTx(ctx, tx, sinkID)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create http alert sink: %w", r.mapHTTPAlertSinkWriteErr(err, false))
	}

	return &out, nil
}

// getHTTPAlertSinkByQuery is a helper function to reduce duplication between GetByID and GetByName.
func (r *HTTPAlertSinkRepo) getHTTPAlertSinkByQuery(
	ctx context.Context,
	params httpAlertSinkQueryParams,
) (*model.HTTPAlertSink, error) {
	var sink model.HTTPAlertSink
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, params.query, params.arg)
		if err != nil {
			return err
		}
		defer rows.Close()
		// Use pgx.CollectOneRow with pgx.RowToStructByName for automatic field mapping
		sink, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.HTTPAlertSink])
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrHTTPAlertSinkNotFound
		}
		return nil, fmt.Errorf("%s: %w", params.errorMsg, err)
	}

	return &sink, nil
}

// GetByID retrieves an HTTP alert sink by its ID.
func (r *HTTPAlertSinkRepo) GetByID(ctx context.Context, id string) (*model.HTTPAlertSink, error) {
	query := `
		SELECT h.id, h.name, h.uri, h.method, h.body, h.query_params, h.headers,
		       h.ok_status, h.retry, h.created_at,
		       COALESCE(array_agg(sec.name ORDER BY sec.name)
		                FILTER (WHERE sec.name IS NOT NULL), '{}') AS secrets
		FROM http_alert_sinks h
		LEFT JOIN http_alert_sink_secrets hss ON hss.http_alert_sink_id = h.id
		LEFT JOIN secrets sec ON sec.id = hss.secret_id
		WHERE h.id = $1
		GROUP BY h.id`

	return r.getHTTPAlertSinkByQuery(ctx, httpAlertSinkQueryParams{
		query:    query,
		arg:      id,
		errorMsg: "failed to get http alert sink by ID",
	})
}

// GetByName retrieves an HTTP alert sink by its name.
func (r *HTTPAlertSinkRepo) GetByName(ctx context.Context, name string) (*model.HTTPAlertSink, error) {
	query := `
		SELECT h.id, h.name, h.uri, h.method, h.body, h.query_params, h.headers,
		       h.ok_status, h.retry, h.created_at,
		       COALESCE(array_agg(sec.name ORDER BY sec.name)
		                FILTER (WHERE sec.name IS NOT NULL), '{}') AS secrets
		FROM http_alert_sinks h
		LEFT JOIN http_alert_sink_secrets hss ON hss.http_alert_sink_id = h.id
		LEFT JOIN secrets sec ON sec.id = hss.secret_id
		WHERE h.name = $1
		GROUP BY h.id`

	return r.getHTTPAlertSinkByQuery(ctx, httpAlertSinkQueryParams{
		query:    query,
		arg:      name,
		errorMsg: "failed to get http alert sink by name",
	})
}

// List retrieves a list of HTTP alert sinks with pagination.
func (r *HTTPAlertSinkRepo) List(ctx context.Context, limit, offset int) ([]*model.HTTPAlertSink, error) {
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if offset < 0 {
		offset = 0
	}

	query := `
		SELECT h.id, h.name, h.uri, h.method, h.body, h.query_params, h.headers,
		       h.ok_status, h.retry, h.created_at,
		       COALESCE(array_agg(sec.name ORDER BY sec.name)
		                FILTER (WHERE sec.name IS NOT NULL), '{}') AS secrets
		FROM http_alert_sinks h
		LEFT JOIN http_alert_sink_secrets hss ON hss.http_alert_sink_id = h.id
		LEFT JOIN secrets sec ON sec.id = hss.secret_id
		GROUP BY h.id
		ORDER BY h.created_at DESC, h.id DESC
		LIMIT $1 OFFSET $2`

	var sinks []*model.HTTPAlertSink
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()

		sinksSlice, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.HTTPAlertSink])
		if err != nil {
			return err
		}

		// Convert slice to pointer slice (preallocate for efficiency)
		sinks = make([]*model.HTTPAlertSink, len(sinksSlice))
		for i := range sinksSlice {
			sinks[i] = &sinksSlice[i]
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list http alert sinks: %w", err)
	}

	return sinks, nil
}

// --- helpers to reduce complexity in Create/Update ---

func (r *HTTPAlertSinkRepo) insertHTTPAlertSinkTx(
	ctx context.Context,
	tx pgx.Tx,
	p *httpAlertSinkInsertParams,
) (string, error) {
	if p == nil || p.req == nil {
		return "", errors.New("insert params are required")
	}

	// Set defaults for optional fields
	req := p.req
	createdAt := p.createdAt
	okStatus := 200
	if req.OkStatus != nil {
		okStatus = *req.OkStatus
	}
	retry := 3
	if req.Retry != nil {
		retry = *req.Retry
	}

	row := tx.QueryRow(ctx, `
		INSERT INTO http_alert_sinks (name, uri, method, body, query_params, headers, ok_status, retry, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id
	`, req.Name, req.URI, req.Method, req.Body, req.QueryParams, req.Headers, okStatus, retry, createdAt)
	var id string
	if err := row.Scan(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (r *HTTPAlertSinkRepo) ensureSecretsExistTx(ctx context.Context, tx pgx.Tx, names []string) error {
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

func (r *HTTPAlertSinkRepo) associateSecretsByNamesTx(
	ctx context.Context,
	tx pgx.Tx,
	p httpAlertSinkAssocParams,
) error {
	if len(p.names) == 0 {
		return nil
	}
	_, err := tx.Exec(ctx, `
		INSERT INTO http_alert_sink_secrets (http_alert_sink_id, secret_id)
		SELECT $1, s.id
		FROM secrets s
		WHERE s.name = ANY($2::text[])
		ON CONFLICT (http_alert_sink_id, secret_id) DO NOTHING
	`, p.sinkID, p.names)
	return err
}

func (r *HTTPAlertSinkRepo) loadHTTPAlertSinkByIDTx(
	ctx context.Context,
	tx pgx.Tx,
	id string,
) (model.HTTPAlertSink, error) {
	rows, err := tx.Query(ctx, `
		SELECT h.id, h.name, h.uri, h.method, h.body, h.query_params, h.headers,
		       h.ok_status, h.retry, h.created_at,
		       COALESCE(array_agg(sec.name ORDER BY sec.name)
		                FILTER (WHERE sec.name IS NOT NULL), '{}') AS secrets
		FROM http_alert_sinks h
		LEFT JOIN http_alert_sink_secrets hss ON hss.http_alert_sink_id = h.id
		LEFT JOIN secrets sec ON sec.id = hss.secret_id
		WHERE h.id = $1
		GROUP BY h.id
	`, id)
	if err != nil {
		return model.HTTPAlertSink{}, err
	}
	defer rows.Close()

	return pgx.CollectOneRow(rows, pgx.RowToStructByName[model.HTTPAlertSink])
}

func (r *HTTPAlertSinkRepo) updateHTTPAlertSinkFieldsTx(
	ctx context.Context,
	tx pgx.Tx,
	p *httpAlertSinkUpdateFieldsParams,
) error {
	if p == nil || p.req == nil {
		return errors.New("update params are required")
	}

	setParts, args := r.buildUpdateParts(p.req)
	if len(setParts) == 0 {
		return nil // No fields to update
	}

	args = append(args, p.id)
	argIdx := len(args)

	query := "UPDATE http_alert_sinks SET " + strings.Join(setParts, ", ") + fmt.Sprintf(" WHERE id = $%d", argIdx)

	ct, err := tx.Exec(ctx, query, args...)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (r *HTTPAlertSinkRepo) buildUpdateParts(req *model.UpdateHTTPAlertSinkRequest) ([]string, []any) {
	if req == nil {
		return nil, nil
	}

	var setParts []string
	var args []any
	argIdx := 1

	if req.Name != nil {
		setParts = append(setParts, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)

		argIdx++
	}
	if req.URI != nil {
		setParts = append(setParts, fmt.Sprintf("uri = $%d", argIdx))
		args = append(args, *req.URI)
		argIdx++
	}
	if req.Method != nil {
		setParts = append(setParts, fmt.Sprintf("method = $%d", argIdx))
		args = append(args, *req.Method)
		argIdx++
	}
	if req.Body != nil {
		setParts = append(setParts, fmt.Sprintf("body = $%d", argIdx))
		args = append(args, *req.Body)
		argIdx++
	}
	if req.QueryParams != nil {
		setParts = append(setParts, fmt.Sprintf("query_params = $%d", argIdx))
		args = append(args, *req.QueryParams)
		argIdx++
	}
	if req.Headers != nil {
		setParts = append(setParts, fmt.Sprintf("headers = $%d", argIdx))
		args = append(args, *req.Headers)
		argIdx++
	}
	if req.OkStatus != nil {
		setParts = append(setParts, fmt.Sprintf("ok_status = $%d", argIdx))
		args = append(args, *req.OkStatus)
		argIdx++
	}
	if req.Retry != nil {
		setParts = append(setParts, fmt.Sprintf("retry = $%d", argIdx))
		args = append(args, *req.Retry)
	}

	return setParts, args
}

func (r *HTTPAlertSinkRepo) replaceAssociationsTx(ctx context.Context, tx pgx.Tx, p httpAlertSinkAssocParams) error {
	if _, err := tx.Exec(ctx, `DELETE FROM http_alert_sink_secrets WHERE http_alert_sink_id = $1`, p.sinkID); err != nil {
		return err
	}
	if len(p.names) == 0 {
		return nil
	}
	if err := r.ensureSecretsExistTx(ctx, tx, p.names); err != nil {
		return err
	}
	return r.associateSecretsByNamesTx(ctx, tx, p)
}

// Update updates an existing HTTP alert sink and optionally replaces its secret associations.
func (r *HTTPAlertSinkRepo) Update(
	ctx context.Context,
	id string,
	req *model.UpdateHTTPAlertSinkRequest,
) (*model.HTTPAlertSink, error) {
	if req == nil {
		return nil, errors.New("update http alert sink request is required")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Normalize the request
	req.Normalize()

	var out model.HTTPAlertSink
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
		if fieldsErr := r.updateHTTPAlertSinkFieldsTx(ctx, tx, &httpAlertSinkUpdateFieldsParams{id: id, req: req}); fieldsErr != nil {
			return fieldsErr
		}

		// Replace secret associations if provided
		if req.Secrets != nil {
			// Ensure sink exists even when no updatable fields were provided
			if ensureErr := r.ensureHTTPAlertSinkExistsTx(ctx, tx, id); ensureErr != nil {
				return ensureErr
			}
			if replaceErr := r.replaceAssociationsTx(ctx, tx, httpAlertSinkAssocParams{sinkID: id, names: req.Secrets}); replaceErr != nil {
				return replaceErr
			}
		}

		// Read back and commit
		out, err = r.loadHTTPAlertSinkByIDTx(ctx, tx, id)
		if err != nil {
			return err
		}
		return tx.Commit(ctx)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update http alert sink: %w", r.mapHTTPAlertSinkWriteErr(err, true))
	}

	return &out, nil
}

// Delete deletes an HTTP alert sink by its ID.
func (r *HTTPAlertSinkRepo) Delete(ctx context.Context, id string) (bool, error) {
	query := `DELETE FROM http_alert_sinks WHERE id = $1`

	var rowsAffected int64
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		ct, err := conn.Exec(ctx, query, id)
		if err != nil {
			return err
		}
		rowsAffected = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to delete http alert sink: %w", err)
	}

	return rowsAffected > 0, nil
}

// mapHTTPAlertSinkWriteErr maps database errors to domain-specific errors.
func (r *HTTPAlertSinkRepo) mapHTTPAlertSinkWriteErr(err error, includeNotFound bool) error {
	if err == nil {
		return nil
	}
	if includeNotFound && errors.Is(err, pgx.ErrNoRows) {
		return ErrHTTPAlertSinkNotFound
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return err
	}
	if pgErr.Code != "23505" {
		return err
	}

	// Only map to name-exists when the unique violation is on the sinks table.
	if pgErr.TableName == "http_alert_sinks" {
		return ErrHTTPAlertSinkNameExists
	}

	return err
}

func (r *HTTPAlertSinkRepo) ensureHTTPAlertSinkExistsTx(ctx context.Context, tx pgx.Tx, id string) error {
	var got string
	row := tx.QueryRow(ctx, `SELECT id FROM http_alert_sinks WHERE id = $1`, id)
	if err := row.Scan(&got); err != nil {
		return err
	}
	return nil
}

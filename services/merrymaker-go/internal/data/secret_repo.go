package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// SecretRepo provides CRUD operations for secrets with at-rest encryption.
type SecretRepo struct {
	DB  *sql.DB
	Enc cryptoutil.Encryptor
}

// NewSecretRepo creates a new SecretRepo.
func NewSecretRepo(db *sql.DB, enc cryptoutil.Encryptor) *SecretRepo {
	return &SecretRepo{DB: db, Enc: enc}
}

var (
	// ErrSecretNotFound is returned when a secret is not found.
	ErrSecretNotFound = errors.New("secret not found")
	// ErrSecretNameExists is returned when creating or renaming to an existing name.
	ErrSecretNameExists = errors.New("secret name already exists")
)

func (r *SecretRepo) mapWriteErr(err error, includeNotFound bool) error {
	if err == nil {
		return nil
	}
	if includeNotFound && errors.Is(err, pgx.ErrNoRows) {
		return ErrSecretNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "secrets_name_key" {
		return ErrSecretNameExists
	}
	return err
}

// --- helpers to reduce duplication and complexity ---

type secretQueryParams struct {
	query    string
	arg      any
	errorMsg string
}

func (r *SecretRepo) decryptSecretValue(secret *model.Secret) error {
	if secret == nil || secret.Value == "" {
		return nil
	}

	pt, err := r.Enc.Decrypt(secret.Value)
	if err != nil {
		prefix := secret.Value
		if len(prefix) > 20 {
			prefix = prefix[:20] + "..."
		}
		return fmt.Errorf("decrypt value (prefix: %s): %w", prefix, err)
	}

	secret.Value = string(pt)
	return nil
}

func (r *SecretRepo) getSecretByQuery(ctx context.Context, params secretQueryParams) (*model.Secret, error) {
	var s model.Secret
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, params.query, params.arg)
		if err != nil {
			return err
		}
		defer rows.Close()
		// Use pgx.CollectOneRow with pgx.RowToStructByName for automatic field mapping
		s, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Secret])
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", params.errorMsg, err)
	}

	if decryptErr := r.decryptSecretValue(&s); decryptErr != nil {
		return nil, decryptErr
	}

	return &s, nil
}

// Create inserts a new secret, storing the encrypted value.
// Supports both static secrets (with value) and dynamic secrets (with refresh configuration).
func (r *SecretRepo) Create(ctx context.Context, req model.CreateSecretRequest) (*model.Secret, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Encrypt value if provided (optional for dynamic secrets)
	var cipher string
	if req.Value != "" {
		c, err := r.Enc.Encrypt([]byte(req.Value))
		if err != nil {
			return nil, fmt.Errorf("encrypt: %w", err)
		}
		cipher = c
	}

	// Determine if refresh is enabled
	refreshEnabled := req.RefreshEnabled != nil && *req.RefreshEnabled

	var out model.Secret
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		params := &createSecretParams{
			conn:   conn,
			req:    &req,
			cipher: cipher,
			out:    &out,
		}
		if refreshEnabled {
			return r.createDynamicSecret(ctx, params)
		}
		return r.createStaticSecret(ctx, params)
	})
	if err != nil {
		return nil, r.mapWriteErr(err, false)
	}

	// Decrypt value before returning (if present)
	if out.Value != "" {
		pt, derr := r.Enc.Decrypt(out.Value)
		if derr != nil {
			return nil, fmt.Errorf("decrypt value: %w", derr)
		}
		out.Value = string(pt)
	}

	return &out, nil
}

// createSecretParams groups parameters for creating a secret.
type createSecretParams struct {
	conn   *pgx.Conn
	req    *model.CreateSecretRequest
	cipher string
	out    *model.Secret
}

// createDynamicSecret inserts a secret with refresh configuration.
func (r *SecretRepo) createDynamicSecret(ctx context.Context, params *createSecretParams) error {
	if params == nil || params.req == nil || params.req.RefreshInterval == nil {
		return errors.New("create secret params are required")
	}

	intervalSecs := *params.req.RefreshInterval
	rows, err := params.conn.Query(ctx, `
		INSERT INTO secrets (
			name, value,
			provider_script_path, env_config, refresh_interval, refresh_enabled
		)
		VALUES ($1, $2, $3, COALESCE($4::jsonb, '{}'::jsonb), ($5::bigint * interval '1 second'), $6)
		RETURNING id, name, value, created_at, updated_at,
		          provider_script_path, env_config,
		          EXTRACT(EPOCH FROM refresh_interval)::bigint AS refresh_interval_seconds,
		          last_refreshed_at, last_refresh_status, last_refresh_error, refresh_enabled
`, params.req.Name, params.cipher, params.req.ProviderScriptPath, params.req.EnvConfig, intervalSecs, true)
	if err != nil {
		return err
	}
	defer rows.Close()
	*params.out, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Secret])
	return err
}

// createStaticSecret inserts a static secret without refresh configuration.
func (r *SecretRepo) createStaticSecret(ctx context.Context, params *createSecretParams) error {
	if params == nil || params.req == nil {
		return errors.New("create secret params are required")
	}

	rows, err := params.conn.Query(ctx, `
		INSERT INTO secrets (name, value)
		VALUES ($1, $2)
		RETURNING id, name, value, created_at, updated_at,
		          provider_script_path, env_config,
		          EXTRACT(EPOCH FROM refresh_interval)::bigint AS refresh_interval_seconds,
		          last_refreshed_at, last_refresh_status, last_refresh_error, refresh_enabled
	`, params.req.Name, params.cipher)
	if err != nil {
		return err
	}
	defer rows.Close()
	*params.out, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Secret])
	return err
}

// GetByID fetches a secret by ID and returns it with decrypted value.
func (r *SecretRepo) GetByID(ctx context.Context, id string) (*model.Secret, error) {
	query := `
		SELECT id, name, value, created_at, updated_at,
		       provider_script_path, env_config,
		       EXTRACT(EPOCH FROM refresh_interval)::bigint AS refresh_interval_seconds,
		       last_refreshed_at, last_refresh_status, last_refresh_error, refresh_enabled
		FROM secrets WHERE id = $1`
	return r.getSecretByQuery(ctx, secretQueryParams{
		query:    query,
		arg:      id,
		errorMsg: "get secret by id",
	})
}

// GetByName fetches a secret by name and returns it with decrypted value.
func (r *SecretRepo) GetByName(ctx context.Context, name string) (*model.Secret, error) {
	query := `
		SELECT id, name, value, created_at, updated_at,
		       provider_script_path, env_config,
		       EXTRACT(EPOCH FROM refresh_interval)::bigint AS refresh_interval_seconds,
		       last_refreshed_at, last_refresh_status, last_refresh_error, refresh_enabled
		FROM secrets WHERE name = $1`
	return r.getSecretByQuery(ctx, secretQueryParams{
		query:    query,
		arg:      name,
		errorMsg: "get secret by name",
	})
}

// List returns secrets metadata (without values) with pagination.
func (r *SecretRepo) List(ctx context.Context, limit, offset int) ([]*model.Secret, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	query := `
		SELECT id, name, ''::text AS value, created_at, updated_at,
		       provider_script_path, env_config,
		       EXTRACT(EPOCH FROM refresh_interval)::bigint AS refresh_interval_seconds,
		       last_refreshed_at, last_refresh_status, last_refresh_error, refresh_enabled
		FROM secrets
		ORDER BY created_at DESC, id DESC
		LIMIT $1 OFFSET $2`

	var secretsSlice []model.Secret
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		// Use pgx.CollectRows with pgx.RowToStructByName for automatic collection
		secretsSlice, err = pgx.CollectRows(rows, pgx.RowToStructByName[model.Secret])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}

	// Convert slice to pointer slice (preallocate for efficiency)
	secrets := make([]*model.Secret, len(secretsSlice))
	for i := range secretsSlice {
		secrets[i] = &secretsSlice[i]
		// Ensure Value is not set for list responses (already empty from query)
		secrets[i].Value = ""
	}

	return secrets, nil
}

// buildUpdateSQLParams groups parameters for building UPDATE SQL.
type buildUpdateSQLParams struct {
	id     string
	req    model.UpdateSecretRequest
	cipher *string
}

// buildUpdateSQL constructs the UPDATE statement for a secret and its args.
func (r *SecretRepo) buildUpdateSQL(params buildUpdateSQLParams) (string, []any, error) {
	req := params.req
	id := params.id
	cipher := params.cipher
	setParts := make([]string, 0, 8)
	args := make([]any, 0, 9)
	argIdx := 1

	if req.Name != nil {
		setParts = append(setParts, fmt.Sprintf("name = $%d", argIdx))
		args = append(args, *req.Name)
		argIdx++
	}
	if cipher != nil {
		setParts = append(setParts, fmt.Sprintf("value = $%d", argIdx))
		args = append(args, *cipher)
		argIdx++
	}

	// Refresh configuration fields
	if req.ProviderScriptPath != nil {
		setParts = append(setParts, fmt.Sprintf("provider_script_path = $%d", argIdx))
		args = append(args, *req.ProviderScriptPath)
		argIdx++
	}
	if req.EnvConfig != nil {
		setParts = append(setParts, fmt.Sprintf("env_config = $%d::jsonb", argIdx))
		args = append(args, *req.EnvConfig)
		argIdx++
	}
	if req.RefreshInterval != nil {
		setParts = append(setParts, fmt.Sprintf("refresh_interval = ($%d::bigint * interval '1 second')", argIdx))
		args = append(args, *req.RefreshInterval)
		argIdx++
	}
	if req.RefreshEnabled != nil {
		setParts = append(setParts, fmt.Sprintf("refresh_enabled = $%d", argIdx))
		args = append(args, *req.RefreshEnabled)
		argIdx++
	}

	if len(setParts) == 0 {
		return "", nil, errors.New("no fields to update")
	}

	args = append(args, id)
	query := "UPDATE secrets SET " + strings.Join(setParts, ", ") + fmt.Sprintf(` WHERE id = $%d
		RETURNING id, name, value, created_at, updated_at,
		          provider_script_path, env_config,
		          EXTRACT(EPOCH FROM refresh_interval)::bigint AS refresh_interval_seconds,
		          last_refreshed_at, last_refresh_status, last_refresh_error, refresh_enabled`, argIdx)
	return query, args, nil
}

// Update modifies a secret's name/value and refresh configuration, returning the updated secret with decrypted value.
func (r *SecretRepo) Update(ctx context.Context, id string, req model.UpdateSecretRequest) (*model.Secret, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}
	var err error
	var cipher *string
	if req.Value != nil {
		c, e := r.Enc.Encrypt([]byte(*req.Value))
		if e != nil {
			return nil, fmt.Errorf("encrypt: %w", e)
		}
		cipher = &c
	}
	query, args, err := r.buildUpdateSQL(buildUpdateSQLParams{
		id:     id,
		req:    req,
		cipher: cipher,
	})
	if err != nil {
		return nil, err
	}

	var out model.Secret
	err = pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, queryErr := conn.Query(ctx, query, args...)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()
		collected, collectErr := pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Secret])
		if collectErr != nil {
			return collectErr
		}
		out = collected
		return nil
	})

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrSecretNotFound
	}
	if err != nil {
		return nil, r.mapWriteErr(err, true)
	}

	// Decrypt value if present
	if out.Value != "" {
		pt, derr := r.Enc.Decrypt(out.Value)
		if derr != nil {
			return nil, fmt.Errorf("decrypt value: %w", derr)
		}
		out.Value = string(pt)
	}

	return &out, nil
}

// Delete removes a secret by ID.
func (r *SecretRepo) Delete(ctx context.Context, id string) (bool, error) {
	var affected int64
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		ct, err := conn.Exec(ctx, `DELETE FROM secrets WHERE id = $1`, id)
		if err != nil {
			return err
		}
		affected = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("delete secret: %w", err)
	}
	return affected > 0, nil
}

// FindDueForRefresh finds secrets that need to be refreshed based on their refresh_interval.
// Returns secrets where refresh_enabled=true AND (last_refreshed_at IS NULL OR last_refreshed_at + refresh_interval <= now).
func (r *SecretRepo) FindDueForRefresh(ctx context.Context, limit int) ([]*model.Secret, error) {
	if limit <= 0 {
		limit = 50
	}

	query := `
		SELECT id, name, value, created_at, updated_at,
		       provider_script_path, env_config,
		       EXTRACT(EPOCH FROM refresh_interval)::bigint AS refresh_interval_seconds,
		       last_refreshed_at, last_refresh_status, last_refresh_error, refresh_enabled
		FROM secrets
		WHERE refresh_enabled = TRUE
		  AND (last_refreshed_at IS NULL OR last_refreshed_at + refresh_interval <= now())
		ORDER BY
			CASE WHEN last_refreshed_at IS NULL THEN 0 ELSE 1 END,
			last_refreshed_at ASC,
			created_at ASC
		LIMIT $1
	`

	var secretsSlice []model.Secret
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, limit)
		if err != nil {
			return err
		}
		defer rows.Close()
		// Use pgx.CollectRows with pgx.RowToStructByName for automatic collection
		secretsSlice, err = pgx.CollectRows(rows, pgx.RowToStructByName[model.Secret])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("find due for refresh: %w", err)
	}

	// Convert slice to pointer slice and clear values (not needed for scheduling)
	secrets := make([]*model.Secret, len(secretsSlice))
	for i := range secretsSlice {
		secretsSlice[i].Value = "" // Don't decrypt value - not needed for scheduling refresh jobs
		secrets[i] = &secretsSlice[i]
	}

	return secrets, nil
}

// UpdateRefreshStatus updates the refresh status fields after a refresh attempt.
func (r *SecretRepo) UpdateRefreshStatus(ctx context.Context, params core.UpdateSecretRefreshStatusParams) error {
	query := `
		UPDATE secrets
		SET last_refreshed_at = $2,
		    last_refresh_status = $3,
		    last_refresh_error = $4,
		    updated_at = now()
		WHERE id = $1
	`

	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, query, params.SecretID, params.RefreshedAt, params.Status, params.ErrorMsg)
		return err
	})
	if err != nil {
		return fmt.Errorf("update refresh status: %w", err)
	}

	return nil
}

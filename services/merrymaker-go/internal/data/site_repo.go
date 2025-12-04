package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/data/database"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	// ErrSiteNotFound is returned when a site is not found.
	ErrSiteNotFound = errors.New("site not found")
	// ErrSiteNameExists is returned when attempting to create/update a site with a duplicate name.
	ErrSiteNameExists = errors.New("site name already exists")
)

// SiteRepo provides database operations for sites.
type SiteRepo struct {
	DB           *sql.DB
	timeProvider TimeProvider
}

// NewSiteRepo creates a new SiteRepo with real time provider.
func NewSiteRepo(db *sql.DB) *SiteRepo {
	return &SiteRepo{DB: db, timeProvider: &RealTimeProvider{}}
}

// NewSiteRepoWithTimeProvider creates a new SiteRepo with a custom time provider (useful for tests).
func NewSiteRepoWithTimeProvider(db *sql.DB, tp TimeProvider) *SiteRepo {
	return &SiteRepo{DB: db, timeProvider: tp}
}

// Create inserts a new site.
func (r *SiteRepo) Create(ctx context.Context, req *model.CreateSiteRequest) (*model.Site, error) {
	if req == nil {
		return nil, errors.New("create site request is required")
	}

	if err := req.Validate(); err != nil {
		return nil, err
	}

	// Default enabled to true if not specified (matches DB default)
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	alertMode := req.AlertMode
	if alertMode == "" {
		alertMode = model.SiteAlertModeActive
	}

	createdAt := r.timeProvider.Now().UTC()
	var out model.Site
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, `
			INSERT INTO sites (
				name, enabled, alert_mode, scope, http_alert_sink_id, last_enabled, last_run, run_every_minutes, source_id, created_at
			) VALUES (
				$1, $2, $3, $4, $5, $6, NULL, $7, $8, $9
			) RETURNING id, name, enabled, alert_mode, scope, http_alert_sink_id, last_enabled, last_run, run_every_minutes, source_id, created_at, updated_at
		`,
			strings.TrimSpace(req.Name),
			enabled,
			alertMode,
			req.Scope,
			req.HTTPAlertSinkID,
			// Set last_enabled at creation only when enabled is true
			func() *time.Time {
				if enabled {
					t := createdAt
					return &t
				}
				return nil
			}(),
			req.RunEveryMinutes,
			req.SourceID,
			createdAt,
		)
		if err != nil {
			return err
		}
		defer rows.Close()
		out, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Site])
		return err
	}); err != nil {
		return nil, r.mapWriteErr(err, false)
	}
	return &out, nil
}

// GetByID retrieves a site by ID.
func (r *SiteRepo) GetByID(ctx context.Context, id string) (*model.Site, error) {
	return r.getByQuery(ctx, siteGetByIDQuery, "failed to get site by ID", id)
}

// GetByName retrieves a site by name.
func (r *SiteRepo) GetByName(ctx context.Context, name string) (*model.Site, error) {
	return r.getByQuery(ctx, siteGetByNameQuery, "failed to get site by name", name)
}

// List retrieves sites with pagination.
func (r *SiteRepo) List(ctx context.Context, limit, offset int) ([]*model.Site, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	var rowsOut []model.Site
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, siteListQuery, limit, offset)
		if err != nil {
			return err
		}
		defer rows.Close()
		rowsOut, err = pgx.CollectRows(rows, pgx.RowToStructByName[model.Site])
		return err
	}); err != nil {
		return nil, fmt.Errorf("failed to list sites: %w", err)
	}

	res := make([]*model.Site, len(rowsOut))
	for i := range rowsOut {
		res[i] = &rowsOut[i]
	}
	return res, nil
}

// ListWithOptions retrieves sites with optional filters and sorting.
func (r *SiteRepo) ListWithOptions(
	ctx context.Context,
	opts model.SitesListOptions,
) ([]*model.Site, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	offset := max(opts.Offset, 0)

	queryOpts := r.buildSiteQueryOptions(opts, limit, offset)
	query, args := database.BuildListQuery(queryOpts)

	var rowsOut []model.Site
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		rowsOut, err = pgx.CollectRows(rows, pgx.RowToStructByName[model.Site])
		return err
	}); err != nil {
		return nil, fmt.Errorf("failed to list sites with options: %w", err)
	}
	res := make([]*model.Site, len(rowsOut))
	for i := range rowsOut {
		res[i] = &rowsOut[i]
	}
	return res, nil
}

// Update updates fields of a site. If Enabled is set to true, last_enabled is updated to now.
func (r *SiteRepo) Update(
	ctx context.Context,
	id string,
	req model.UpdateSiteRequest,
) (*model.Site, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	var out model.Site
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		setClause, args := r.buildUpdateClause(req)
		if setClause == "" {
			rows, err := conn.Query(ctx, `
					SELECT id, name, enabled, alert_mode, scope, http_alert_sink_id, last_enabled, last_run, run_every_minutes, source_id, created_at, updated_at
					FROM sites WHERE id = $1`, id)
			if err != nil {
				return err
			}
			defer rows.Close()
			var e error
			out, e = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Site])
			return e
		}
		args = append(args, id)
		query := "UPDATE sites SET " + setClause + " WHERE id = $" + strconv.Itoa(
			len(args),
		) + " RETURNING id, name, enabled, alert_mode, scope, http_alert_sink_id, last_enabled, last_run, run_every_minutes, source_id, created_at, updated_at"
		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		var e error
		out, e = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Site])
		return e
	})
	if err != nil {
		return nil, r.mapWriteErr(err, true)
	}
	return &out, nil
}

// buildUpdateClause builds the SQL SET clause and args for updating a site based on the request.
func (r *SiteRepo) buildUpdateClause(req model.UpdateSiteRequest) (string, []any) {
	setParts := make([]string, 0, 6)
	args := make([]any, 0, 8)
	nextIdx := func() int { return len(args) + 1 }

	if req.Name != nil {
		setParts = append(setParts, fmt.Sprintf("name = $%d", nextIdx()))
		args = append(args, strings.TrimSpace(*req.Name))
	}
	if req.Enabled != nil {
		setParts = append(setParts, fmt.Sprintf("enabled = $%d", nextIdx()))
		args = append(args, *req.Enabled)
		if *req.Enabled {
			setParts = append(setParts, fmt.Sprintf("last_enabled = $%d", nextIdx()))
			args = append(args, r.timeProvider.Now().UTC())
		}
	}
	if req.AlertMode != nil {
		setParts = append(setParts, fmt.Sprintf("alert_mode = $%d", nextIdx()))
		args = append(args, *req.AlertMode)
	}
	if req.Scope != nil {
		setParts = append(setParts, fmt.Sprintf("scope = $%d", nextIdx()))
		args = append(args, *req.Scope)
	}
	if req.HTTPAlertSinkID != nil {
		if strings.TrimSpace(*req.HTTPAlertSinkID) == "" {
			setParts = append(setParts, "http_alert_sink_id = NULL")
		} else {
			setParts = append(setParts, fmt.Sprintf("http_alert_sink_id = $%d", nextIdx()))
			args = append(args, *req.HTTPAlertSinkID)
		}
	}
	if req.RunEveryMinutes != nil {
		setParts = append(setParts, fmt.Sprintf("run_every_minutes = $%d", nextIdx()))
		args = append(args, *req.RunEveryMinutes)
	}
	if req.SourceID != nil {
		setParts = append(setParts, fmt.Sprintf("source_id = $%d", nextIdx()))
		args = append(args, *req.SourceID)
	}

	if len(setParts) == 0 {
		return "", nil
	}
	return strings.Join(setParts, ", "), args
}

// Delete deletes a site by ID.
func (r *SiteRepo) Delete(ctx context.Context, id string) (bool, error) {
	var rows int64
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		ct, err := conn.Exec(ctx, `DELETE FROM sites WHERE id = $1`, id)
		if err != nil {
			return err
		}
		rows = ct.RowsAffected()
		return nil
	})
	if err != nil {
		return false, fmt.Errorf("failed to delete site: %w", err)
	}
	return rows > 0, nil
}

// --- helpers ---

// SQL query constants for static queries (no dynamic WHERE/ORDER BY).
// Using constants avoids runtime query building overhead for hot paths.
const (
	siteGetByIDQuery = `
		SELECT id, name, enabled, alert_mode, scope, http_alert_sink_id, last_enabled, last_run,
		       run_every_minutes, source_id, created_at, updated_at
		FROM sites
		WHERE id = $1`

	siteGetByNameQuery = `
		SELECT id, name, enabled, alert_mode, scope, http_alert_sink_id, last_enabled, last_run,
		       run_every_minutes, source_id, created_at, updated_at
		FROM sites
		WHERE name = $1`

	siteListQuery = `
		SELECT id, name, enabled, alert_mode, scope, http_alert_sink_id, last_enabled, last_run,
		       run_every_minutes, source_id, created_at, updated_at
		FROM sites
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2`
)

// siteColumns returns the standard column list for site queries.
// Used by dynamic queries that need to build column lists at runtime.
func siteColumns() []string {
	return []string{
		"id",
		"name",
		"enabled",
		"alert_mode",
		"scope",
		"http_alert_sink_id",
		"last_enabled",
		"last_run",
		"run_every_minutes",
		"source_id",
		"created_at",
		"updated_at",
	}
}

// buildSiteQueryOptions builds query options for site listing with filters and sorting.
func (r *SiteRepo) buildSiteQueryOptions(
	opts model.SitesListOptions,
	limit, offset int,
) *database.ListQueryOptions {
	// Start with base options
	queryOpts := []database.ListQueryOption{
		database.WithColumns(siteColumns()...),
		database.WithLimit(limit),
		database.WithOffset(offset),
	}

	// Add filters
	if opts.Q != nil && strings.TrimSpace(*opts.Q) != "" {
		queryOpts = append(queryOpts, database.WithCondition(
			database.WhereCond("name", database.ILike, "%"+strings.TrimSpace(*opts.Q)+"%"),
		))
	}
	if opts.Enabled != nil {
		queryOpts = append(queryOpts, database.WithCondition(
			database.WhereCond("enabled", database.Equal, *opts.Enabled),
		))
	}
	if opts.Scope != nil && strings.TrimSpace(*opts.Scope) != "" {
		queryOpts = append(queryOpts, database.WithCondition(
			database.WhereCond("scope", database.Equal, strings.TrimSpace(*opts.Scope)),
		))
	}

	// Add sorting with defaults
	sortCol, sortDir := validateSortOptions(opts.Sort, opts.Dir)
	queryOpts = append(queryOpts, database.WithOrderBy(sortCol, sortDir))

	return database.NewListQueryOptions("sites", queryOpts...)
}

// validateSortOptions validates and returns safe sort column and direction.
func validateSortOptions(sort, dir string) (string, string) {
	sortCol := "created_at"
	sortDir := sortDirDesc

	if sort != "" {
		allowedSorts := map[string]string{
			"name":       "name",
			"created_at": "created_at",
		}
		if validSort, ok := allowedSorts[strings.ToLower(strings.TrimSpace(sort))]; ok {
			sortCol = validSort
		}
	}
	if dir != "" {
		allowedDirs := map[string]string{
			"asc":  sortDirAsc,
			"desc": sortDirDesc,
		}
		if validDir, ok := allowedDirs[strings.ToLower(strings.TrimSpace(dir))]; ok {
			sortDir = validDir
		}
	}
	return sortCol, sortDir
}

// getByQuery is a helper function to execute a query and return a single site.
// Uses variadic args to avoid slice allocation at call sites.
func (r *SiteRepo) getByQuery(
	ctx context.Context,
	q string,
	errMsg string,
	args ...any,
) (*model.Site, error) {
	var site model.Site
	err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, q, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		site, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Site])
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrSiteNotFound
		}
		return nil, fmt.Errorf("%s: %w", errMsg, err)
	}
	return &site, nil
}

func (r *SiteRepo) mapWriteErr(err error, includeNotFound bool) error {
	if err == nil {
		return nil
	}
	if includeNotFound && errors.Is(err, pgx.ErrNoRows) {
		return ErrSiteNotFound
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrSiteNameExists
	}
	return err
}

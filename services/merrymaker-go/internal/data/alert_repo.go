package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data/database"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// ErrAlertNotFound is returned when an alert is not found.
var ErrAlertNotFound = errors.New("alert not found")

// AlertRepo provides database operations for alert management.
type AlertRepo struct {
	DB           *sql.DB
	timeProvider TimeProvider
}

// NewAlertRepo creates a new AlertRepo instance with the given database connection.
func NewAlertRepo(db *sql.DB) *AlertRepo {
	return &AlertRepo{
		DB:           db,
		timeProvider: &RealTimeProvider{},
	}
}

// alertColumns defines the column list for Alert SELECT queries to ensure consistent field mapping.
const alertColumns = `id, site_id, rule_id, rule_type, severity, title, description, event_context, metadata, delivery_status, fired_at, resolved_at, resolved_by, created_at`

// alertColumnsWithSiteName defines the column list for Alert SELECT queries with site name JOIN.
const alertColumnsWithSiteName = `a.id, a.site_id, a.rule_id, a.rule_type, a.severity, a.title, a.description, a.event_context, a.metadata, a.delivery_status, a.fired_at, a.resolved_at, a.resolved_by, a.created_at, COALESCE(s.name, '') as site_name, COALESCE(s.alert_mode, '') as site_alert_mode`

// getAlertColumnList returns a slice of alert column names for use with the query builder.
func getAlertColumnList() []string {
	return []string{
		"id", "site_id", "rule_id", "rule_type", "severity", "title", "description",
		"event_context", "metadata", "delivery_status", "fired_at", "resolved_at", "resolved_by", "created_at",
	}
}

const (
	sortDirAsc         = "ASC"
	sortDirDesc        = "DESC"
	sortFieldCreatedAt = "created_at"
	sortFieldFiredAt   = "fired_at"
	sortFieldSeverity  = "severity"
)

// handleCreateError handles database errors during alert creation.
func (r *AlertRepo) handleCreateError(err error) error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		return fmt.Errorf("create alert: %w", err)
	}

	if pgErr.Code == "23503" && strings.Contains(pgErr.Detail, "site_id") {
		return errors.New("site not found")
	}

	return fmt.Errorf("create alert: %w", err)
}

// Create creates a new alert with the given request parameters.
func (r *AlertRepo) Create(
	ctx context.Context,
	req *model.CreateAlertRequest,
) (*model.Alert, error) {
	if req == nil {
		return nil, errors.New("create alert request is required")
	}

	req.Normalize()
	if err := req.Validate(); err != nil {
		return nil, err
	}

	now := r.timeProvider.Now()
	firedAt := now
	if req.FiredAt != nil {
		firedAt = *req.FiredAt
	}

	// Set default empty JSON if not provided
	eventContext := req.EventContext
	if eventContext == nil {
		eventContext = []byte("{}")
	}
	metadata := req.Metadata
	if metadata == nil {
		metadata = []byte("{}")
	}

	query := `
		INSERT INTO alerts (site_id, rule_id, rule_type, severity, title, description, event_context, metadata, delivery_status, fired_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING ` + alertColumns

	var alert model.Alert
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, query,
			req.SiteID, req.RuleID, req.RuleType, req.Severity, req.Title, req.Description,
			eventContext, metadata, req.DeliveryStatus, firedAt, now,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		alert, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Alert])
		return err
	})
	if err != nil {
		return nil, r.handleCreateError(err)
	}

	return &alert, nil
}

// UpdateDeliveryStatus updates an alert's delivery status and returns the updated alert.
func (r *AlertRepo) UpdateDeliveryStatus(
	ctx context.Context,
	params core.UpdateAlertDeliveryStatusParams,
) (*model.Alert, error) {
	if params.ID == "" {
		return nil, errors.New("alert id is required")
	}
	if !params.Status.Valid() {
		return nil, errors.New("invalid delivery status")
	}
	if _, err := uuid.Parse(params.ID); err != nil {
		return nil, ErrAlertNotFound
	}

	var alert model.Alert
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, `
			UPDATE alerts
			SET delivery_status = $1
			WHERE id = $2
			RETURNING `+alertColumns,
			params.Status,
			params.ID,
		)
		if err != nil {
			return err
		}
		defer rows.Close()

		alert, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Alert])
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlertNotFound
		}
		return nil, fmt.Errorf("update alert delivery status: %w", err)
	}

	return &alert, nil
}

// GetByID retrieves an alert by its ID.
func (r *AlertRepo) GetByID(ctx context.Context, id string) (*model.Alert, error) {
	query := `SELECT ` + alertColumns + ` FROM alerts WHERE id = $1`

	var alert model.Alert
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, query, id)
		if err != nil {
			return err
		}
		defer rows.Close()

		alert, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Alert])
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlertNotFound
		}
		return nil, fmt.Errorf("get alert by id: %w", err)
	}

	return &alert, nil
}

// normalizePagination normalizes limit and offset values for pagination.
func (r *AlertRepo) normalizePagination(limit, offset int) (int, int) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}

	if offset < 0 {
		offset = 0
	}

	return limit, offset
}

// validateSortOptions validates and returns safe sort column and direction.
// Returns base column names (without table aliases) that are valid for both
// List and ListWithSiteNames methods. The ListWithSiteNames method will
// prefix returned columns with "a." for the alerts table alias.
func (r *AlertRepo) validateSortOptions(sort, dir string) (string, string) {
	// Validate sort field - allow fired_at, created_at, and severity
	// Note: These are base alert table columns only. Joined columns like
	// site_name would need special handling if sorting by them is required.
	switch sort {
	case sortFieldFiredAt, sortFieldCreatedAt, sortFieldSeverity:
		// Valid sort fields
	default:
		sort = sortFieldFiredAt // Default to fired_at
	}

	// Validate and normalize direction (case-insensitive)
	if strings.EqualFold(dir, "asc") {
		dir = sortDirAsc
	} else {
		dir = sortDirDesc // Default to DESC
	}

	return sort, dir
}

// buildListWhereClauseWithAlias builds the WHERE clause and arguments for the List query with table aliases.
func (r *AlertRepo) buildListWhereClauseWithAlias(
	opts *model.AlertListOptions,
) (string, []any, int) {
	if opts == nil {
		opts = &model.AlertListOptions{}
	}

	var conditions []string
	var args []any
	argIndex := 1

	if opts.SiteID != nil {
		conditions = append(conditions, fmt.Sprintf("a.site_id = $%d", argIndex))
		args = append(args, *opts.SiteID)
		argIndex++
	}

	if opts.RuleType != nil {
		conditions = append(conditions, fmt.Sprintf("a.rule_type = $%d", argIndex))
		args = append(args, *opts.RuleType)
		argIndex++
	}

	if opts.Severity != nil {
		conditions = append(conditions, fmt.Sprintf("a.severity = $%d", argIndex))
		args = append(args, *opts.Severity)
		argIndex++
	}

	if opts.Unresolved {
		conditions = append(conditions, "a.resolved_at IS NULL")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	return whereClause, args, argIndex
}

// List retrieves a list of alerts with the given options using the query builder.
func (r *AlertRepo) List(ctx context.Context, opts *model.AlertListOptions) ([]*model.Alert, error) {
	if opts == nil {
		opts = &model.AlertListOptions{}
	}

	limit, offset := r.normalizePagination(opts.Limit, opts.Offset)
	sortCol, sortDir := r.validateSortOptions(opts.Sort, opts.Dir)

	// Build query options using the query builder
	queryOpts := []database.ListQueryOption{
		database.WithColumns(getAlertColumnList()...),
		database.WithLimit(limit),
		database.WithOffset(offset),
		database.WithOrderBy(sortCol, sortDir),
	}

	// Add filter conditions
	if opts.SiteID != nil {
		queryOpts = append(queryOpts, database.WithCondition(
			database.WhereCond("site_id", database.Equal, *opts.SiteID),
		))
	}
	if opts.RuleType != nil {
		queryOpts = append(queryOpts, database.WithCondition(
			database.WhereCond("rule_type", database.Equal, *opts.RuleType),
		))
	}
	if opts.Severity != nil {
		queryOpts = append(queryOpts, database.WithCondition(
			database.WhereCond("severity", database.Equal, *opts.Severity),
		))
	}
	if opts.Unresolved {
		queryOpts = append(queryOpts, database.WithCondition(
			database.WhereRawCond("resolved_at IS NULL"),
		))
	}

	query, args := database.BuildListQuery(database.NewListQueryOptions("alerts", queryOpts...))

	// Add secondary sort key for deterministic ordering
	// Replace "ORDER BY column DIR" with "ORDER BY column DIR, id DESC"
	if strings.Contains(query, "ORDER BY") {
		query = strings.Replace(query, fmt.Sprintf("ORDER BY %s %s", sortCol, sortDir),
			fmt.Sprintf("ORDER BY %s %s, id DESC", sortCol, sortDir), 1)
	}

	var alerts []*model.Alert
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		alerts, err = pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[model.Alert])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}

	return alerts, nil
}

// ListWithSiteNames retrieves a list of alerts with site names using a JOIN query.
// This method eliminates N+1 queries by fetching site names in a single query.
func (r *AlertRepo) ListWithSiteNames(
	ctx context.Context,
	opts *model.AlertListOptions,
) ([]*model.AlertWithSiteName, error) {
	if opts == nil {
		opts = &model.AlertListOptions{}
	}

	limit, offset := r.normalizePagination(opts.Limit, opts.Offset)
	sortCol, sortDir := r.validateSortOptions(opts.Sort, opts.Dir)

	// Build WHERE clause and arguments manually since we need JOIN support
	whereClause, args, argIndex := r.buildListWhereClauseWithAlias(opts)

	// Build the query with JOIN
	var queryBuilder strings.Builder
	queryBuilder.WriteString("SELECT ")
	queryBuilder.WriteString(alertColumnsWithSiteName)
	queryBuilder.WriteString(" FROM alerts a LEFT JOIN sites s ON a.site_id = s.id ")
	queryBuilder.WriteString(whereClause)
	queryBuilder.WriteString(fmt.Sprintf(" ORDER BY a.%s %s, a.id DESC", sortCol, sortDir))
	queryBuilder.WriteString(" LIMIT $")
	queryBuilder.WriteString(strconv.Itoa(argIndex))
	queryBuilder.WriteString(" OFFSET $")
	queryBuilder.WriteString(strconv.Itoa(argIndex + 1))
	query := queryBuilder.String()

	args = append(args, limit, offset)

	var alerts []*model.AlertWithSiteName
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()

		alerts, err = pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[model.AlertWithSiteName])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("list alerts with site names: %w", err)
	}

	return alerts, nil
}

// Delete deletes an alert by its ID.
func (r *AlertRepo) Delete(ctx context.Context, id string) (bool, error) {
	query := `DELETE FROM alerts WHERE id = $1`

	result, err := r.DB.ExecContext(ctx, query, id)
	if err != nil {
		return false, fmt.Errorf("delete alert: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}

// Stats retrieves alert statistics, optionally filtered by site ID.
func (r *AlertRepo) Stats(ctx context.Context, siteID *string) (*model.AlertStats, error) {
	whereClause := ""
	var args []any
	if siteID != nil {
		whereClause = "WHERE site_id = $1"
		args = append(args, *siteID)
	}

	// Build query with safe string concatenation instead of fmt.Sprintf for SQL
	query := `SELECT
		COUNT(*) as total,
		COUNT(CASE WHEN severity = 'critical' THEN 1 END) as critical,
		COUNT(CASE WHEN severity = 'high' THEN 1 END) as high,
		COUNT(CASE WHEN severity = 'medium' THEN 1 END) as medium,
		COUNT(CASE WHEN severity = 'low' THEN 1 END) as low,
		COUNT(CASE WHEN severity = 'info' THEN 1 END) as info,
		COUNT(CASE WHEN resolved_at IS NULL THEN 1 END) as unresolved
	FROM alerts ` + whereClause

	var stats model.AlertStats
	err := r.DB.QueryRowContext(ctx, query, args...).Scan(
		&stats.Total, &stats.Critical, &stats.High, &stats.Medium,
		&stats.Low, &stats.Info, &stats.Unresolved,
	)
	if err != nil {
		return nil, fmt.Errorf("get alert stats: %w", err)
	}

	return &stats, nil
}

// Resolve marks an alert as resolved by setting resolved_at and resolved_by.
func (r *AlertRepo) Resolve(
	ctx context.Context,
	params core.ResolveAlertParams,
) (*model.Alert, error) {
	now := r.timeProvider.Now()

	query := `
		UPDATE alerts
		SET resolved_at = $1, resolved_by = $2
		WHERE id = $3 AND resolved_at IS NULL
		RETURNING ` + alertColumns

	var alert model.Alert
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, query, now, params.ResolvedBy, params.ID)
		if err != nil {
			return err
		}
		defer rows.Close()

		alert, err = pgx.CollectOneRow(rows, pgx.RowToStructByName[model.Alert])
		return err
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrAlertNotFound
		}
		return nil, fmt.Errorf("resolve alert: %w", err)
	}

	return &alert, nil
}

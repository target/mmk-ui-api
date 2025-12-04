package data

import (
	"context"
	"errors"
	"fmt"

	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/jackc/pgx/v5"
)

// ListBySource returns jobs associated with a specific source, ordered by created_at DESC.
func (r *JobRepo) ListBySource(ctx context.Context, params model.JobListBySourceOptions) ([]*model.Job, error) {
	if params.SourceID == "" {
		return nil, errors.New("source id is required")
	}
	if params.Limit <= 0 {
		params.Limit = 50 // Default limit
	}
	if params.Limit > 1000 {
		params.Limit = 1000 // Max limit
	}
	if params.Offset < 0 {
		params.Offset = 0
	}

	query := `
		SELECT ` + jobColumns + `
		FROM jobs
		WHERE source_id = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2 OFFSET $3
	`

	var result []*model.Job
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, params.SourceID, params.Limit, params.Offset)
		if err != nil {
			return fmt.Errorf("query jobs by source: %w", err)
		}
		defer rows.Close()

		vals, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[model.Job])
		if err != nil {
			return fmt.Errorf("collect jobs by source: %w", err)
		}

		result = vals
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// buildJobsWithEventCountsQuery constructs the SQL query and args for jobs with event counts.
type jobFilterQueryBuilder struct {
	query  string
	args   []any
	argIdx int
}

func (b *jobFilterQueryBuilder) addFilter(condition string, value any) {
	if value != nil {
		b.query += fmt.Sprintf(" AND %s = $%d", condition, b.argIdx)
		b.args = append(b.args, value)
		b.argIdx++
	}
}

func buildJobsWithEventCountsQuery(opts model.JobListBySiteOptions) (string, []any) {
	builder := &jobFilterQueryBuilder{
		query: `
		SELECT
			j.id, j.type, j.status, j.priority, j.payload, j.metadata,
			j.session_id, j.site_id, j.source_id, j.is_test,
			j.scheduled_at, j.started_at, j.completed_at,
			j.retry_count, j.max_retries, j.last_error, j.lease_expires_at,
			j.created_at, j.updated_at,
			COALESCE(jm.event_count, 0) as event_count,
			COALESCE(s.name, '') as site_name
		FROM jobs j
		LEFT JOIN sites s ON s.id = j.site_id
		LEFT JOIN job_meta jm ON jm.job_id = j.id
		WHERE 1=1`,
		args:   []any{},
		argIdx: 1,
	}

	if opts.SiteID != nil && *opts.SiteID != "" {
		builder.addFilter("j.site_id", *opts.SiteID)
	}
	if opts.Status != nil && *opts.Status != "" {
		builder.addFilter("j.status", *opts.Status)
	}
	if opts.Type != nil && *opts.Type != "" {
		builder.addFilter("j.type", *opts.Type)
	}

	builder.query += `
		ORDER BY j.created_at DESC, j.id DESC`

	return builder.query, builder.args
}

// ListBySiteWithFilters returns jobs with optional filters and event counts for UI display.
func (r *JobRepo) ListBySiteWithFilters(
	ctx context.Context,
	opts model.JobListBySiteOptions,
) ([]*model.JobWithEventCount, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 1000 {
		limit = 1000 // Max limit
	}
	offset := max(opts.Offset, 0)

	query, args := buildJobsWithEventCountsQuery(opts)
	return r.executeJobListWithEventCountQuery(ctx, jobListWithEventCountParams{
		Query:  query,
		Args:   args,
		Limit:  limit,
		Offset: offset,
	})
}

// ListRecentByType returns the most recent jobs of a given type, ordered by created_at DESC.
func (r *JobRepo) ListRecentByType(ctx context.Context, jobType model.JobType, limit int) ([]*model.Job, error) {
	if limit <= 0 {
		limit = 5 // sensible default for dashboard
	}
	if limit > 1000 {
		limit = 1000 // cap to prevent large scans
	}
	query := `
		SELECT ` + jobColumns + `
		FROM jobs
		WHERE type = $1
		ORDER BY created_at DESC, id DESC
		LIMIT $2
	`

	var result []*model.Job
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, string(jobType), limit)
		if err != nil {
			return fmt.Errorf("query jobs by type: %w", err)
		}
		defer rows.Close()

		vals, err := pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[model.Job])
		if err != nil {
			return fmt.Errorf("collect jobs: %w", err)
		}
		result = vals
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// ListRecentByTypeWithSiteNames returns the most recent jobs of a given type with site names.
func (r *JobRepo) ListRecentByTypeWithSiteNames(
	ctx context.Context,
	jobType model.JobType,
	limit int,
) ([]*model.JobWithEventCount, error) {
	if limit <= 0 {
		limit = 5 // sensible default for dashboard
	}
	if limit > 1000 {
		limit = 1000 // cap to prevent large scans
	}

	query := `
		SELECT
			j.id, j.type, j.status, j.priority, j.payload, j.metadata,
			j.session_id, j.site_id, j.source_id, j.is_test,
			j.scheduled_at, j.started_at, j.completed_at,
			j.retry_count, j.max_retries, j.last_error, j.lease_expires_at,
			j.created_at, j.updated_at,
			COALESCE(jm.event_count, 0) as event_count,
			COALESCE(s.name, '') as site_name
		FROM jobs j
		LEFT JOIN sites s ON s.id = j.site_id
		LEFT JOIN job_meta jm ON jm.job_id = j.id
		WHERE j.type = $1 AND NOT j.is_test
		ORDER BY j.created_at DESC, j.id DESC
		LIMIT $2
	`

	var result []*model.JobWithEventCount
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, string(jobType), limit)
		if err != nil {
			return fmt.Errorf("query jobs by type=%s limit=%d: %w", jobType, limit, err)
		}
		defer rows.Close()

		result, err = pgx.CollectRows(rows, pgx.RowToAddrOfStructByName[model.JobWithEventCount])
		if err != nil {
			return fmt.Errorf("collect jobs with site names: %w", err)
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

// CountBySource returns the total number of jobs for a given source.
func (r *JobRepo) CountBySource(ctx context.Context, sourceID string, includeTests bool) (int, error) {
	var n int
	row := r.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM jobs
		WHERE source_id = $1 AND (NOT is_test OR $2)
	`, sourceID, includeTests)
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count jobs by source: %w", err)
	}
	return n, nil
}

// CountBrowserBySource returns the number of browser jobs for a given source.
func (r *JobRepo) CountBrowserBySource(ctx context.Context, sourceID string, includeTests bool) (int, error) {
	var n int
	row := r.DB.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM jobs
		WHERE source_id = $1 AND type = $2 AND (NOT is_test OR $3)
	`, sourceID, string(model.JobTypeBrowser), includeTests)
	if err := row.Scan(&n); err != nil {
		return 0, fmt.Errorf("count browser jobs by source: %w", err)
	}
	return n, nil
}

// CountAggregatesBySources returns total and browser counts for many sources in one query.
func (r *JobRepo) CountAggregatesBySources(
	ctx context.Context,
	ids []string,
	includeTests bool,
) (map[string]model.SourceJobCounts, error) {
	if len(ids) == 0 {
		return map[string]model.SourceJobCounts{}, nil
	}
	res := make(map[string]model.SourceJobCounts, len(ids))
	query := `
		SELECT source_id,
		       COUNT(*) AS total,
		       COUNT(*) FILTER (WHERE type = $2) AS browser
		FROM jobs
		WHERE source_id = ANY($1) AND (NOT is_test OR $3)
		GROUP BY source_id
	`
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, ids, string(model.JobTypeBrowser), includeTests)
		if err != nil {
			return fmt.Errorf("count aggregates: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			var total, browser int
			if scanErr := rows.Scan(&id, &total, &browser); scanErr != nil {
				return fmt.Errorf("scan: %w", scanErr)
			}
			res[id] = model.SourceJobCounts{Total: total, Browser: browser}
		}
		return rows.Err()
	}); err != nil {
		return nil, err
	}
	return res, nil
}

// jobListWithEventCountParams groups parameters for executing job list queries with pagination.
type jobListWithEventCountParams struct {
	Query  string
	Args   []any
	Limit  int
	Offset int
}

// executeJobListWithEventCountQuery executes a job list query and returns JobWithEventCount results.
func (r *JobRepo) executeJobListWithEventCountQuery(
	ctx context.Context,
	p jobListWithEventCountParams,
) ([]*model.JobWithEventCount, error) {
	argIdx := len(p.Args) + 1
	query := p.Query + fmt.Sprintf(" LIMIT $%d OFFSET $%d", argIdx, argIdx+1)
	args := make([]any, len(p.Args), len(p.Args)+2)
	copy(args, p.Args)
	args = append(args, p.Limit, p.Offset)

	var result []*model.JobWithEventCount
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(conn *pgx.Conn) error {
		rows, err := conn.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("query jobs with filters: %w", err)
		}
		defer rows.Close()

		vals, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.JobWithEventCount])
		if err != nil {
			return fmt.Errorf("collect jobs with event counts: %w", err)
		}
		result = make([]*model.JobWithEventCount, len(vals))
		for i := range vals {
			result[i] = &vals[i]
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// buildJobListQuery constructs the SQL query and args for the global job list with filtering.
func buildJobListQuery(opts *model.JobListOptions) (string, []any) {
	if opts == nil {
		opts = &model.JobListOptions{}
	}

	builder := &jobFilterQueryBuilder{
		query: `
		SELECT
			j.id, j.type, j.status, j.priority, j.payload, j.metadata,
			j.session_id, j.site_id, j.source_id, j.is_test,
			j.scheduled_at, j.started_at, j.completed_at,
			j.retry_count, j.max_retries, j.last_error, j.lease_expires_at,
			j.created_at, j.updated_at,
			COALESCE(jm.event_count, 0) as event_count,
			COALESCE(s.name, '') as site_name
		FROM jobs j
		LEFT JOIN sites s ON s.id = j.site_id
		LEFT JOIN job_meta jm ON jm.job_id = j.id
		WHERE 1=1`,
		args:   []any{},
		argIdx: 1,
	}

	addJobListFilters(builder, opts)
	addJobListSorting(builder, opts)
	return builder.query, builder.args
}

// addJobListFilters adds filter conditions to the query builder.
func addJobListFilters(builder *jobFilterQueryBuilder, opts *model.JobListOptions) {
	if opts == nil {
		return
	}

	if opts.Status != nil {
		builder.addFilter("j.status", string(*opts.Status))
	}
	if opts.Type != nil {
		builder.addFilter("j.type", string(*opts.Type))
	}
	if opts.SiteID != nil && *opts.SiteID != "" {
		builder.addFilter("j.site_id", *opts.SiteID)
	}
	if opts.IsTest != nil {
		builder.addFilter("j.is_test", *opts.IsTest)
	}
}

// addJobListSorting adds sorting to the query builder.
func addJobListSorting(builder *jobFilterQueryBuilder, opts *model.JobListOptions) {
	if opts == nil {
		opts = &model.JobListOptions{}
	}

	sortBy := opts.SortBy
	if sortBy == "" {
		sortBy = "created_at"
	}
	sortOrder := opts.SortOrder
	if sortOrder == "" {
		sortOrder = "desc"
	}

	validSortFields := map[string]string{
		"created_at": "j.created_at",
		"status":     "j.status",
		"type":       "j.type",
	}

	dbField, ok := validSortFields[sortBy]
	if !ok {
		builder.query += " ORDER BY j.created_at DESC, j.id DESC"
		return
	}

	if sortOrder == "asc" {
		builder.query += fmt.Sprintf(" ORDER BY %s ASC, j.id ASC", dbField)
		return
	}

	builder.query += fmt.Sprintf(" ORDER BY %s DESC, j.id DESC", dbField)
}

// List returns all jobs with optional filtering and event counts for admin view.
func (r *JobRepo) List(ctx context.Context, opts *model.JobListOptions) ([]*model.JobWithEventCount, error) {
	if opts == nil {
		opts = &model.JobListOptions{}
	}

	limit := opts.Limit
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 1000 {
		limit = 1000 // Max limit
	}
	offset := max(opts.Offset, 0)

	query, args := buildJobListQuery(opts)
	return r.executeJobListWithEventCountQuery(ctx, jobListWithEventCountParams{
		Query:  query,
		Args:   args,
		Limit:  limit,
		Offset: offset,
	})
}

// Package data provides database access layer and repository implementations for the merrymaker job system.
package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const (
	// Default sort field for events.
	defaultEventSortField = "created_at"
	sortByEventType       = "event_type"
)

// EventRepo provides database operations for event management.
type EventRepo struct{ DB *sql.DB }

type jobMetaDelta struct {
	JobID string
	Delta int
}

func applyJobMetaDelta(ctx context.Context, tx pgx.Tx, delta jobMetaDelta) error {
	if delta.Delta <= 0 || strings.TrimSpace(delta.JobID) == "" {
		return nil
	}

	_, err := tx.Exec(ctx, `
		INSERT INTO job_meta (job_id, event_count, updated_at)
		VALUES ($1, $2, now())
		ON CONFLICT (job_id) DO UPDATE
		SET event_count = job_meta.event_count + EXCLUDED.event_count,
		    updated_at = now()
	`, delta.JobID, delta.Delta)
	if err != nil {
		return fmt.Errorf("update job_meta event_count: %w", err)
	}
	return nil
}

type insertEventsOptions struct {
	Req           model.BulkEventRequest
	Process       bool
	ShouldProcess map[int]bool
}

func (r *EventRepo) runEventTx(
	ctx context.Context,
	fn func(pgx.Tx) (int, error),
) (int, error) {
	var created int
	err := pgxutil.WithPgxTx(ctx, r.DB, pgxutil.TxConfig{
		Opts: &sql.TxOptions{
			Isolation: sql.LevelReadCommitted,
			ReadOnly:  false,
		},
		Fn: func(tx pgx.Tx) error {
			var execErr error
			created, execErr = fn(tx)
			return execErr
		},
	})
	return created, err
}

func (r *EventRepo) insertEventsWithBatch(
	ctx context.Context,
	tx pgx.Tx,
	opts insertEventsOptions,
) (int, error) {
	batch := &pgx.Batch{}

	for i, e := range opts.Req.Events {
		p := 0
		if e.Priority != nil {
			p = *e.Priority
		}
		metadata := normalizeEventMetadata(e.Metadata)

		shouldProcessVal := opts.Process
		if opts.ShouldProcess != nil {
			shouldProcessVal = opts.ShouldProcess[i]
		}

		batch.Queue(`
				INSERT INTO events(
					session_id,
					source_job_id,
					event_type,
					event_data,
					metadata,
					storage_key,
					priority,
					should_process)
				VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			`, opts.Req.SessionID, opts.Req.SourceJobID, e.Type, e.Data, metadata, e.StorageKey, p, shouldProcessVal)
	}

	br := tx.SendBatch(ctx, batch)

	created := 0
	for i := range opts.Req.Events {
		if _, err := br.Exec(); err != nil {
			return 0, fmt.Errorf("failed to insert event %d: %w", i, err)
		}
		created++
	}

	if cerr := br.Close(); cerr != nil {
		return 0, fmt.Errorf("batch close: %w", cerr)
	}

	if opts.Req.SourceJobID != nil && created > 0 {
		if err := applyJobMetaDelta(ctx, tx, jobMetaDelta{
			JobID: *opts.Req.SourceJobID,
			Delta: created,
		}); err != nil {
			return 0, err
		}
	}

	return created, nil
}

// BulkInsert inserts multiple events into the database in a single transaction using pgx batching for optimal performance.
// and then the jobs should be processed.
// Examples:
// - Have we seen requests to this domain before?.
// - Is the domain a known IOC?.
// - have we seen this file before (static analysis).
func (r *EventRepo) BulkInsert(
	ctx context.Context,
	req model.BulkEventRequest,
	process bool,
) (int, error) {
	created, err := r.runEventTx(ctx, func(tx pgx.Tx) (int, error) {
		return r.insertEventsWithBatch(ctx, tx, insertEventsOptions{
			Req:     req,
			Process: process,
		})
	})
	if err != nil {
		return 0, fmt.Errorf("bulk insert transaction failed: %w", err)
	}
	return created, nil
}

// BulkInsertCopy inserts multiple events using PostgreSQL COPY for maximum performance with large batches.
// This method is more efficient than BulkInsert for very large batches (>1000 events) but provides less
// granular error reporting. Use BulkInsert for smaller batches where individual error reporting is important.
func (r *EventRepo) BulkInsertCopy(
	ctx context.Context,
	req model.BulkEventRequest,
	process bool,
) (int, error) {
	created, err := r.runEventTx(ctx, func(tx pgx.Tx) (int, error) {
		rows := make([][]any, 0, len(req.Events))
		for _, e := range req.Events {
			p := 0
			if e.Priority != nil {
				p = *e.Priority
			}
			metadata := normalizeEventMetadata(e.Metadata)
			rows = append(rows, []any{
				req.SessionID,
				req.SourceJobID,
				e.Type,
				e.Data,
				metadata,
				e.StorageKey,
				p,
				process,
			})
		}

		copyCount, copyErr := tx.CopyFrom(
			ctx,
			pgx.Identifier{"events"},
			[]string{
				"session_id",
				"source_job_id",
				"event_type",
				"event_data",
				"metadata",
				"storage_key",
				"priority",
				"should_process",
			},
			pgx.CopyFromRows(rows),
		)
		if copyErr != nil {
			return 0, fmt.Errorf("failed to bulk copy events: %w", copyErr)
		}

		if req.SourceJobID != nil {
			if updateErr := applyJobMetaDelta(ctx, tx, jobMetaDelta{
				JobID: *req.SourceJobID,
				Delta: int(copyCount),
			}); updateErr != nil {
				return 0, updateErr
			}
		}

		return int(copyCount), nil
	})
	if err != nil {
		return 0, fmt.Errorf("bulk copy transaction failed: %w", err)
	}
	return created, nil
}

// BulkInsertWithProcessingFlags inserts multiple events with individual processing flags per event.
// This allows fine-grained control over which events should be processed by the rules engine.
func (r *EventRepo) BulkInsertWithProcessingFlags(
	ctx context.Context,
	req model.BulkEventRequest,
	shouldProcessMap map[int]bool,
) (int, error) {
	created, err := r.runEventTx(ctx, func(tx pgx.Tx) (int, error) {
		return r.insertEventsWithBatch(ctx, tx, insertEventsOptions{
			Req:           req,
			Process:       false,
			ShouldProcess: shouldProcessMap,
		})
	})
	if err != nil {
		return 0, fmt.Errorf("bulk insert with processing flags transaction failed: %w", err)
	}
	return created, nil
}

// eventColumns defines the column list for Event SELECT queries to ensure consistent field mapping.
const (
	eventColumns     = `id, session_id, source_job_id, event_type, event_data, metadata, storage_key, priority, should_process, processed, created_at`
	eventSortDirDesc = "desc"
)

func normalizeEventMetadata(meta json.RawMessage) json.RawMessage {
	if len(meta) == 0 {
		return json.RawMessage(`{}`)
	}
	return meta
}

// buildJobEventFilters constructs WHERE clause and args for job event filtering.
func buildJobEventFilters(opts model.EventListByJobOptions) (string, []any, int) {
	query := ` WHERE source_job_id = $1`
	args := []any{opts.JobID}
	argIndex := 2

	if opts.EventType != nil && *opts.EventType != "" {
		query += fmt.Sprintf(` AND event_type = $%d`, argIndex)
		args = append(args, *opts.EventType)
		argIndex++
	}

	if opts.Category != nil && *opts.Category != "" {
		categoryQuery, categoryArgs, newArgIndex := buildCategoryFilter(*opts.Category, argIndex)
		query += categoryQuery
		args = append(args, categoryArgs...)
		argIndex = newArgIndex
	}

	if opts.SearchQuery != nil && *opts.SearchQuery != "" {
		query += fmt.Sprintf(` AND event_data::text ILIKE $%d`, argIndex)
		args = append(args, "%"+*opts.SearchQuery+"%")
		argIndex++
	}

	return query, args, argIndex
}

// resolveEventSort picks the active sort from options and (optionally) a cursor.
func resolveEventSort(opts model.EventListByJobOptions, cur *eventCursorPayload) (string, string, error) {
	sortBy := defaultEventSortField
	sortDir := sortDirAsc
	hasSortBy := false
	hasSortDir := false

	if opts.SortBy != nil {
		if v := normalizeSortBy(*opts.SortBy); v != "" {
			sortBy = v
			hasSortBy = true
		}
	}

	if opts.SortDir != nil {
		if v := normalizeSortDir(*opts.SortDir); v != "" {
			sortDir = v
			hasSortDir = true
		}
	}

	if cur != nil {
		if hasSortBy && sortBy != cur.SortBy {
			return "", "", fmt.Errorf("cursor sort mismatch: %s vs %s", sortBy, cur.SortBy)
		}
		if hasSortDir && sortDir != cur.SortDir {
			return "", "", fmt.Errorf("cursor sort direction mismatch: %s vs %s", sortDir, cur.SortDir)
		}
		sortBy = cur.SortBy
		sortDir = cur.SortDir
	}

	return sortBy, sortDir, nil
}

func eventSortColumns(sortBy string) []string {
	if normalizeSortBy(sortBy) == sortByEventType {
		return []string{sortByEventType, defaultEventSortField, "id"}
	}
	return []string{defaultEventSortField, "id"}
}

func buildEventOrderClause(sortBy, sortDir string) string {
	cols := eventSortColumns(sortBy)
	parts := make([]string, 0, len(cols))
	for _, c := range cols {
		parts = append(parts, fmt.Sprintf("%s %s", c, sortDir))
	}
	return ` ORDER BY ` + strings.Join(parts, ", ")
}

// ListByJob returns events associated with a specific job with optional filters.
// Filters (EventType, Category, SearchQuery, SortBy, SortDir) are applied when non-nil/non-empty.
func (r *EventRepo) ListByJob(
	ctx context.Context,
	opts model.EventListByJobOptions,
) (*model.EventListPage, error) {
	limit := clampLimit(opts.Limit)
	offset := max(opts.Offset, 0)

	whereClause, args, argIndex := buildJobEventFilters(opts)

	cursorPayload, seekBefore, err := parseCursorFromOpts(opts)
	if err != nil {
		return nil, err
	}

	sortBy, sortDir, err := resolveEventSort(opts, cursorPayload)
	if err != nil {
		return nil, err
	}

	orderClause := buildEventOrderClause(sortBy, sortDir)

	if cursorPayload == nil {
		events, listErr := r.listByJobOffset(ctx, whereClause, orderClause, args, argIndex, limit, offset)
		if listErr != nil {
			return nil, listErr
		}
		return &model.EventListPage{Events: events}, nil
	}

	return r.listByJobKeyset(ctx, keysetParams{
		whereClause: whereClause,
		args:        args,
		argIndex:    argIndex,
		limit:       limit,
		sortBy:      sortBy,
		sortDir:     sortDir,
		seekBefore:  seekBefore,
		cursor:      cursorPayload,
	})
}

type keysetParams struct {
	whereClause string
	args        []any
	argIndex    int
	limit       int
	sortBy      string
	sortDir     string
	seekBefore  bool
	cursor      *eventCursorPayload
}

func (r *EventRepo) listByJobOffset(
	ctx context.Context,
	whereClause string,
	orderClause string,
	args []any,
	argIndex int,
	limit int,
	offset int,
) ([]*model.Event, error) {
	query := `SELECT ` + eventColumns + ` FROM events` + whereClause + orderClause
	query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argIndex, argIndex+1)
	args = append(args, limit, offset)

	var vals []*model.Event
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, qErr := pgxConn.Query(ctx, query, args...)
		if qErr != nil {
			return fmt.Errorf("query events by job: %w", qErr)
		}
		defer rows.Close()

		result, collectErr := pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
		if collectErr != nil {
			return fmt.Errorf("collect events: %w", collectErr)
		}
		valPtrs := make([]*model.Event, len(result))
		for i := range result {
			valPtrs[i] = &result[i]
		}
		vals = valPtrs
		return nil
	})
	if err != nil {
		return nil, err
	}

	return vals, nil
}

func (r *EventRepo) listByJobKeyset(
	ctx context.Context,
	p keysetParams,
) (*model.EventListPage, error) {
	query, args := buildKeysetQuery(p)

	collected, hasMore, err := r.collectKeysetEvents(ctx, query, args, p.limit, p.seekBefore)
	if err != nil {
		return nil, err
	}

	events := make([]*model.Event, len(collected))
	for i := range collected {
		events[i] = &collected[i]
	}

	nextCursor, prevCursor, cursorErr := buildPageCursors(events, p.sortBy, p.sortDir, hasMore, p.seekBefore)
	if cursorErr != nil {
		return nil, cursorErr
	}

	return &model.EventListPage{
		Events:     events,
		NextCursor: nextCursor,
		PrevCursor: prevCursor,
	}, nil
}

func buildKeysetQuery(p keysetParams) (string, []any) {
	columns := eventSortColumns(p.sortBy)
	comparator := ">"
	if p.sortDir == sortDirDesc {
		comparator = "<"
	}

	orderDir := p.sortDir
	if p.seekBefore {
		comparator = invertComparator(comparator)
		orderDir = invertSortDir(p.sortDir)
	}

	whereClause := p.whereClause + fmt.Sprintf(
		" AND (%s) %s (%s)",
		strings.Join(columns, ", "),
		comparator,
		placeholderList(p.argIndex, len(columns)),
	)
	args := append(append([]any{}, p.args...), cursorArgs(p.sortBy, p.cursor)...)
	argIndex := p.argIndex + len(columns)

	query := `SELECT ` + eventColumns + ` FROM events` + whereClause + buildEventOrderClause(p.sortBy, orderDir)
	args = append(args, p.limit+1) // fetch one extra to know if another page exists
	query += fmt.Sprintf(` LIMIT $%d`, argIndex)

	return query, args
}

func (r *EventRepo) collectKeysetEvents(
	ctx context.Context,
	query string,
	args []any,
	limit int,
	seekBefore bool,
) ([]model.Event, bool, error) {
	var collected []model.Event
	err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, qErr := pgxConn.Query(ctx, query, args...)
		if qErr != nil {
			return fmt.Errorf("query events by job (keyset): %w", qErr)
		}
		defer rows.Close()

		var collectErr error
		collected, collectErr = pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
		if collectErr != nil {
			return fmt.Errorf("collect events (keyset): %w", collectErr)
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}

	hasMore := len(collected) > limit
	if hasMore {
		collected = collected[:limit]
	}

	if seekBefore {
		reverseEvents(collected)
	}

	return collected, hasMore, nil
}

func clampLimit(limit int) int {
	switch {
	case limit <= 0:
		return 50
	case limit > 1000:
		return 1000
	default:
		return limit
	}
}

func parseCursorFromOpts(opts model.EventListByJobOptions) (*eventCursorPayload, bool, error) {
	if opts.CursorAfter != nil && opts.CursorBefore != nil {
		return nil, false, errors.New("only one of cursor_after or cursor_before can be set")
	}

	var cursorToken string
	var seekBefore bool

	if opts.CursorAfter != nil {
		cursorToken = *opts.CursorAfter
	}
	if opts.CursorBefore != nil {
		cursorToken = *opts.CursorBefore
		seekBefore = true
	}

	if cursorToken == "" {
		return nil, seekBefore, nil
	}

	cur, err := decodeEventCursorPayload(cursorToken)
	if err != nil {
		return nil, false, err
	}

	return &cur, seekBefore, nil
}

func cursorArgs(sortBy string, cur *eventCursorPayload) []any {
	args := make([]any, 0, len(eventSortColumns(sortBy)))
	if sortBy == sortByEventType {
		if cur.EventType != nil {
			args = append(args, *cur.EventType)
		} else {
			args = append(args, nil)
		}
	}
	args = append(args, cur.CreatedAt, cur.ID)
	return args
}

func placeholderList(start, count int) string {
	placeholders := make([]string, count)
	for i := range count {
		placeholders[i] = fmt.Sprintf("$%d", start+i)
	}
	return strings.Join(placeholders, ", ")
}

func invertComparator(op string) string {
	if op == "<" {
		return ">"
	}
	return "<"
}

func invertSortDir(dir string) string {
	if dir == sortDirDesc {
		return sortDirAsc
	}
	return sortDirDesc
}

func reverseEvents(events []model.Event) {
	for i, j := 0, len(events)-1; i < j; i, j = i+1, j-1 {
		events[i], events[j] = events[j], events[i]
	}
}

func buildPageCursors(
	events []*model.Event,
	sortBy string,
	sortDir string,
	hasMore bool,
	seekBefore bool,
) (*string, *string, error) {
	if len(events) == 0 {
		return nil, nil, nil
	}

	encode := func(ev *model.Event, context string) (*string, error) {
		token, err := encodeEventCursorPayload(newEventCursorFromEvent(ev, sortBy, sortDir))
		if err != nil {
			return nil, fmt.Errorf("encode %s cursor: %w", context, err)
		}
		return &token, nil
	}

	var nextCursor *string
	var prevCursor *string

	last := events[len(events)-1]
	first := events[0]
	nextNeeded := seekBefore || hasMore
	prevNeeded := !seekBefore || hasMore

	if nextNeeded {
		c, err := encode(last, "next")
		if err != nil {
			return nil, nil, err
		}
		nextCursor = c
	}

	if prevNeeded {
		c, err := encode(first, "prev")
		if err != nil {
			return nil, nil, err
		}
		prevCursor = c
	}

	return nextCursor, prevCursor, nil
}

func (r *EventRepo) precomputedEventCount(ctx context.Context, jobID string) (int, bool, error) {
	if strings.TrimSpace(jobID) == "" {
		return 0, false, nil
	}

	var count int
	err := r.DB.QueryRowContext(ctx, `
		SELECT event_count
		FROM job_meta
		WHERE job_id = $1
	`, jobID).Scan(&count)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return 0, false, nil
	case err != nil:
		return 0, false, fmt.Errorf("get precomputed event count: %w", err)
	default:
		return count, true, nil
	}
}

// CountByJob returns the total count of events for a specific job with optional filters.
// Filters (EventType, Category, SearchQuery) are applied when non-nil/non-empty.
// This is useful for pagination to show accurate total event count.
func (r *EventRepo) CountByJob(
	ctx context.Context,
	opts model.EventListByJobOptions,
) (int, error) {
	if opts.EventType == nil && opts.Category == nil && opts.SearchQuery == nil {
		if count, ok, err := r.precomputedEventCount(ctx, opts.JobID); err != nil {
			return 0, err
		} else if ok {
			return count, nil
		}
	}

	// Build WHERE clause with optional filters (same logic as ListByJob)
	whereClause, args, _ := buildJobEventFilters(opts)
	query := `SELECT COUNT(*) FROM events` + whereClause

	var count int
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		return pgxConn.QueryRow(ctx, query, args...).Scan(&count)
	}); err != nil {
		return 0, fmt.Errorf("count events by job: %w", err)
	}

	return count, nil
}

type categoryFilterConfig struct {
	ilike  []string
	equals []string
}

var categoryFilterConfigs = map[string]categoryFilterConfig{ //nolint:gochecknoglobals // intentional package-level cache for static configs
	"screenshot": {
		ilike: []string{"%screenshot%"},
	},
	"worker_log": {
		equals: []string{"worker.log"},
		ilike:  []string{"%.log"},
	},
	"job_failure": {
		ilike: []string{"%jobfailure%", "%job.failure%"},
	},
	"network": {
		ilike: []string{"%request%", "%response%", "%network%"},
	},
	"console": {
		ilike:  []string{"%console%"},
		equals: []string{"log"},
	},
	"security": {
		ilike: []string{"Security.%", "%dynamiccodeeval%"},
	},
	"page": {
		ilike: []string{"%goto%", "%navigate%", "%page.goto%"},
	},
	"action": {
		ilike: []string{"%click%", "%type%", "%waitforselector%", "%setcontent%", "%select%", "%hover%"},
	},
	"error": {
		ilike: []string{"%error%", "%exception%"},
	},
}

// buildCategoryFilter constructs the category filter clause and arguments.
func buildCategoryFilter(category string, argIndex int) (string, []any, int) {
	cfg, ok := categoryFilterConfigs[category]
	if !ok {
		return "", nil, argIndex
	}

	clauses := make([]string, 0, len(cfg.ilike)+len(cfg.equals))
	args := make([]any, 0, len(cfg.ilike)+len(cfg.equals))
	nextIndex := argIndex

	for _, pattern := range cfg.ilike {
		clauses = append(clauses, fmt.Sprintf("event_type ILIKE $%d", nextIndex))
		args = append(args, pattern)
		nextIndex++
	}

	for _, value := range cfg.equals {
		clauses = append(clauses, fmt.Sprintf("event_type = $%d", nextIndex))
		args = append(args, value)
		nextIndex++
	}

	if len(clauses) == 0 {
		return "", nil, argIndex
	}

	return fmt.Sprintf(" AND (%s)", strings.Join(clauses, " OR ")), args, nextIndex
}

// buildEventFiltersQuery constructs the WHERE clause and arguments for event filtering.
func buildEventFiltersQuery(opts model.EventListByJobOptions) (string, []any, int) {
	query := `
		SELECT ` + eventColumns + `
		FROM events
		WHERE source_job_id = $1`

	args := []any{opts.JobID}
	argIndex := 2

	// Add event type filter
	if opts.EventType != nil && *opts.EventType != "" {
		query += fmt.Sprintf(` AND event_type = $%d`, argIndex)
		args = append(args, *opts.EventType)
		argIndex++
	}

	// Add category filter (requires event type pattern matching)
	if opts.Category != nil && *opts.Category != "" {
		categoryQuery, categoryArgs, newArgIndex := buildCategoryFilter(*opts.Category, argIndex)
		query += categoryQuery
		args = append(args, categoryArgs...)
		argIndex = newArgIndex
	}

	// Add text search in event_data JSON
	if opts.SearchQuery != nil && *opts.SearchQuery != "" {
		query += fmt.Sprintf(` AND event_data::text ILIKE $%d`, argIndex)
		args = append(args, "%"+*opts.SearchQuery+"%")
		argIndex++
	}

	return query, args, argIndex
}

// buildEventSortClause constructs the ORDER BY clause for event queries.
func buildEventSortClause(opts model.EventListByJobOptions, argIndex int) (string, string) {
	sortBy := defaultEventSortField
	sortDir := "ASC"
	if opts.SortBy != nil {
		if v := normalizeSortBy(*opts.SortBy); v != "" {
			sortBy = v
		}
	}
	if opts.SortDir != nil {
		if v := normalizeSortDir(*opts.SortDir); v != "" {
			sortDir = v
		}
	}

	orderClause := fmt.Sprintf(`
		%s
		LIMIT $%d OFFSET $%d`, buildEventOrderClause(sortBy, sortDir), argIndex, argIndex+1)

	return orderClause, sortDir
}

// ListWithFilters returns events with optional filtering by event type, category, and text search.
// This extends ListByJob with additional filtering capabilities while maintaining the same ordering.
func (r *EventRepo) ListWithFilters(
	ctx context.Context,
	opts model.EventListByJobOptions,
) ([]*model.Event, error) {
	limit := opts.Limit
	if limit <= 0 {
		limit = 50 // Default limit
	}
	if limit > 1000 {
		limit = 1000 // Max limit
	}
	// clamp offset to 0
	offset := max(opts.Offset, 0)

	// Build query with filters
	query, args, argIndex := buildEventFiltersQuery(opts)

	// Add sorting and pagination
	orderClause, _ := buildEventSortClause(opts, argIndex)
	query += orderClause
	args = append(args, limit, offset)

	var result []*model.Event
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, query, args...)
		if err != nil {
			return fmt.Errorf("query events with filters: %w", err)
		}
		defer rows.Close()

		vals, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
		if err != nil {
			return fmt.Errorf("collect events: %w", err)
		}
		result = make([]*model.Event, len(vals))
		for i := range vals {
			result[i] = &vals[i]
		}
		return nil
	}); err != nil {
		return nil, err
	}

	return result, nil
}

// MarkProcessedByIDs sets processed=true for the given event IDs and returns number of rows updated.
func (r *EventRepo) MarkProcessedByIDs(ctx context.Context, eventIDs []string) (int, error) {
	if len(eventIDs) == 0 {
		return 0, nil
	}
	// Convert to []uuid.UUID for stricter binding and avoid relying on text[]::uuid[] cast.
	uids := make([]uuid.UUID, 0, len(eventIDs))
	for _, s := range eventIDs {
		id, err := uuid.Parse(s)
		if err != nil {
			return 0, fmt.Errorf("invalid uuid in eventIDs: %w", err)
		}
		uids = append(uids, id)
	}
	query := `
		UPDATE events
		SET processed = TRUE
		WHERE id = ANY($1)
		  AND processed = FALSE
	`
	var updated int
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		ct, err := pgxConn.Exec(ctx, query, uids)
		if err != nil {
			return fmt.Errorf("mark events processed: %w", err)
		}
		updated = int(ct.RowsAffected())
		return nil
	}); err != nil {
		return 0, err
	}
	return updated, nil
}

// GetByIDs retrieves events by their IDs.
func (r *EventRepo) GetByIDs(ctx context.Context, eventIDs []string) ([]*model.Event, error) {
	if len(eventIDs) == 0 {
		return []*model.Event{}, nil
	}

	// Convert to []uuid for stricter binding and early validation.
	uids := make([]uuid.UUID, 0, len(eventIDs))
	for _, s := range eventIDs {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, fmt.Errorf("invalid uuid in eventIDs: %w", err)
		}
		uids = append(uids, id)
	}

	query := `
		SELECT ` + eventColumns + `
		FROM events
		WHERE id = ANY($1)
		ORDER BY created_at ASC, id ASC
	`

	var result []*model.Event
	if err := pgxutil.WithPgxConn(ctx, r.DB, func(pgxConn *pgx.Conn) error {
		rows, err := pgxConn.Query(ctx, query, uids)
		if err != nil {
			return fmt.Errorf("query events by IDs: %w", err)
		}
		defer rows.Close()
		vals, err := pgx.CollectRows(rows, pgx.RowToStructByName[model.Event])
		if err != nil {
			return fmt.Errorf("collect events: %w", err)
		}
		result = make([]*model.Event, len(vals))
		for i := range vals {
			result[i] = &vals[i]
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return result, nil
}

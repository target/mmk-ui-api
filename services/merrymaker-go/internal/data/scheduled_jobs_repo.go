package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain"
)

// ScheduledJobsRepo provides database operations for scheduled jobs management.
type ScheduledJobsRepo struct {
	DB           *sql.DB
	timeProvider TimeProvider
}

// NewScheduledJobsRepo creates a new ScheduledJobsRepo instance with the given database connection.
func NewScheduledJobsRepo(db *sql.DB) *ScheduledJobsRepo {
	return &ScheduledJobsRepo{
		DB:           db,
		timeProvider: &RealTimeProvider{},
	}
}

// NewScheduledJobsRepoWithTimeProvider creates a ScheduledJobsRepo with a custom TimeProvider (useful for testing).
func NewScheduledJobsRepoWithTimeProvider(db *sql.DB, timeProvider TimeProvider) *ScheduledJobsRepo {
	return &ScheduledJobsRepo{
		DB:           db,
		timeProvider: timeProvider,
	}
}

// fnvHash computes FNV-1a 64-bit hash of the given string for use as advisory lock key.
func fnvHash(s string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(s))
	// Advisory locks accept BIGINT; constrain the unsigned hash into int64 range before casting.
	u := h.Sum64()
	if u > uint64(math.MaxInt64) {
		u %= uint64(math.MaxInt64)
	}
	return int64(u) // #nosec G115 -- value is explicitly bounded to <= MaxInt64 before casting to int64.
}

const scheduledJobColumns = `
  id,
  task_name,
  payload,
  EXTRACT(EPOCH FROM scheduled_interval)::bigint AS interval_seconds,
  last_queued_at,
  updated_at,
  overrun_policy,
  overrun_state_mask,
  active_fire_key
`

// FindDue finds scheduled tasks that are due for execution.
// Uses FOR UPDATE SKIP LOCKED to prevent concurrent schedulers from processing the same tasks.
// A task is due when last_queued_at IS NULL OR last_queued_at + interval <= now.
func (r *ScheduledJobsRepo) FindDue(ctx context.Context, now time.Time, limit int) ([]domain.ScheduledTask, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", limit)
	}

	query := `
		SELECT ` + scheduledJobColumns + `
		FROM scheduled_jobs
		WHERE (last_queued_at IS NULL OR last_queued_at + scheduled_interval <= $1)
		ORDER BY
			CASE WHEN last_queued_at IS NULL THEN 0 ELSE 1 END,
			last_queued_at ASC,
			created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`

	// Use pgx via stdlib bridge to leverage pgx v5 helpers
	conn, err := r.DB.Conn(ctx)
	if err != nil {
		// Important: Closing the acquired *sql.Conn here returns it to the pool.
		// It does NOT close the shared *sql.DB or underlying pool; this prevents leaks.

		return nil, fmt.Errorf("get connection: %w", err)
	}
	defer func() {
		if cerr := conn.Close(); cerr != nil {
			// connection close failure is best-effort and ignored
			_ = cerr
		}
	}()

	var tasks []domain.ScheduledTask
	err = conn.Raw(func(dc any) error {
		stdConn, ok := dc.(*stdlib.Conn)
		if !ok {
			return fmt.Errorf("unexpected driver connection type: %T", dc)
		}
		pgxConn := stdConn.Conn()
		rows, queryErr := pgxConn.Query(ctx, query, now.UTC(), limit)
		if queryErr != nil {
			return queryErr
		}
		defer rows.Close()

		collected, collectErr := pgx.CollectRows(rows, rowToScheduledTask)
		if collectErr != nil {
			return collectErr
		}
		tasks = collected
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("query due scheduled tasks: %w", err)
	}

	return tasks, nil
}

// FindDueTx is the transactional variant of FindDue. It must be paired with any updates
// (e.g., MarkQueued) within the same transaction to ensure SKIP LOCKED semantics hold
// across selection and subsequent updates.
func (r *ScheduledJobsRepo) FindDueTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.FindDueParams,
) ([]domain.ScheduledTask, error) {
	if p.Limit <= 0 {
		return nil, fmt.Errorf("limit must be positive, got %d", p.Limit)
	}

	query := `
		SELECT ` + scheduledJobColumns + `
		FROM scheduled_jobs
		WHERE (last_queued_at IS NULL OR last_queued_at + scheduled_interval <= $1)
		ORDER BY
			CASE WHEN last_queued_at IS NULL THEN 0 ELSE 1 END,
			last_queued_at ASC,
			created_at ASC
		LIMIT $2
		FOR UPDATE SKIP LOCKED
	`

	rows, queryErr := tx.QueryContext(ctx, query, p.Now.UTC(), p.Limit)
	if queryErr != nil {
		return nil, fmt.Errorf("query due scheduled tasks: %w", queryErr)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			// best-effort close; nothing further to do
			_ = closeErr
		}
	}()

	var tasks []domain.ScheduledTask
	for rows.Next() {
		task, scanErr := scanScheduledTaskFromSQLRows(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan scheduled task: %w", scanErr)
		}
		tasks = append(tasks, task)
	}

	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate scheduled tasks: %w", rowsErr)
	}

	return tasks, nil
}

// MarkQueued updates the last_queued_at timestamp for a scheduled task.
// Return semantics:
//   - (true, nil): task found and updated
//   - (false, nil): task not found
//   - (false, err): update failed due to error
func (r *ScheduledJobsRepo) MarkQueued(ctx context.Context, id string, now time.Time) (bool, error) {
	currentTime := r.timeProvider.Now()

	clauses := []string{"last_queued_at = $2", "updated_at = $3"}
	args := []any{id, now.UTC(), currentTime.UTC()}

	clauses, args = appendActiveFireKeyUpdate(
		clauses,
		args,
		activeFireKeyUpdateParams{fallback: currentTime.UTC()},
	)

	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE scheduled_jobs SET ")
	queryBuilder.WriteString(strings.Join(clauses, ", "))
	queryBuilder.WriteString(" WHERE id = $1")

	res, err := r.DB.ExecContext(ctx, queryBuilder.String(), args...)
	if err != nil {
		return false, fmt.Errorf("update scheduled task: %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get rows affected: %w", err)
	}

	return rowsAffected > 0, nil
}

// MarkQueuedTx updates last_queued_at within an existing transaction.
// Use this with FindDueTx to ensure selection and update happen under the same locks.
func (r *ScheduledJobsRepo) MarkQueuedTx(ctx context.Context, tx *sql.Tx, p domain.MarkQueuedParams) (bool, error) {
	currentTime := r.timeProvider.Now()

	clauses := []string{"last_queued_at = $2", "updated_at = $3"}
	args := []any{p.ID, p.Now.UTC(), currentTime.UTC()}

	clauses, args = appendActiveFireKeyUpdate(
		clauses,
		args,
		activeFireKeyUpdateParams{keyPtr: p.ActiveFireKey, setAt: p.ActiveFireKeySetAt, fallback: currentTime.UTC()},
	)

	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE scheduled_jobs SET ")
	queryBuilder.WriteString(strings.Join(clauses, ", "))
	queryBuilder.WriteString(" WHERE id = $1")

	res, err := tx.ExecContext(ctx, queryBuilder.String(), args...)
	if err != nil {
		return false, fmt.Errorf("update scheduled task (tx): %w", err)
	}

	rowsAffected, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("get rows affected (tx): %w", err)
	}

	return rowsAffected > 0, nil
}

// UpdateActiveFireKeyTx updates or clears the active fire key for a scheduled task within a transaction.
func (r *ScheduledJobsRepo) UpdateActiveFireKeyTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.UpdateActiveFireKeyParams,
) error {
	currentTime := r.timeProvider.Now().UTC()
	updateAt := currentTime
	if !p.SetAt.IsZero() {
		updateAt = p.SetAt.UTC()
	}

	clauses := []string{"updated_at = $2"}
	args := []any{p.ID, currentTime}

	clauses, args = appendActiveFireKeyUpdate(
		clauses,
		args,
		activeFireKeyUpdateParams{keyPtr: p.FireKey, setAt: &p.SetAt, fallback: updateAt},
	)

	var queryBuilder strings.Builder
	queryBuilder.WriteString("UPDATE scheduled_jobs SET ")
	queryBuilder.WriteString(strings.Join(clauses, ", "))
	queryBuilder.WriteString(" WHERE id = $1")

	if _, err := tx.ExecContext(ctx, queryBuilder.String(), args...); err != nil {
		return fmt.Errorf("update active fire key: %w", err)
	}
	return nil
}

// TryWithTaskLock attempts to acquire an advisory lock for the given task name.
// Uses FNV-1a 64-bit hash of task_name for the lock key.
// If the lock is acquired, executes fn within the same transaction.
// Return semantics:
//   - (false, nil): lock not acquired; fn was not executed
//   - (true, nil): lock acquired; fn executed and succeeded
//   - (true, err): lock acquired; fn executed and failed with err
func (r *ScheduledJobsRepo) TryWithTaskLock(
	ctx context.Context,
	taskName string,
	fn func(context.Context, *sql.Tx) error,
) (bool, error) {
	lockKey := fnvHash(taskName)

	var locked bool
	var fnErr error

	err := pgxutil.WithSQLTx(ctx, r.DB, pgxutil.SQLTxConfig{
		Fn: func(tx *sql.Tx) error {
			// Try to acquire advisory lock within transaction
			if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1)", lockKey).Scan(&locked); err != nil {
				return fmt.Errorf("acquire advisory lock for task %s: %w", taskName, err)
			}

			if !locked {
				return nil // Lock not acquired, but no error
			}

			// Lock acquired, execute function with the same transaction
			fnErr = fn(ctx, tx)
			// Don't return fnErr here - we want to commit the transaction regardless
			// The function error will be returned separately
			return nil
		},
	})
	if err != nil {
		return false, err
	}

	return locked, fnErr
}

// scheduledTaskRow represents the database row structure for scheduled tasks.
// This struct matches the database schema exactly, allowing pgx.RowToStructByName to work.
type scheduledTaskRow struct {
	ID               string         `db:"id"`
	TaskName         string         `db:"task_name"`
	Payload          []byte         `db:"payload"`
	IntervalSeconds  sql.NullInt64  `db:"interval_seconds"`
	LastQueuedAt     sql.NullTime   `db:"last_queued_at"`
	UpdatedAt        time.Time      `db:"updated_at"`
	OverrunPolicy    sql.NullString `db:"overrun_policy"`
	OverrunStateMask sql.NullInt64  `db:"overrun_state_mask"`
	ActiveFireKey    sql.NullString `db:"active_fire_key"`
}

// toDomainScheduledTask converts a scheduledTaskRow to domain.ScheduledTask.
func (r *scheduledTaskRow) toDomainScheduledTask() domain.ScheduledTask {
	if r == nil {
		return domain.ScheduledTask{}
	}

	task := domain.ScheduledTask{
		ID:        r.ID,
		TaskName:  r.TaskName,
		UpdatedAt: r.UpdatedAt,
	}

	if r.IntervalSeconds.Valid {
		task.Interval = time.Duration(r.IntervalSeconds.Int64) * time.Second
	}
	if r.Payload != nil {
		task.Payload = json.RawMessage(r.Payload)
	}
	if r.LastQueuedAt.Valid {
		task.LastQueuedAt = &r.LastQueuedAt.Time
	}
	if r.OverrunPolicy.Valid {
		p := domain.OverrunPolicy(r.OverrunPolicy.String)
		task.OverrunPolicy = &p
	}
	if r.OverrunStateMask.Valid {
		if val := r.OverrunStateMask.Int64; val >= 0 && val <= math.MaxUint8 {
			mask := domain.OverrunStateMask(val)
			task.OverrunStates = &mask
		}
	}
	if r.ActiveFireKey.Valid {
		key := strings.TrimSpace(r.ActiveFireKey.String)
		if key != "" {
			task.ActiveFireKey = &key
		}
	}

	return task
}

// rowToScheduledTask maps a pgx row to domain.ScheduledTask using pgx v5 generics.
func rowToScheduledTask(row pgx.CollectableRow) (domain.ScheduledTask, error) {
	dbRow, err := pgx.RowToStructByName[scheduledTaskRow](row)
	if err != nil {
		return domain.ScheduledTask{}, fmt.Errorf("scan scheduled task row: %w", err)
	}
	return dbRow.toDomainScheduledTask(), nil
}

type activeFireKeyUpdateParams struct {
	keyPtr   *string
	setAt    *time.Time
	fallback time.Time
}

func appendActiveFireKeyUpdate(
	clauses []string,
	args []any,
	params activeFireKeyUpdateParams,
) ([]string, []any) {
	if params.keyPtr == nil {
		clauses = append(clauses, "active_fire_key = NULL", "active_fire_key_set_at = NULL")
		return clauses, args
	}

	key := strings.TrimSpace(*params.keyPtr)
	if key == "" {
		clauses = append(clauses, "active_fire_key = NULL", "active_fire_key_set_at = NULL")
		return clauses, args
	}

	idx := len(args) + 1
	clauses = append(clauses, fmt.Sprintf("active_fire_key = $%d", idx))
	args = append(args, key)
	idx++

	ts := params.fallback
	if params.setAt != nil && !params.setAt.IsZero() {
		ts = params.setAt.UTC()
	}
	clauses = append(clauses, fmt.Sprintf("active_fire_key_set_at = $%d", idx))
	args = append(args, ts)

	return clauses, args
}

// scanScheduledTaskFromSQLRows scans a database/sql row into a ScheduledTask struct.
// This is used for methods that work with database/sql instead of pgx.
func scanScheduledTaskFromSQLRows(rows *sql.Rows) (domain.ScheduledTask, error) {
	var dbRow scheduledTaskRow
	err := rows.Scan(
		&dbRow.ID,
		&dbRow.TaskName,
		&dbRow.Payload,
		&dbRow.IntervalSeconds,
		&dbRow.LastQueuedAt,
		&dbRow.UpdatedAt,
		&dbRow.OverrunPolicy,
		&dbRow.OverrunStateMask,
		&dbRow.ActiveFireKey,
	)
	if err != nil {
		return domain.ScheduledTask{}, fmt.Errorf("scan scheduled task row: %w", err)
	}
	return dbRow.toDomainScheduledTask(), nil
}

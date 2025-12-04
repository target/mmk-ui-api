package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
)

// Advisory lock namespace for reaper operations.
// Using two-arg pg_try_advisory_xact_lock(major, minor) for proper namespacing.
// Major key 1000 is reserved for merrymaker reaper operations.
const (
	advisoryLockReaperMajor         = 1000
	advisoryLockReaperFailPending   = 1 // minor key for FailStalePendingJobs
	advisoryLockReaperDelete        = 2 // minor key for DeleteOldJobs
	advisoryLockReaperDeleteResults = 3 // minor key for DeleteOldJobResults
)

// FailStalePendingJobs marks pending jobs older than maxAge as failed.
// Processes up to batchSize jobs per call to prevent long locks and I/O spikes.
// Uses advisory locks to prevent concurrent reaper instances from conflicting.
// Returns the number of jobs marked as failed.
func (r *JobRepo) FailStalePendingJobs(ctx context.Context, maxAge time.Duration, batchSize int) (int64, error) {
	var rowsAffected int64
	err := pgxutil.WithSQLTx(ctx, r.DB, pgxutil.SQLTxConfig{
		Fn: func(tx *sql.Tx) error {
			var locked bool
			if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1, $2)", advisoryLockReaperMajor, advisoryLockReaperFailPending).Scan(&locked); err != nil {
				return fmt.Errorf("acquire advisory lock: %w", err)
			}
			if !locked {
				rowsAffected = 0
				return nil
			}

			currentTime := r.timeProvider.Now()
			cutoffTime := currentTime.Add(-maxAge)

			res, err := tx.ExecContext(ctx, `
				UPDATE jobs
				SET status = 'failed',
					last_error = 'Job timed out in pending status',
					completed_at = $1,
					updated_at = $1
				WHERE id IN (
					SELECT id FROM jobs
					WHERE status = 'pending'
					  AND created_at < $2
					ORDER BY created_at
					LIMIT $3
				)
			`, currentTime.UTC(), cutoffTime.UTC(), batchSize)
			if err != nil {
				return fmt.Errorf("fail stale pending jobs: %w", err)
			}

			ra, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("rows affected: %w", err)
			}
			rowsAffected = ra
			return nil
		},
	})
	if err != nil {
		return 0, err
	}
	return rowsAffected, nil
}

// DeleteOldJobs deletes jobs with the given status older than maxAge.
// Processes up to batchSize jobs per call to prevent long locks and I/O spikes.
// Uses advisory locks to prevent concurrent reaper instances from conflicting.
// Returns the number of jobs deleted.
func (r *JobRepo) DeleteOldJobs(ctx context.Context, params core.DeleteOldJobsParams) (int64, error) {
	if !params.Status.Valid() {
		return 0, fmt.Errorf("invalid job status: %s", params.Status)
	}

	var rowsAffected int64
	err := pgxutil.WithSQLTx(ctx, r.DB, pgxutil.SQLTxConfig{
		Fn: func(tx *sql.Tx) error {
			var locked bool
			if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1, $2)", advisoryLockReaperMajor, advisoryLockReaperDelete).Scan(&locked); err != nil {
				return fmt.Errorf("acquire advisory lock: %w", err)
			}
			if !locked {
				rowsAffected = 0
				return nil
			}

			currentTime := r.timeProvider.Now()
			cutoffTime := currentTime.Add(-params.MaxAge)

			res, err := tx.ExecContext(ctx, `
				DELETE FROM jobs
				WHERE id IN (
					SELECT id FROM jobs
					WHERE status = $1
					  AND (completed_at < $2 OR (completed_at IS NULL AND updated_at < $2))
					ORDER BY COALESCE(completed_at, updated_at)
					LIMIT $3
				)
			`, params.Status, cutoffTime.UTC(), params.BatchSize)
			if err != nil {
				return fmt.Errorf("delete old jobs: %w", err)
			}

			ra, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("rows affected: %w", err)
			}
			rowsAffected = ra
			return nil
		},
	})
	if err != nil {
		return 0, err
	}
	return rowsAffected, nil
}

// DeleteOldJobResults deletes persisted job_results rows for the given job type that are older than maxAge.
// Processes up to batchSize rows per call to prevent long locks and I/O spikes.
// Uses advisory locks to prevent concurrent reaper instances from conflicting.
func (r *JobRepo) DeleteOldJobResults(ctx context.Context, params core.DeleteOldJobResultsParams) (int64, error) {
	if !params.JobType.Valid() {
		return 0, fmt.Errorf("invalid job type: %s", params.JobType)
	}
	if params.BatchSize <= 0 {
		return 0, errors.New("batch size must be greater than zero")
	}
	if params.MaxAge <= 0 {
		return 0, errors.New("max age must be greater than zero")
	}

	var rowsAffected int64
	err := pgxutil.WithSQLTx(ctx, r.DB, pgxutil.SQLTxConfig{
		Fn: func(tx *sql.Tx) error {
			var locked bool
			if err := tx.QueryRowContext(ctx, "SELECT pg_try_advisory_xact_lock($1, $2)", advisoryLockReaperMajor, advisoryLockReaperDeleteResults).Scan(&locked); err != nil {
				return fmt.Errorf("acquire advisory lock: %w", err)
			}
			if !locked {
				rowsAffected = 0
				return nil
			}

			cutoffTime := r.timeProvider.Now().Add(-params.MaxAge).UTC()

			res, err := tx.ExecContext(ctx, `
				DELETE FROM job_results
				USING (
					SELECT ctid
					FROM job_results
					WHERE job_type = $1
					  AND updated_at < $2
					ORDER BY updated_at
					LIMIT $3
				) sub
				WHERE job_results.ctid = sub.ctid
			`, params.JobType, cutoffTime, params.BatchSize)
			if err != nil {
				return fmt.Errorf("delete old job_results: %w", err)
			}

			ra, err := res.RowsAffected()
			if err != nil {
				return fmt.Errorf("rows affected: %w", err)
			}
			rowsAffected = ra
			return nil
		},
	})
	if err != nil {
		return 0, err
	}
	return rowsAffected, nil
}

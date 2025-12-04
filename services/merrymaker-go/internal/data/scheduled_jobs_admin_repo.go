package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/target/mmk-ui-api/internal/domain"
)

// ScheduledJobsAdminRepo provides admin operations for scheduled_jobs (upsert/delete by task name).
// This is separate from the concurrency-focused ScheduledJobsRepo used by the scheduler tick loop.
type ScheduledJobsAdminRepo struct {
	DB           *sql.DB
	timeProvider TimeProvider
}

// NewScheduledJobsAdminRepo creates a new ScheduledJobsAdminRepo instance with the given database connection.
func NewScheduledJobsAdminRepo(db *sql.DB) *ScheduledJobsAdminRepo {
	return &ScheduledJobsAdminRepo{DB: db, timeProvider: &RealTimeProvider{}}
}

// NewScheduledJobsAdminRepoWithTimeProvider allows injecting a custom time provider (for testing).
func NewScheduledJobsAdminRepoWithTimeProvider(db *sql.DB, tp TimeProvider) *ScheduledJobsAdminRepo {
	return &ScheduledJobsAdminRepo{DB: db, timeProvider: tp}
}

// UpsertByTaskName creates or updates a scheduled job identified by taskName.
// Updates payload and scheduled_interval; preserves last_queued_at.
func (r *ScheduledJobsAdminRepo) UpsertByTaskName(ctx context.Context, req domain.UpsertTaskParams) error {
	if req.TaskName == "" {
		return errors.New("taskName is required")
	}
	secs := int64(req.Interval / time.Second)
	if secs <= 0 {
		return errors.New("interval must be positive")
	}
	now := r.timeProvider.Now().UTC()

	var policyVal any
	if req.OverrunPolicy != nil {
		policy := string(*req.OverrunPolicy)
		policyVal = policy
	}

	var stateVal any
	if req.OverrunStates != nil {
		stateVal = int16(*req.OverrunStates)
	}

	q := `
		INSERT INTO scheduled_jobs (task_name, payload, scheduled_interval, overrun_policy, overrun_state_mask, created_at, updated_at)
		VALUES ($1, $2, ($3::int * interval '1 second'), $4, $5, $6, $6)
	ON CONFLICT (task_name) DO UPDATE
	SET payload = EXCLUDED.payload,
	    scheduled_interval = EXCLUDED.scheduled_interval,
	    overrun_policy = COALESCE(EXCLUDED.overrun_policy, scheduled_jobs.overrun_policy),
	    overrun_state_mask = COALESCE(EXCLUDED.overrun_state_mask, scheduled_jobs.overrun_state_mask),
	    updated_at = EXCLUDED.updated_at
	`
	_, err := r.DB.ExecContext(ctx, q, req.TaskName, req.Payload, secs, policyVal, stateVal, now)
	if err != nil {
		return fmt.Errorf("upsert scheduled_job: %w", err)
	}
	return nil
}

// DeleteByTaskName deletes a scheduled job identified by taskName.
func (r *ScheduledJobsAdminRepo) DeleteByTaskName(ctx context.Context, taskName string) (bool, error) {
	if taskName == "" {
		return false, errors.New("taskName is required")
	}
	q := `DELETE FROM scheduled_jobs WHERE task_name = $1`
	res, err := r.DB.ExecContext(ctx, q, taskName)
	if err != nil {
		return false, fmt.Errorf("delete scheduled_job: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("rows affected: %w", err)
	}
	return n > 0, nil
}

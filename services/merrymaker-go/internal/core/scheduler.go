// Package core provides the business logic and service layer for the merrymaker job system.
package core

import (
	"context"
	"database/sql"
	"time"

	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// ScheduledJobsRepository defines the interface for scheduled jobs data operations.
// It provides concurrency-safe operations for managing scheduled tasks.
type ScheduledJobsRepository interface {
	// FindDue finds scheduled tasks that are due for execution.
	// Uses FOR UPDATE SKIP LOCKED to prevent concurrent schedulers from processing the same tasks.
	// A task is due when last_queued_at IS NULL OR last_queued_at + interval <= now.
	FindDue(ctx context.Context, now time.Time, limit int) ([]domain.ScheduledTask, error)

	// FindDueTx is the transactional variant of FindDue; rows remain locked until tx ends.
	FindDueTx(ctx context.Context, tx *sql.Tx, p domain.FindDueParams) ([]domain.ScheduledTask, error)

	// MarkQueued updates the last_queued_at timestamp for a scheduled task.
	// Return semantics:
	//   - (true, nil): task found and updated
	//   - (false, nil): task not found
	//   - (false, err): update failed due to error
	// Note: We may later replace the bool with a sentinel error (e.g., ErrScheduledTaskNotFound) for idiomatic errors.Is checks.
	MarkQueued(ctx context.Context, id string, now time.Time) (bool, error)

	// MarkQueuedTx updates last_queued_at within an existing transaction.
	MarkQueuedTx(ctx context.Context, tx *sql.Tx, p domain.MarkQueuedParams) (bool, error)

	// UpdateActiveFireKeyTx sets or clears the active fire key for a task within the provided transaction.
	UpdateActiveFireKeyTx(ctx context.Context, tx *sql.Tx, p domain.UpdateActiveFireKeyParams) error

	// TryWithTaskLock attempts to acquire an advisory lock for the given task name.
	// Uses FNV-1a 64-bit hash of task_name for the lock key.
	// If the lock is acquired, executes fn within the same transaction.
	// Return semantics:
	//   - (false, nil): lock not acquired; fn was not executed
	//   - (true, nil): lock acquired; fn executed and succeeded
	//   - (true, err): lock acquired; fn executed and failed with err
	TryWithTaskLock(
		ctx context.Context,
		taskName string,
		fn func(context.Context, *sql.Tx) error,
	) (bool, error)
}

// ScheduledJobsAdminRepository defines minimal admin operations for creating/updating/removing scheduled tasks by name.
// This is used by higher-level services (e.g., Sites) to reconcile scheduler state.
type ScheduledJobsAdminRepository interface {
	// UpsertByTaskName creates or updates a scheduled task identified by taskName.
	// If the task exists, updates payload and scheduled_interval; preserves last_queued_at.
	UpsertByTaskName(ctx context.Context, req domain.UpsertTaskParams) error
	// DeleteByTaskName deletes a scheduled task by its taskName. Returns true if a row was deleted.
	DeleteByTaskName(ctx context.Context, taskName string) (bool, error)
}

// JobIntrospector defines the interface for inspecting running jobs.
// Note: "running" means status='running' AND lease_expires_at > now (unexpired lease).
type JobIntrospector interface {
	// RunningJobExistsByTaskName checks if there are any running jobs with unexpired lease
	// that have the specified task_name in their metadata (scheduler.task_name).
	// Only counts jobs where status='running' AND lease_expires_at > now.
	RunningJobExistsByTaskName(ctx context.Context, taskName string, now time.Time) (bool, error)
	// JobStatesByTaskName returns a bitmask describing which overrun states currently exist for the task.
	JobStatesByTaskName(ctx context.Context, taskName string, now time.Time) (domain.OverrunStateMask, error)
}

// JobScheduler defines the interface for the scheduler service.
type JobScheduler interface {
	// Tick processes due scheduled tasks and enqueues jobs according to strategy.
	// Returns the number of tasks processed.
	Tick(ctx context.Context, now time.Time) (int, error)
}

// SchedulerConfig holds configuration for the job scheduler.
type SchedulerConfig struct {
	BatchSize       int                    `json:"batch_size"`
	DefaultJobType  model.JobType          `json:"default_job_type"`
	DefaultPriority int                    `json:"default_priority"`
	MaxRetries      int                    `json:"max_retries"`
	Strategy        domain.StrategyOptions `json:"strategy"`
}

// DefaultSchedulerConfig returns a SchedulerConfig with sensible defaults.
func DefaultSchedulerConfig() SchedulerConfig {
	return SchedulerConfig{
		BatchSize:       25,
		DefaultJobType:  model.JobTypeBrowser,
		DefaultPriority: 0,
		MaxRetries:      3,
		Strategy: domain.StrategyOptions{
			Overrun:       domain.OverrunPolicySkip,
			OverrunStates: domain.OverrunStatesDefault,
		},
	}
}

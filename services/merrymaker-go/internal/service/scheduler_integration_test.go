package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// jobRepoAdapter adapts data.JobRepo to core.JobRepository for integration wiring.
type jobRepoAdapter struct{ r *data.JobRepo }

func (a *jobRepoAdapter) Create(ctx context.Context, req *model.CreateJobRequest) (*model.Job, error) {
	return a.r.Create(ctx, req)
}

func (a *jobRepoAdapter) GetByID(ctx context.Context, id string) (*model.Job, error) {
	return a.r.GetByID(ctx, id)
}

func (a *jobRepoAdapter) ReserveNext(ctx context.Context, jobType model.JobType, leaseSeconds int) (*model.Job, error) {
	return a.r.ReserveNext(ctx, jobType, leaseSeconds)
}

func (a *jobRepoAdapter) WaitForNotification(ctx context.Context, jobType model.JobType) error {
	return a.r.WaitForNotification(ctx, jobType)
}

func (a *jobRepoAdapter) Heartbeat(ctx context.Context, jobID string, leaseSeconds int) (bool, error) {
	return a.r.Heartbeat(ctx, jobID, leaseSeconds)
}

func (a *jobRepoAdapter) Complete(ctx context.Context, id string) (bool, error) {
	return a.r.Complete(ctx, id)
}

func (a *jobRepoAdapter) Fail(ctx context.Context, id, errMsg string) (bool, error) {
	return a.r.Fail(ctx, id, errMsg)
}

func (a *jobRepoAdapter) Stats(ctx context.Context, jobType model.JobType) (*model.JobStats, error) {
	return a.r.Stats(ctx, jobType)
}

func (a *jobRepoAdapter) List(ctx context.Context, opts *model.JobListOptions) ([]*model.JobWithEventCount, error) {
	return a.r.List(ctx, opts)
}

func (a *jobRepoAdapter) Delete(ctx context.Context, id string) error {
	return a.r.Delete(ctx, id)
}

func (a *jobRepoAdapter) DeleteByPayloadField(
	ctx context.Context,
	params core.DeleteByPayloadFieldParams,
) (int, error) {
	return a.r.DeleteByPayloadField(ctx, params)
}

// scheduledJobsRepoAdapter adapts data.ScheduledJobsRepo to core.ScheduledJobsRepository.
type scheduledJobsRepoAdapter struct{ r *data.ScheduledJobsRepo }

func (a *scheduledJobsRepoAdapter) FindDue(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]domain.ScheduledTask, error) {
	return a.r.FindDue(ctx, now, limit)
}

func (a *scheduledJobsRepoAdapter) FindDueTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.FindDueParams,
) ([]domain.ScheduledTask, error) {
	return a.r.FindDueTx(ctx, tx, p)
}

func (a *scheduledJobsRepoAdapter) MarkQueued(ctx context.Context, id string, now time.Time) (bool, error) {
	return a.r.MarkQueued(ctx, id, now)
}

func (a *scheduledJobsRepoAdapter) MarkQueuedTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.MarkQueuedParams,
) (bool, error) {
	return a.r.MarkQueuedTx(ctx, tx, p)
}

func (a *scheduledJobsRepoAdapter) TryWithTaskLock(
	ctx context.Context,
	taskName string,
	fn func(context.Context, *sql.Tx) error,
) (bool, error) {
	return a.r.TryWithTaskLock(ctx, taskName, fn)
}

func (a *scheduledJobsRepoAdapter) UpdateActiveFireKeyTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.UpdateActiveFireKeyParams,
) error {
	return a.r.UpdateActiveFireKeyTx(ctx, tx, p)
}

// jobIntrospectorAdapter adapts data.JobRepo to core.JobIntrospector.
type jobIntrospectorAdapter struct{ r *data.JobRepo }

func (a *jobIntrospectorAdapter) RunningJobExistsByTaskName(
	ctx context.Context,
	taskName string,
	now time.Time,
) (bool, error) {
	return a.r.RunningJobExistsByTaskName(ctx, taskName, now)
}

func (a *jobIntrospectorAdapter) JobStatesByTaskName(
	ctx context.Context,
	taskName string,
	now time.Time,
) (domain.OverrunStateMask, error) {
	return a.r.JobStatesByTaskName(ctx, taskName, now)
}

func TestSchedulerService_Integration_QueuePolicy(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		now := time.Now()

		// Clean up any existing data
		_, err := db.Exec("DELETE FROM jobs")
		require.NoError(t, err)
		_, err = db.Exec("DELETE FROM scheduled_jobs")
		require.NoError(t, err)

		// Create repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		scheduledJobsRepo := data.NewScheduledJobsRepo(db)

		// Create adapters
		jobAdapter := &jobRepoAdapter{r: jobRepo}
		scheduledAdapter := &scheduledJobsRepoAdapter{r: scheduledJobsRepo}
		introspectorAdapter := &jobIntrospectorAdapter{r: jobRepo}

		// Create scheduler with Queue policy
		cfg := core.DefaultSchedulerConfig()
		cfg.Strategy.Overrun = domain.OverrunPolicyQueue
		cfg.BatchSize = 10

		scheduler := NewSchedulerService(SchedulerServiceOptions{
			Repo:            scheduledAdapter,
			Jobs:            jobAdapter,
			JobIntrospector: introspectorAdapter,
			Config:          &cfg,
		})

		// Insert a scheduled task
		taskID := insertScheduledTask(t, db, "test-task-queue")

		// Run scheduler tick
		processed, err := scheduler.Tick(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)

		// Verify job was created
		jobs := getJobsByTaskName(t, db, "test-task-queue")
		require.Len(t, jobs, 1)
		assert.Equal(t, model.JobTypeBrowser, jobs[0].Type)
		assert.JSONEq(t, `{"url": "https://example.com"}`, string(jobs[0].Payload))

		// Verify metadata
		var metadata map[string]any
		err = json.Unmarshal(jobs[0].Metadata, &metadata)
		require.NoError(t, err)
		assert.Equal(t, "test-task-queue", metadata["scheduler.task_name"])
		assert.Equal(t, "30s", metadata["scheduler.interval"])

		// Verify last_queued_at was updated
		var lastQueued sql.NullTime
		err = db.QueryRowContext(ctx, "SELECT last_queued_at FROM scheduled_jobs WHERE id = $1", taskID).
			Scan(&lastQueued)
		require.NoError(t, err)
		assert.True(t, lastQueued.Valid)
	})
}

func TestSchedulerService_Integration_SkipPolicy_RunningJob(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		now := time.Now()

		// Clean up any existing data
		_, err := db.Exec("DELETE FROM jobs")
		require.NoError(t, err)
		_, err = db.Exec("DELETE FROM scheduled_jobs")
		require.NoError(t, err)

		// Create repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		scheduledJobsRepo := data.NewScheduledJobsRepo(db)

		// Create adapters
		jobAdapter := &jobRepoAdapter{r: jobRepo}
		scheduledAdapter := &scheduledJobsRepoAdapter{r: scheduledJobsRepo}
		introspectorAdapter := &jobIntrospectorAdapter{r: jobRepo}

		// Create scheduler with Skip policy
		cfg := core.DefaultSchedulerConfig()
		cfg.Strategy.Overrun = domain.OverrunPolicySkip

		scheduler := NewSchedulerService(SchedulerServiceOptions{
			Repo:            scheduledAdapter,
			Jobs:            jobAdapter,
			JobIntrospector: introspectorAdapter,
			Config:          &cfg,
		})

		// Insert a scheduled task
		taskID := insertScheduledTask(t, db, "test-task-skip")

		// Create a running job with the same task name
		createRunningJob(t, db, "test-task-skip", now.Add(5*time.Minute))

		// Run scheduler tick - should skip due to running job
		processed, err := scheduler.Tick(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, 1, processed) // Task was processed but job was not enqueued

		// Verify no new job was created (only the existing running job)
		jobs := getJobsByTaskName(t, db, "test-task-skip")
		require.Len(t, jobs, 1)
		assert.Equal(t, model.JobStatus("running"), jobs[0].Status)

		// Verify last_queued_at was still updated (Skip policy updates timestamp)
		var lastQueued sql.NullTime
		err = db.QueryRowContext(ctx, "SELECT last_queued_at FROM scheduled_jobs WHERE id = $1", taskID).
			Scan(&lastQueued)
		require.NoError(t, err)
		assert.True(t, lastQueued.Valid)
	})
}

func TestSchedulerService_Integration_SkipPolicy_PendingState(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		now := time.Now()

		_, err := db.Exec("DELETE FROM jobs")
		require.NoError(t, err)
		_, err = db.Exec("DELETE FROM scheduled_jobs")
		require.NoError(t, err)

		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		scheduledJobsRepo := data.NewScheduledJobsRepo(db)

		jobAdapter := &jobRepoAdapter{r: jobRepo}
		scheduledAdapter := &scheduledJobsRepoAdapter{r: scheduledJobsRepo}
		introspectorAdapter := &jobIntrospectorAdapter{r: jobRepo}

		cfg := core.DefaultSchedulerConfig()
		cfg.Strategy.Overrun = domain.OverrunPolicySkip

		scheduler := NewSchedulerService(SchedulerServiceOptions{
			Repo:            scheduledAdapter,
			Jobs:            jobAdapter,
			JobIntrospector: introspectorAdapter,
			Config:          &cfg,
		})

		policy := domain.OverrunPolicySkip
		states := domain.OverrunStateRunning | domain.OverrunStatePending | domain.OverrunStateRetrying
		taskID := insertScheduledTaskWith(t, db, "test-task-pending", ScheduledTaskOpts{
			OverrunPolicy: &policy,
			OverrunStates: &states,
		})

		createPendingJob(t, db, "test-task-pending", 0)

		processed, err := scheduler.Tick(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)

		jobs := getJobsByTaskName(t, db, "test-task-pending")
		require.Len(t, jobs, 1)
		assert.Equal(t, "pending", string(jobs[0].Status))

		var lastQueued sql.NullTime
		err = db.QueryRowContext(ctx, "SELECT last_queued_at FROM scheduled_jobs WHERE id = $1", taskID).
			Scan(&lastQueued)
		require.NoError(t, err)
		assert.True(t, lastQueued.Valid)
	})
}

func TestSchedulerService_Integration_ConcurrentSchedulers(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		now := time.Now()

		// Clean up any existing data
		_, err := db.Exec("DELETE FROM jobs")
		require.NoError(t, err)
		_, err = db.Exec("DELETE FROM scheduled_jobs")
		require.NoError(t, err)

		// Create repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		scheduledJobsRepo := data.NewScheduledJobsRepo(db)

		// Create two scheduler instances (simulating concurrent replicas)
		createScheduler := func() *SchedulerService {
			jobAdapter := &jobRepoAdapter{r: jobRepo}
			scheduledAdapter := &scheduledJobsRepoAdapter{r: scheduledJobsRepo}
			introspectorAdapter := &jobIntrospectorAdapter{r: jobRepo}

			cfg := core.DefaultSchedulerConfig()
			cfg.Strategy.Overrun = domain.OverrunPolicyQueue

			return NewSchedulerService(SchedulerServiceOptions{
				Repo:            scheduledAdapter,
				Jobs:            jobAdapter,
				JobIntrospector: introspectorAdapter,
				Config:          &cfg,
			})
		}

		scheduler1 := createScheduler()
		scheduler2 := createScheduler()

		// Insert a scheduled task with unique name to avoid conflicts
		taskName := fmt.Sprintf("test-task-concurrent-%d", now.UnixNano())
		taskID := insertScheduledTask(t, db, taskName)

		// Verify exactly one task was created
		var taskCount int
		err = db.QueryRow("SELECT COUNT(*) FROM scheduled_jobs WHERE task_name = $1", taskName).Scan(&taskCount)
		require.NoError(t, err)
		require.Equal(t, 1, taskCount, "Exactly one scheduled task should exist")

		// Log initial state for debugging
		t.Logf("Created task %s with ID %s", taskName, taskID)

		// Run both schedulers concurrently
		done1 := make(chan int)
		done2 := make(chan int)

		go func() {
			processed, err := scheduler1.Tick(ctx, now)
			assert.NoError(t, err)
			t.Logf("Scheduler 1 processed %d tasks", processed)
			done1 <- processed
		}()

		go func() {
			processed, err := scheduler2.Tick(ctx, now)
			assert.NoError(t, err)
			t.Logf("Scheduler 2 processed %d tasks", processed)
			done2 <- processed
		}()

		processed1 := <-done1
		processed2 := <-done2

		// Log results for debugging
		t.Logf("Final results: Scheduler 1: %d, Scheduler 2: %d", processed1, processed2)

		// Exactly one scheduler should have processed the task
		totalProcessed := processed1 + processed2
		if totalProcessed != 1 {
			// Additional debugging when test fails
			jobs := getJobsByTaskName(t, db, taskName)
			t.Logf("Jobs created: %d", len(jobs))
			for i, job := range jobs {
				t.Logf("Job %d: ID=%s, Status=%s", i+1, job.ID, job.Status)
			}
		}
		assert.Equal(t, 1, totalProcessed, "Exactly one scheduler should process the task")

		// Verify exactly one job was created (one scheduler should have succeeded)
		jobs := getJobsByTaskName(t, db, taskName)
		assert.Len(t, jobs, 1, "Exactly one job should be created despite concurrent schedulers")

		// Verify the job has the correct properties
		if len(jobs) > 0 {
			assert.Equal(t, model.JobTypeBrowser, jobs[0].Type)
			assert.JSONEq(t, `{"url": "https://example.com"}`, string(jobs[0].Payload))
		}
	})
}

// Helper functions

// ScheduledTaskOpts provides optional overrides for insertScheduledTaskWith.
type ScheduledTaskOpts struct {
	Payload       string
	Interval      string
	LastQueued    *time.Time
	OverrunPolicy *domain.OverrunPolicy
	OverrunStates *domain.OverrunStateMask
}

// insertScheduledTask creates a scheduled task with default values for common test cases.
func insertScheduledTask(t *testing.T, db *sql.DB, taskName string) string {
	return insertScheduledTaskWith(t, db, taskName, ScheduledTaskOpts{})
}

// insertScheduledTaskWith creates a scheduled task with optional custom values.
func insertScheduledTaskWith(t *testing.T, db *sql.DB, taskName string, opts ScheduledTaskOpts) string {
	var taskID string
	query := `
		INSERT INTO scheduled_jobs (task_name, payload, scheduled_interval, last_queued_at, overrun_policy, overrun_state_mask)
		VALUES ($1, $2, $3::interval, $4, $5, $6)
		RETURNING id
	`

	// Apply defaults
	payload := opts.Payload
	if payload == "" {
		payload = `{"url": "https://example.com"}`
	}

	interval := opts.Interval
	if interval == "" {
		interval = "30 seconds"
	}

	var policy any
	if opts.OverrunPolicy != nil {
		policy = string(*opts.OverrunPolicy)
	}

	var states any
	if opts.OverrunStates != nil {
		states = int16(*opts.OverrunStates)
	}

	err := db.QueryRow(query, taskName, payload, interval, opts.LastQueued, policy, states).Scan(&taskID)
	require.NoError(t, err)
	return taskID
}

func createRunningJob(t *testing.T, db *sql.DB, taskName string, leaseExpires time.Time) {
	metadata := map[string]any{
		"scheduler.task_name": taskName,
	}
	metadataJSON, err := json.Marshal(metadata)
	require.NoError(t, err)

	query := `
		INSERT INTO jobs (type, status, payload, metadata, lease_expires_at)
		VALUES ($1, 'running', $2, $3, $4)
	`
	_, err = db.Exec(query, model.JobTypeBrowser, `{}`, metadataJSON, leaseExpires)
	require.NoError(t, err)
}

func createPendingJob(t *testing.T, db *sql.DB, taskName string, retryCount int) {
	metadata := map[string]any{
		"scheduler.task_name": taskName,
	}
	metadataJSON, err := json.Marshal(metadata)
	require.NoError(t, err)

	query := `
		INSERT INTO jobs (type, status, payload, metadata, retry_count)
		VALUES ($1, 'pending', $2, $3, $4)
	`
	_, err = db.Exec(query, model.JobTypeBrowser, `{}`, metadataJSON, retryCount)
	require.NoError(t, err)
}

func getJobsByTaskName(t *testing.T, db *sql.DB, taskName string) []model.Job {
	query := `
		SELECT id, type, status, payload, metadata, created_at
		FROM jobs
		WHERE metadata->>'scheduler.task_name' = $1
		ORDER BY created_at
	`
	rows, err := db.Query(query, taskName)
	require.NoError(t, err)
	defer rows.Close()

	var jobs []model.Job
	for rows.Next() {
		var job model.Job
		err := rows.Scan(&job.ID, &job.Type, &job.Status, &job.Payload, &job.Metadata, &job.CreatedAt)
		require.NoError(t, err)
		jobs = append(jobs, job)
	}
	require.NoError(t, rows.Err())
	return jobs
}

func TestSchedulerService_Integration_SkipPolicy_ExpiredLease(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		now := time.Now()

		// Clean up any existing data
		_, err := db.Exec("DELETE FROM jobs")
		require.NoError(t, err)
		_, err = db.Exec("DELETE FROM scheduled_jobs")
		require.NoError(t, err)

		// Create repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		scheduledJobsRepo := data.NewScheduledJobsRepo(db)

		// Create adapters
		jobAdapter := &jobRepoAdapter{r: jobRepo}
		scheduledAdapter := &scheduledJobsRepoAdapter{r: scheduledJobsRepo}
		introspectorAdapter := &jobIntrospectorAdapter{r: jobRepo}

		// Create scheduler with Skip policy
		cfg := core.DefaultSchedulerConfig()
		cfg.Strategy.Overrun = domain.OverrunPolicySkip

		scheduler := NewSchedulerService(SchedulerServiceOptions{
			Repo:            scheduledAdapter,
			Jobs:            jobAdapter,
			JobIntrospector: introspectorAdapter,
			Config:          &cfg,
		})

		// Insert a scheduled task
		taskID := insertScheduledTask(t, db, "test-task-expired")

		// Create a running job with EXPIRED lease (lease_expires_at in the past)
		createRunningJob(t, db, "test-task-expired", now.Add(-5*time.Minute))

		// Run scheduler tick - should NOT skip because lease is expired
		processed, err := scheduler.Tick(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, 1, processed) // Task was processed and job was enqueued

		// Verify a new job was created (in addition to the expired one)
		jobs := getJobsByTaskName(t, db, "test-task-expired")
		require.GreaterOrEqual(t, len(jobs), 1, "Should have at least the new job")

		// Find the new job (should be pending)
		var newJobFound bool
		for _, job := range jobs {
			if job.Status == model.JobStatusPending {
				newJobFound = true
				assert.JSONEq(t, `{"url": "https://example.com"}`, string(job.Payload))
				break
			}
		}
		require.True(t, newJobFound, "Should have created a new pending job")

		// Verify last_queued_at was updated
		var lastQueued sql.NullTime
		err = db.QueryRowContext(ctx, "SELECT last_queued_at FROM scheduled_jobs WHERE id = $1", taskID).
			Scan(&lastQueued)
		require.NoError(t, err)
		assert.True(t, lastQueued.Valid)
	})
}

func TestSchedulerService_Integration_ReschedulePolicy(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		now := time.Now()

		// Clean up any existing data
		_, err := db.Exec("DELETE FROM jobs")
		require.NoError(t, err)
		_, err = db.Exec("DELETE FROM scheduled_jobs")
		require.NoError(t, err)

		// Create repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		scheduledJobsRepo := data.NewScheduledJobsRepo(db)

		// Create adapters
		jobAdapter := &jobRepoAdapter{r: jobRepo}
		scheduledAdapter := &scheduledJobsRepoAdapter{r: scheduledJobsRepo}
		introspectorAdapter := &jobIntrospectorAdapter{r: jobRepo}

		// Create scheduler with Reschedule policy
		cfg := core.DefaultSchedulerConfig()
		cfg.Strategy.Overrun = domain.OverrunPolicyReschedule

		scheduler := NewSchedulerService(SchedulerServiceOptions{
			Repo:            scheduledAdapter,
			Jobs:            jobAdapter,
			JobIntrospector: introspectorAdapter,
			Config:          &cfg,
		})

		// Insert a scheduled task
		taskID := insertScheduledTask(t, db, "test-task-reschedule")

		// Run scheduler tick
		processed, err := scheduler.Tick(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, 1, processed) // Task was processed

		// Verify NO job was created (reschedule policy doesn't enqueue)
		jobs := getJobsByTaskName(t, db, "test-task-reschedule")
		assert.Empty(t, jobs, "Reschedule policy should not create jobs")

		// Verify last_queued_at was updated (reschedule still updates timestamp)
		var lastQueued sql.NullTime
		err = db.QueryRowContext(ctx, "SELECT last_queued_at FROM scheduled_jobs WHERE id = $1", taskID).
			Scan(&lastQueued)
		require.NoError(t, err)
		assert.True(t, lastQueued.Valid, "Reschedule policy should update last_queued_at")
		assert.WithinDuration(t, now, lastQueued.Time, time.Second)
	})
}

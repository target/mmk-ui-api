package data

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduledJobsRepo_FindDue(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()
		now := time.Now()

		// Use unique task names and IDs to avoid conflicts with other tests
		taskPrefix := fmt.Sprintf("finddue_%d_", now.UnixNano())

		// Insert test data with unique UUIDs
		_, err := db.ExecContext(ctx, `
			INSERT INTO scheduled_jobs (task_name, payload, scheduled_interval, last_queued_at)
			VALUES
				($1, '{"key": "value1"}', '5 minutes', NULL),
				($2, '{"key": "value2"}', '10 minutes', $3),
				($4, '{"key": "value3"}', '1 hour', $5),
				($6, '{"key": "value4"}', '30 minutes', $7)
		`, taskPrefix+"task1", taskPrefix+"task2", now.Add(-5*time.Minute), taskPrefix+"task3", now.Add(-2*time.Hour), taskPrefix+"task4", now.Add(-1*time.Minute))
		require.NoError(t, err)

		// Test finding due tasks - only look for our specific tasks
		allTasks, err := repo.FindDue(ctx, now, 500)
		require.NoError(t, err)

		// Filter to only our test tasks
		var ourTasks []string
		for _, task := range allTasks {
			if strings.HasPrefix(task.TaskName, taskPrefix) {
				ourTasks = append(ourTasks, task.TaskName)
			}
		}

		// Should find:
		// - task1 (never queued) ✓
		// - task3 (last queued 2 hours ago, interval 1 hour) ✓
		// Should NOT find:
		// - task2 (last queued 5 minutes ago, interval 10 minutes) - not due yet
		// - task4 (last queued 1 minute ago, interval 30 minutes) - not due yet
		assert.Len(t, ourTasks, 2)
		assert.Contains(t, ourTasks, taskPrefix+"task1")
		assert.Contains(t, ourTasks, taskPrefix+"task3")
	})
}

func TestScheduledJobsRepo_FindDue_WithLimit(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()
		now := time.Now()

		// Insert multiple due tasks with unique names
		taskPrefix := fmt.Sprintf("limit_test_%d_", now.UnixNano())
		for i := 1; i <= 5; i++ {
			_, err := db.ExecContext(ctx, `
				INSERT INTO scheduled_jobs (task_name, payload, scheduled_interval, last_queued_at)
				VALUES ($1, '{}', '5 minutes', NULL)
			`, fmt.Sprintf("%stask_%d", taskPrefix, i))
			require.NoError(t, err)
		}

		// Test with limit
		tasks, err := repo.FindDue(ctx, now, 3)
		require.NoError(t, err)
		assert.Len(t, tasks, 3)
	})
}

func TestScheduledJobsRepo_FindDue_InvalidLimit(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()
		now := time.Now()

		// Test with invalid limit
		_, err := repo.FindDue(ctx, now, 0)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "limit must be positive")

		_, err = repo.FindDue(ctx, now, -1)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "limit must be positive")
	})
}

func TestScheduledJobsRepo_MarkQueued(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		timeProvider := NewFixedTimeProvider(time.Now())
		repo := NewScheduledJobsRepoWithTimeProvider(db, timeProvider)
		ctx := context.Background()
		now := time.Now()

		// Insert test task with unique name and get its ID
		taskName := fmt.Sprintf("mark_queued_test_%d", now.UnixNano())
		var taskID string
		err := db.QueryRowContext(ctx, `
			INSERT INTO scheduled_jobs (task_name, payload, scheduled_interval, last_queued_at)
			VALUES ($1, '{}', '5 minutes', NULL)
			RETURNING id
		`, taskName).Scan(&taskID)
		require.NoError(t, err)

		// Mark as queued
		found, err := repo.MarkQueued(ctx, taskID, now)
		require.NoError(t, err)
		assert.True(t, found)

		// Verify the update
		var lastQueued sql.NullTime
		err = db.QueryRowContext(ctx, "SELECT last_queued_at FROM scheduled_jobs WHERE id = $1", taskID).
			Scan(&lastQueued)
		require.NoError(t, err)
		assert.True(t, lastQueued.Valid)
		assert.WithinDuration(t, now, lastQueued.Time, time.Second)
	})
}

func TestScheduledJobsRepo_MarkQueued_NotFound(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()
		now := time.Now()

		// Try to mark non-existent task
		found, err := repo.MarkQueued(ctx, "99999999-9999-9999-9999-999999999999", now)
		require.NoError(t, err)
		assert.False(t, found)
	})
}

func TestScheduledJobsRepo_TryWithTaskLock(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()

		executed := false
		taskName := "test_task"

		// Test successful lock acquisition and execution
		locked, err := repo.TryWithTaskLock(
			ctx,
			taskName,
			func(_ context.Context, _ *sql.Tx) error {
				executed = true
				return nil
			},
		)
		require.NoError(t, err)
		assert.True(t, locked)
		assert.True(t, executed)
	})
}

func TestScheduledJobsRepo_TryWithTaskLock_FunctionError(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()

		taskName := "function_error_test_task"
		expectedErr := errors.New("function failed")

		// Test lock acquired but function fails
		locked, err := repo.TryWithTaskLock(
			ctx,
			taskName,
			func(_ context.Context, _ *sql.Tx) error {
				return expectedErr
			},
		)
		assert.True(t, locked, "Lock should be acquired")
		require.Error(t, err, "Function error should be returned")
		assert.Equal(t, expectedErr, err, "Should return the exact function error")
	})
}

func TestScheduledJobsRepo_TryWithTaskLock_Concurrent(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()
		taskName := "concurrent_task"

		// Channel to coordinate goroutines
		ready := make(chan struct{})
		results := make(chan bool, 2)

		// Start two goroutines trying to acquire the same lock
		for range 2 {
			go func() {
				<-ready // Wait for signal to start
				locked, err := repo.TryWithTaskLock(
					ctx,
					taskName,
					func(_ context.Context, _ *sql.Tx) error {
						time.Sleep(100 * time.Millisecond) // Hold lock briefly
						return nil
					},
				)
				assert.NoError(t, err)
				results <- locked
			}()
		}

		// Signal both goroutines to start
		close(ready)

		// Collect results
		var lockResults []bool
		for range 2 {
			lockResults = append(lockResults, <-results)
		}

		// Exactly one should have acquired the lock
		lockedCount := 0
		for _, locked := range lockResults {
			if locked {
				lockedCount++
			}
		}
		assert.Equal(t, 1, lockedCount, "Exactly one goroutine should acquire the lock")
	})
}

func TestFnvHash(t *testing.T) {
	// Test that the same string produces the same hash
	hash1 := fnvHash("test_task")
	hash2 := fnvHash("test_task")
	assert.Equal(t, hash1, hash2)

	// Test that different strings produce different hashes
	hash3 := fnvHash("different_task")
	assert.NotEqual(t, hash1, hash3)
}

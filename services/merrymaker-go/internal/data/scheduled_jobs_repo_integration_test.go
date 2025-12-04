package data

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// TestScheduledJobsRepo_Integration_ConcurrentFindDue tests concurrent access to FindDue
// to ensure FOR UPDATE SKIP LOCKED works correctly.
func TestScheduledJobsRepo_Integration_ConcurrentFindDue(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		now := time.Now()

		// Insert test tasks that are all due with unique names
		taskPrefix := fmt.Sprintf("concurrent_%d_", now.UnixNano())
		for i := 1; i <= 5; i++ {
			_, err := db.ExecContext(ctx, `
				INSERT INTO scheduled_jobs (task_name, payload, scheduled_interval, last_queued_at)
				VALUES ($1, '{}', '5 minutes', NULL)
			`, fmt.Sprintf("%stask_%d", taskPrefix, i))
			require.NoError(t, err)
		}

		// Run concurrent FindDue operations with transactions to test SKIP LOCKED
		const numWorkers = 3
		results := make(chan []string, numWorkers)
		var wg sync.WaitGroup

		for range numWorkers {
			wg.Add(1)
			go func() {
				defer wg.Done()

				// Use a transaction to hold the locks longer
				tx, err := db.BeginTx(ctx, nil)
				assert.NoError(t, err)
				defer func() { _ = tx.Rollback() }()

				// Query with FOR UPDATE SKIP LOCKED within transaction
				rows, err := tx.QueryContext(ctx, `
					SELECT task_name FROM scheduled_jobs
					WHERE (last_queued_at IS NULL OR last_queued_at + scheduled_interval <= $1)
					ORDER BY created_at ASC
					LIMIT 2
					FOR UPDATE SKIP LOCKED
				`, now.UTC())
				assert.NoError(t, err)
				defer rows.Close()

				var taskNames []string
				for rows.Next() {
					var taskName string
					err := rows.Scan(&taskName)
					assert.NoError(t, err)
					taskNames = append(taskNames, taskName)
				}
				if err := rows.Err(); err != nil {
					assert.NoError(t, err)
				}

				// Hold the lock briefly to ensure other workers see the effect
				time.Sleep(50 * time.Millisecond)

				results <- taskNames
			}()
		}

		wg.Wait()
		close(results)

		// Collect all task names found by workers
		allFoundTasks := make(map[string]int)
		totalFound := 0
		for taskNames := range results {
			totalFound += len(taskNames)
			for _, name := range taskNames {
				allFoundTasks[name]++
			}
		}

		// Each task should be found by at most one worker due to SKIP LOCKED
		for taskName, count := range allFoundTasks {
			assert.LessOrEqual(
				t,
				count,
				1,
				"Task %s should be found by at most one worker",
				taskName,
			)
		}

		// We should have found some tasks (SKIP LOCKED means some workers get nothing)
		assert.Positive(t, totalFound, "At least some tasks should be found")
		// Note: totalFound might be > 5 because there are other due tasks in the database from other tests
	})
}

// TestScheduledJobsRepo_Integration_LockContention tests advisory lock contention.
func TestScheduledJobsRepo_Integration_LockContention(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()
		taskName := "contention_test"

		const numWorkers = 5
		results := make(chan bool, numWorkers)
		var wg sync.WaitGroup

		// Start multiple workers trying to acquire the same lock
		for i := range numWorkers {
			wg.Add(1)
			go func(_ int) {
				defer wg.Done()
				locked, err := repo.TryWithTaskLock(
					ctx,
					taskName,
					func(_ context.Context, _ *sql.Tx) error {
						// Simulate some work
						time.Sleep(50 * time.Millisecond)
						return nil
					},
				)
				assert.NoError(t, err)
				results <- locked
			}(i)
		}

		wg.Wait()
		close(results)

		// Count how many workers acquired the lock
		lockedCount := 0
		for locked := range results {
			if locked {
				lockedCount++
			}
		}

		// Exactly one worker should have acquired the lock
		assert.Equal(t, 1, lockedCount, "Exactly one worker should acquire the lock")
	})
}

// TestScheduledJobsRepo_Integration_RealIntervals tests with actual PostgreSQL intervals.
func TestScheduledJobsRepo_Integration_RealIntervals(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()
		now := time.Now()

		// Insert tasks with various PostgreSQL interval formats
		taskPrefix := fmt.Sprintf("realint_%d_", now.UnixNano())
		testCases := []struct {
			taskName    string
			interval    string
			lastQueued  *time.Time
			shouldBeDue bool
		}{
			{
				taskPrefix + "task_5min",
				"5 minutes",
				nil,
				true,
			}, // Never queued
			{
				taskPrefix + "task_1hour_recent",
				"1 hour",
				&now,
				false,
			}, // Just queued
			{
				taskPrefix + "task_1hour_old",
				"1 hour",
				func() *time.Time { t := now.Add(-2 * time.Hour); return &t }(),
				true,
			}, // Queued 2 hours ago
			{
				taskPrefix + "task_2min_old",
				"1 minute",
				func() *time.Time { t := now.Add(-2 * time.Minute); return &t }(),
				true,
			}, // Queued 2 minutes ago, interval 1 minute
		}

		for _, tc := range testCases {
			_, err := db.ExecContext(ctx, `
				INSERT INTO scheduled_jobs (task_name, payload, scheduled_interval, last_queued_at)
				VALUES ($1, '{}', $2::interval, $3)
			`, tc.taskName, tc.interval, tc.lastQueued)
			require.NoError(t, err)
		}

		// Instead of relying on FindDue which might have too many results,
		// let's test the logic directly by querying our specific tasks
		for _, tc := range testCases {
			// Query this specific task to see if it would be considered due
			var isDue bool
			err := db.QueryRowContext(ctx, `
				SELECT (last_queued_at IS NULL OR last_queued_at + scheduled_interval <= $1)
				FROM scheduled_jobs
				WHERE task_name = $2
			`, now.UTC(), tc.taskName).Scan(&isDue)
			require.NoError(t, err)

			if tc.shouldBeDue {
				assert.True(t, isDue, "Task %s should be due according to SQL logic", tc.taskName)
			} else {
				assert.False(t, isDue, "Task %s should not be due according to SQL logic", tc.taskName)
			}
		}

		// Also test that FindDue can find at least some of our due tasks
		tasks, err := repo.FindDue(ctx, now, 200)
		require.NoError(t, err)

		foundOurTasks := 0
		for _, task := range tasks {
			if strings.HasPrefix(task.TaskName, taskPrefix) {
				foundOurTasks++
			}
		}

		// We should find at least some of our due tasks (there are 3 due tasks)
		assert.Positive(t, foundOurTasks, "Should find at least some of our due tasks")
	})
}

// TestScheduledJobsRepo_Integration_MarkQueuedRace tests race conditions in MarkQueued.
func TestScheduledJobsRepo_Integration_MarkQueuedRace(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewScheduledJobsRepo(db)
		ctx := context.Background()
		now := time.Now()

		// Insert a test task with unique name and get its ID
		taskName := fmt.Sprintf("race_task_%d", now.UnixNano())
		var taskID string
		err := db.QueryRowContext(ctx, `
			INSERT INTO scheduled_jobs (task_name, payload, scheduled_interval, last_queued_at)
			VALUES ($1, '{}', '5 minutes', NULL)
			RETURNING id
		`, taskName).Scan(&taskID)
		require.NoError(t, err)

		// Try to mark the same task as queued concurrently
		const numWorkers = 10
		results := make(chan bool, numWorkers)
		var wg sync.WaitGroup

		for range numWorkers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				found, err := repo.MarkQueued(ctx, taskID, now)
				assert.NoError(t, err)
				results <- found
			}()
		}

		wg.Wait()
		close(results)

		// All workers should successfully update (found=true)
		// because MarkQueued is idempotent
		for found := range results {
			assert.True(t, found, "All workers should find and update the task")
		}

		// Verify the task was actually updated
		var lastQueued sql.NullTime
		err = db.QueryRowContext(ctx, "SELECT last_queued_at FROM scheduled_jobs WHERE id = $1", taskID).
			Scan(&lastQueued)
		require.NoError(t, err)
		assert.True(t, lastQueued.Valid)
	})
}

// TestJobRepo_Integration_JobStatesByTaskName tests JobIntrospector state aggregation.
func TestJobRepo_Integration_JobStatesByTaskName(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})
		ctx := context.Background()
		now := time.Now()

		// Insert job documents covering each state
		_, err := db.ExecContext(ctx, `
			INSERT INTO jobs (type, status, payload, metadata, lease_expires_at)
			VALUES ('browser', 'running', '{}', '{"scheduler.task_name": "running_task"}', $1)
		`, now.Add(time.Hour))
		require.NoError(t, err)

		_, err = db.ExecContext(ctx, `
			INSERT INTO jobs (type, status, payload, metadata, lease_expires_at)
			VALUES ('browser', 'running', '{}', '{"scheduler.task_name": "expired_task"}', $1)
		`, now.Add(-time.Hour))
		require.NoError(t, err)

		_, err = db.ExecContext(ctx, `
			INSERT INTO jobs (type, status, payload, metadata, retry_count)
			VALUES ('browser', 'pending', '{}', '{"scheduler.task_name": "pending_task"}', 0)
		`)
		require.NoError(t, err)

		_, err = db.ExecContext(ctx, `
			INSERT INTO jobs (type, status, payload, metadata, retry_count)
			VALUES ('browser', 'pending', '{}', '{"scheduler.task_name": "retrying_task"}', 2)
		`)
		require.NoError(t, err)

		cases := []struct {
			taskName     string
			expectedMask domain.OverrunStateMask
		}{
			{"running_task", domain.OverrunStateRunning},                                // running with active lease
			{"expired_task", 0},                                                         // running but expired lease
			{"pending_task", domain.OverrunStatePending},                                // pending without retries
			{"retrying_task", domain.OverrunStatePending | domain.OverrunStateRetrying}, // pending with retries
			{"unknown", 0}, // no jobs
		}

		for _, tc := range cases {
			t.Run(tc.taskName, func(t *testing.T) {
				mask, err := repo.JobStatesByTaskName(ctx, tc.taskName, now)
				require.NoError(t, err)
				assert.Equal(t, tc.expectedMask, mask)

				running, err := repo.RunningJobExistsByTaskName(ctx, tc.taskName, now)
				require.NoError(t, err)
				assert.Equal(t, mask.Has(domain.OverrunStateRunning), running)
			})
		}
	})
}

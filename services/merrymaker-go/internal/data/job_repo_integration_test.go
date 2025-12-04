package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestJobRepo_Integration_CreateAndReserve tests the full flow of creating and reserving jobs.
func TestJobRepo_Integration_CreateAndReserve(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Create multiple jobs with different priorities
		jobs := []*model.CreateJobRequest{
			{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(`{"url": "https://low-priority.com"}`),
				Priority: 25,
			},
			{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(`{"url": "https://high-priority.com"}`),
				Priority: 75,
			},
			{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(`{"url": "https://medium-priority.com"}`),
				Priority: 50,
			},
		}

		for _, req := range jobs {
			_, err := repo.Create(context.Background(), req)
			require.NoError(t, err)
		}

		// Reserve jobs and verify they come out in priority order
		reserved1, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		assert.Equal(t, 75, reserved1.Priority) // Highest priority first

		reserved2, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		assert.Equal(t, 50, reserved2.Priority) // Medium priority second

		reserved3, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		assert.Equal(t, 25, reserved3.Priority) // Lowest priority last

		// No more jobs available
		_, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.ErrorIs(t, err, model.ErrNoJobsAvailable)
	})
}

// TestJobRepo_Integration_JobLifecycle tests the complete lifecycle of a job.
func TestJobRepo_Integration_JobLifecycle(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Use a fixed time provider to control time for retry delays
		fixedTime := testutil.TestTime()
		timeProvider := NewFixedTimeProvider(fixedTime)
		repo := NewJobRepo(db, RepoConfig{
			RetryDelaySeconds: 5,
			TimeProvider:      timeProvider,
		})

		// 1. Create a job
		req := &model.CreateJobRequest{
			Type:       model.JobTypeBrowser,
			Payload:    json.RawMessage(`{"url": "https://example.com"}`),
			MaxRetries: 2,
		}
		job, err := repo.Create(context.Background(), req)
		require.NoError(t, err)
		assert.Equal(t, model.JobStatusPending, job.Status)

		// 2. Reserve the job
		reserved, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		assert.Equal(t, job.ID, reserved.ID)
		assert.Equal(t, model.JobStatusRunning, reserved.Status)
		assert.NotNil(t, reserved.StartedAt)
		assert.NotNil(t, reserved.LeaseExpiresAt)

		// 3. Extend the lease (heartbeat)
		success, err := repo.Heartbeat(context.Background(), job.ID, 60)
		require.NoError(t, err)
		assert.True(t, success)

		// 4. Fail the job (first attempt)
		success, err = repo.Fail(context.Background(), job.ID, "first failure")
		require.NoError(t, err)
		assert.True(t, success)

		// 5. Job should be back to pending for retry, but it has a retry delay
		// Advance time beyond the retry delay (5 seconds) to make the job available
		timeProvider.AddTime(6 * time.Second)

		retryJob, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		assert.Equal(t, job.ID, retryJob.ID)
		assert.Equal(t, 1, retryJob.RetryCount)
		assert.Equal(t, "first failure", *retryJob.LastError)

		// 6. Complete the job on retry
		success, err = repo.Complete(context.Background(), job.ID)
		require.NoError(t, err)
		assert.True(t, success)

		// 7. Job should no longer be available
		_, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.ErrorIs(t, err, model.ErrNoJobsAvailable)
	})
}

// TestJobRepo_Integration_ConcurrentReservation tests concurrent job reservation.
func TestJobRepo_Integration_ConcurrentReservation(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Create a single job
		req := &model.CreateJobRequest{
			Type:    model.JobTypeBrowser,
			Payload: json.RawMessage(`{"url": "https://example.com"}`),
		}
		job, err := repo.Create(context.Background(), req)
		require.NoError(t, err)

		// Try to reserve the same job concurrently
		results := make(chan *model.Job, 2)
		errors := make(chan error, 2)

		for range 2 {
			go func() {
				reserved, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
				if err != nil {
					errors <- err
				} else {
					results <- reserved
				}
			}()
		}

		// One should succeed, one should fail
		var successCount, errorCount int
		var reservedJob *model.Job

		for range 2 {
			select {
			case job := <-results:
				successCount++
				reservedJob = job
			case err := <-errors:
				errorCount++
				require.ErrorIs(t, err, model.ErrNoJobsAvailable)
			case <-time.After(5 * time.Second):
				t.Fatal("Test timed out")
			}
		}

		assert.Equal(t, 1, successCount, "Exactly one goroutine should succeed")
		assert.Equal(t, 1, errorCount, "Exactly one goroutine should fail")
		if reservedJob != nil {
			assert.Equal(t, job.ID, reservedJob.ID)
		}
	})
}

// TestJobRepo_Integration_Stats tests job statistics.
func TestJobRepo_Integration_Stats(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Create jobs with different priorities to control reservation order
		// 2 pending jobs (lowest priorities - won't be reserved)
		for i := range 2 {
			req := &model.CreateJobRequest{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(`{"url": "https://pending.com"}`),
				Priority: 10 + i, // Low priorities: 10, 11
			}
			_, err := repo.Create(context.Background(), req)
			require.NoError(t, err)
		}

		// 1 running job (medium priority - will be reserved second)
		req := &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url": "https://running.com"}`),
			Priority: 40,
		}
		runningJob, err := repo.Create(context.Background(), req)
		require.NoError(t, err)

		// 1 completed job (highest priority - will be reserved first)
		req = &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url": "https://completed.com"}`),
			Priority: 50,
		}
		completedJob, err := repo.Create(context.Background(), req)
		require.NoError(t, err)

		// 1 failed job (third highest priority - will be reserved third)
		req = &model.CreateJobRequest{
			Type:       model.JobTypeBrowser,
			Payload:    json.RawMessage(`{"url": "https://failed.com"}`),
			Priority:   30,
			MaxRetries: 1,
		}
		failedJob, err := repo.Create(context.Background(), req)
		require.NoError(t, err)

		// Process jobs in priority order (highest first)
		// 1. Reserve and complete the completed job (priority 50)
		reserved, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		require.Equal(t, completedJob.ID, reserved.ID)
		_, err = repo.Complete(context.Background(), reserved.ID)
		require.NoError(t, err)

		// 2. Reserve the running job (priority 40) and leave it running
		reserved, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		require.Equal(t, runningJob.ID, reserved.ID)

		// 3. Reserve and fail the failed job (priority 30)
		reserved, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		require.Equal(t, failedJob.ID, reserved.ID)
		// With MaxRetries=1, first failure should immediately mark it as failed
		_, err = repo.Fail(context.Background(), reserved.ID, "failure that exceeds max retries")
		require.NoError(t, err)

		// 4. Leave the 2 pending jobs (priorities 10, 11) unreserved

		// Get stats
		stats, err := repo.Stats(context.Background(), model.JobTypeBrowser)
		require.NoError(t, err)

		assert.Equal(t, 2, stats.Pending)
		assert.Equal(t, 1, stats.Running)
		assert.Equal(t, 1, stats.Completed)
		assert.Equal(t, 1, stats.Failed)
	})
}

// TestJobRepo_Integration_NewFields tests the new site_id, source_id, and is_test fields.
func TestJobRepo_Integration_NewFields(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Test with nil values first (no foreign key constraints)
		req1 := &model.CreateJobRequest{
			Type:    model.JobTypeBrowser,
			Payload: json.RawMessage(`{"url": "https://example1.com"}`),
			IsTest:  true, // Test the is_test field
		}

		job1, err := repo.Create(context.Background(), req1)
		require.NoError(t, err)
		assert.Nil(t, job1.SiteID)
		assert.Nil(t, job1.SourceID)
		assert.True(t, job1.IsTest)

		// Reserve the job and verify fields are preserved
		reserved, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		assert.Equal(t, job1.ID, reserved.ID)
		assert.Nil(t, reserved.SiteID)
		assert.Nil(t, reserved.SourceID)
		assert.True(t, reserved.IsTest)

		// Test with is_test false
		req2 := &model.CreateJobRequest{
			Type:    model.JobTypeBrowser,
			Payload: json.RawMessage(`{"url": "https://example2.com"}`),
			IsTest:  false, // Explicitly false
		}

		job2, err := repo.Create(context.Background(), req2)
		require.NoError(t, err)
		assert.Nil(t, job2.SiteID)
		assert.Nil(t, job2.SourceID)
		assert.False(t, job2.IsTest)
	})
}

// TestJobRepo_Integration_ListBySource tests the ListBySource method.
func TestJobRepo_Integration_ListBySource(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		sourceID1 := "550e8400-e29b-41d4-a716-446655440001"
		sourceID2 := "550e8400-e29b-41d4-a716-446655440002"

		// Create required sources first
		_, err := db.ExecContext(context.Background(), `
			INSERT INTO sources (id, name, value, test)
			VALUES
				($1, 'test-source-1', 'test-value-1', false),
				($2, 'test-source-2', 'test-value-2', false)
		`, sourceID1, sourceID2)
		require.NoError(t, err)

		// Insert jobs with source references
		_, err = db.ExecContext(context.Background(), `
			INSERT INTO jobs (type, status, priority, payload, metadata, source_id, is_test)
			VALUES
				('browser', 'pending', 50, '{"url": "https://source1-job1.com"}', '{}', $1, false),
				('browser', 'pending', 50, '{"url": "https://source1-job2.com"}', '{}', $1, false),
				('browser', 'pending', 50, '{"url": "https://source2-job1.com"}', '{}', $2, false),
				('browser', 'pending', 50, '{"url": "https://no-source.com"}', '{}', NULL, false)
		`, sourceID1, sourceID2)
		require.NoError(t, err)

		// Test ListBySource for source-1
		source1Jobs, err := repo.ListBySource(
			context.Background(),
			model.JobListBySourceOptions{SourceID: sourceID1, Limit: 10, Offset: 0},
		)
		require.NoError(t, err)
		assert.Len(t, source1Jobs, 2)

		// Verify all jobs belong to source-1 and are ordered by created_at DESC
		for i, job := range source1Jobs {
			assert.Equal(t, &sourceID1, job.SourceID)
			if i > 0 {
				// Should be ordered by created_at DESC (newer first)
				assert.True(
					t,
					job.CreatedAt.After(source1Jobs[i-1].CreatedAt) || job.CreatedAt.Equal(source1Jobs[i-1].CreatedAt),
				)
			}
		}

		// Test ListBySource for source-2
		source2Jobs, err := repo.ListBySource(
			context.Background(),
			model.JobListBySourceOptions{SourceID: sourceID2, Limit: 10, Offset: 0},
		)
		require.NoError(t, err)
		assert.Len(t, source2Jobs, 1)
		assert.Equal(t, &sourceID2, source2Jobs[0].SourceID)

		// Test pagination
		source1JobsPage1, err := repo.ListBySource(
			context.Background(),
			model.JobListBySourceOptions{SourceID: sourceID1, Limit: 1, Offset: 0},
		)
		require.NoError(t, err)
		assert.Len(t, source1JobsPage1, 1)

		source1JobsPage2, err := repo.ListBySource(
			context.Background(),
			model.JobListBySourceOptions{SourceID: sourceID1, Limit: 1, Offset: 1},
		)
		require.NoError(t, err)
		assert.Len(t, source1JobsPage2, 1)

		// Should be different jobs
		assert.NotEqual(t, source1JobsPage1[0].ID, source1JobsPage2[0].ID)

		// Test non-existent source (use proper UUID format)
		noJobs, err := repo.ListBySource(
			context.Background(),
			model.JobListBySourceOptions{SourceID: "550e8400-e29b-41d4-a716-446655440999", Limit: 10, Offset: 0},
		)
		require.NoError(t, err)
		assert.Empty(t, noJobs)

		// Test limit bounds
		limitedJobs, err := repo.ListBySource(
			context.Background(),
			model.JobListBySourceOptions{SourceID: sourceID1, Limit: 1, Offset: 0},
		)
		require.NoError(t, err)
		assert.Len(t, limitedJobs, 1)
	})
}

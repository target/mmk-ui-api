package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobRepo_FailStalePendingJobs(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	t.Run("fails stale pending jobs", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a pending job that is old
			oldJob, err := repo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			// Manually update created_at to make it old
			_, err = db.ExecContext(ctx, `
				UPDATE jobs
				SET created_at = $1
				WHERE id = $2
			`, time.Now().Add(-2*time.Hour), oldJob.ID)
			require.NoError(t, err)

			// Create a recent pending job
			recentJob, err := repo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			// Fail stale pending jobs older than 1 hour (batch size 1000)
			count, err := repo.FailStalePendingJobs(ctx, 1*time.Hour, 1000)
			require.NoError(t, err)
			assert.Equal(t, int64(1), count)

			// Verify old job is now failed
			oldJobAfter, err := repo.GetByID(ctx, oldJob.ID)
			require.NoError(t, err)
			assert.Equal(t, model.JobStatusFailed, oldJobAfter.Status)
			assert.NotNil(t, oldJobAfter.LastError)
			assert.Contains(t, *oldJobAfter.LastError, "timed out in pending status")
			assert.NotNil(t, oldJobAfter.CompletedAt)

			// Verify recent job is still pending
			recentJobAfter, err := repo.GetByID(ctx, recentJob.ID)
			require.NoError(t, err)
			assert.Equal(t, model.JobStatusPending, recentJobAfter.Status)
		})
	})

	t.Run("no jobs to fail", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a recent pending job
			_, err := repo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			// Try to fail stale jobs with a very short max age (batch size 1000)
			count, err := repo.FailStalePendingJobs(ctx, 24*time.Hour, 1000)
			require.NoError(t, err)
			assert.Equal(t, int64(0), count)
		})
	})

	t.Run("does not fail running jobs", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a pending job
			job, err := repo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			// Reserve the job (makes it running)
			_, err = repo.ReserveNext(ctx, model.JobTypeBrowser, 30)
			require.NoError(t, err)

			// Make the job old
			_, err = db.ExecContext(ctx, `
				UPDATE jobs
				SET created_at = $1
				WHERE id = $2
			`, time.Now().Add(-2*time.Hour), job.ID)
			require.NoError(t, err)

			// Try to fail stale pending jobs (batch size 1000)
			count, err := repo.FailStalePendingJobs(ctx, 1*time.Hour, 1000)
			require.NoError(t, err)
			assert.Equal(t, int64(0), count)

			// Verify job is still running
			jobAfter, err := repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
			assert.Equal(t, model.JobStatusRunning, jobAfter.Status)
		})
	})
}

func TestJobRepo_DeleteOldJobs(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	t.Run("deletes old completed jobs", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a job
			job, err := repo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			// Reserve the job (makes it running)
			_, err = repo.ReserveNext(ctx, model.JobTypeBrowser, 30)
			require.NoError(t, err)

			// Complete the job
			success, err := repo.Complete(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, success)

			// Verify job is completed
			jobAfter, err := repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
			require.Equal(t, model.JobStatusCompleted, jobAfter.Status)
			require.NotNil(t, jobAfter.CompletedAt)

			// Make the job old (8 days ago)
			oldTime := time.Now().Add(-8 * 24 * time.Hour)
			_, err = db.ExecContext(ctx, `
				UPDATE jobs
				SET completed_at = $1, updated_at = $1
				WHERE id = $2
			`, oldTime, job.ID)
			require.NoError(t, err)

			// Delete old completed jobs older than 7 days (batch size 1000)
			count, err := repo.DeleteOldJobs(ctx, core.DeleteOldJobsParams{
				Status:    model.JobStatusCompleted,
				MaxAge:    7 * 24 * time.Hour,
				BatchSize: 1000,
			})
			require.NoError(t, err)
			assert.Equal(t, int64(1), count, "Expected 1 job to be deleted")

			// Verify job is deleted
			_, err = repo.GetByID(ctx, job.ID)
			assert.ErrorIs(t, err, ErrJobNotFound)
		})
	})

	t.Run("deletes old failed jobs", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a job with max_retries=1 so it fails on first failure
			job, err := repo.Create(ctx, &model.CreateJobRequest{
				Type:       model.JobTypeBrowser,
				Payload:    json.RawMessage(`{"url": "https://example.com"}`),
				MaxRetries: 1,
			})
			require.NoError(t, err)

			// Reserve the job (makes it running)
			reservedJob, err := repo.ReserveNext(ctx, model.JobTypeBrowser, 30)
			require.NoError(t, err)
			require.NotNil(t, reservedJob)
			require.Equal(t, model.JobStatusRunning, reservedJob.Status)

			// Fail the job (should mark as failed since max_retries=0)
			success, err := repo.Fail(ctx, job.ID, "test error")
			require.NoError(t, err)
			require.True(t, success, "Fail should return true")

			// Verify job is failed
			jobAfter, err := repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
			t.Logf(
				"Job status after Fail: %s, retry_count: %d, max_retries: %d",
				jobAfter.Status,
				jobAfter.RetryCount,
				jobAfter.MaxRetries,
			)
			require.Equal(t, model.JobStatusFailed, jobAfter.Status)

			// Make the job old
			_, err = db.ExecContext(ctx, `
				UPDATE jobs
				SET completed_at = $1, updated_at = $1
				WHERE id = $2
			`, time.Now().Add(-8*24*time.Hour), job.ID)
			require.NoError(t, err)

			// Delete old failed jobs older than 7 days (batch size 1000)
			count, err := repo.DeleteOldJobs(ctx, core.DeleteOldJobsParams{
				Status:    model.JobStatusFailed,
				MaxAge:    7 * 24 * time.Hour,
				BatchSize: 1000,
			})
			require.NoError(t, err)
			assert.Equal(t, int64(1), count)

			// Verify job is deleted
			_, err = repo.GetByID(ctx, job.ID)
			assert.ErrorIs(t, err, ErrJobNotFound)
		})
	})

	t.Run("does not delete recent jobs", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create and complete a job
			job, err := repo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			// Complete the job
			_, err = repo.Complete(ctx, job.ID)
			require.NoError(t, err)

			// Try to delete jobs older than 7 days (job is recent, batch size 1000)
			count, err := repo.DeleteOldJobs(ctx, core.DeleteOldJobsParams{
				Status:    model.JobStatusCompleted,
				MaxAge:    7 * 24 * time.Hour,
				BatchSize: 1000,
			})
			require.NoError(t, err)
			assert.Equal(t, int64(0), count)

			// Verify job still exists
			_, err = repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
		})
	})

	t.Run("does not delete jobs with different status", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create and complete a job
			job, err := repo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			})
			require.NoError(t, err)

			// Complete the job
			_, err = repo.Complete(ctx, job.ID)
			require.NoError(t, err)

			// Make the job old
			_, err = db.ExecContext(ctx, `
				UPDATE jobs
				SET completed_at = $1, updated_at = $1
				WHERE id = $2
			`, time.Now().Add(-8*24*time.Hour), job.ID)
			require.NoError(t, err)

			// Try to delete old failed jobs (job is completed, not failed, batch size 1000)
			count, err := repo.DeleteOldJobs(ctx, core.DeleteOldJobsParams{
				Status:    model.JobStatusFailed,
				MaxAge:    7 * 24 * time.Hour,
				BatchSize: 1000,
			})
			require.NoError(t, err)
			assert.Equal(t, int64(0), count)

			// Verify job still exists
			_, err = repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
		})
	})

	t.Run("invalid status returns error", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Try to delete jobs with invalid status (batch size 1000)
			_, err := repo.DeleteOldJobs(ctx, core.DeleteOldJobsParams{
				Status:    model.JobStatus("invalid"),
				MaxAge:    7 * 24 * time.Hour,
				BatchSize: 1000,
			})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "invalid job status")
		})
	})
}

package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobRepo_DeleteOldJobResults(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	t.Run("deletes old rows", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			jobRepo := NewJobRepo(db, RepoConfig{})
			resultsRepo := NewJobResultRepo(db)
			ctx := context.Background()

			job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeAlert,
				Payload: json.RawMessage(`{"id":"alert-1"}`),
			})
			require.NoError(t, err)

			err = resultsRepo.Upsert(ctx, core.UpsertJobResultParams{
				JobID:   job.ID,
				JobType: model.JobTypeAlert,
				Result:  json.RawMessage(`{"alert_id":"alert-1"}`),
			})
			require.NoError(t, err)

			oldTime := time.Now().Add(-120 * 24 * time.Hour)
			_, err = db.ExecContext(ctx, `
				UPDATE job_results
				SET updated_at = $1, created_at = $1
				WHERE job_id = $2
			`, oldTime, job.ID)
			require.NoError(t, err)

			count, err := jobRepo.DeleteOldJobResults(ctx, core.DeleteOldJobResultsParams{
				JobType:   model.JobTypeAlert,
				MaxAge:    90 * 24 * time.Hour,
				BatchSize: 1000,
			})
			require.NoError(t, err)
			assert.Equal(t, int64(1), count)

			_, err = resultsRepo.GetByJobID(ctx, job.ID)
			assert.ErrorIs(t, err, ErrJobResultsNotFound)
		})
	})

	t.Run("skips recent rows", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			jobRepo := NewJobRepo(db, RepoConfig{})
			resultsRepo := NewJobResultRepo(db)
			ctx := context.Background()

			job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeAlert,
				Payload: json.RawMessage(`{"id":"alert-2"}`),
			})
			require.NoError(t, err)

			err = resultsRepo.Upsert(ctx, core.UpsertJobResultParams{
				JobID:   job.ID,
				JobType: model.JobTypeAlert,
				Result:  json.RawMessage(`{"alert_id":"alert-2"}`),
			})
			require.NoError(t, err)

			count, err := jobRepo.DeleteOldJobResults(ctx, core.DeleteOldJobResultsParams{
				JobType:   model.JobTypeAlert,
				MaxAge:    90 * 24 * time.Hour,
				BatchSize: 1000,
			})
			require.NoError(t, err)
			assert.Equal(t, int64(0), count)

			result, err := resultsRepo.GetByJobID(ctx, job.ID)
			require.NoError(t, err)
			require.NotNil(t, result.JobID, "JobID should not be nil for recent result")
			assert.Equal(t, job.ID, *result.JobID)
		})
	})

	t.Run("job_results persist after parent job is deleted (orphaned)", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			jobRepo := NewJobRepo(db, RepoConfig{})
			resultsRepo := NewJobResultRepo(db)
			ctx := context.Background()

			// Use unique alert ID to avoid conflicts with leftover test data
			alertID := fmt.Sprintf("alert-orphan-%d", time.Now().UnixNano())

			// Create a job
			job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
				Type:    model.JobTypeAlert,
				Payload: json.RawMessage(fmt.Sprintf(`{"id":"%s"}`, alertID)),
			})
			require.NoError(t, err)

			// Store job result
			err = resultsRepo.Upsert(ctx, core.UpsertJobResultParams{
				JobID:   job.ID,
				JobType: model.JobTypeAlert,
				Result:  json.RawMessage(fmt.Sprintf(`{"alert_id":"%s","status":"delivered"}`, alertID)),
			})
			require.NoError(t, err)
			// Mark job as completed so it can be deleted
			// First update it to running status, then complete it
			_, err = db.ExecContext(ctx, `UPDATE jobs SET status = 'running' WHERE id = $1`, job.ID)
			require.NoError(t, err)

			ok, err := jobRepo.Complete(ctx, job.ID)
			require.NoError(t, err)
			require.True(t, ok)

			// Delete the parent job (simulating reaping)
			err = jobRepo.Delete(ctx, job.ID)
			require.NoError(t, err)

			// Verify job was deleted
			_, err = jobRepo.GetByID(ctx, job.ID)
			require.ErrorIs(t, err, ErrJobNotFound)

			// Verify job_result still exists but with NULL job_id
			var count int
			err = db.QueryRowContext(ctx, `
			SELECT COUNT(*) FROM job_results
			WHERE job_type = $1 AND result->>'alert_id' = $2
		`, model.JobTypeAlert, alertID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count, "job_result should still exist after parent job deletion")

			// Verify job_id is NULL
			var jobID sql.NullString
			err = db.QueryRowContext(ctx, `
			SELECT job_id FROM job_results
			WHERE job_type = $1 AND result->>'alert_id' = $2
		`, model.JobTypeAlert, alertID).Scan(&jobID)
			require.NoError(t, err)
			assert.False(t, jobID.Valid, "job_id should be NULL after parent job deletion")

			// Verify we can still query by alert_id to find orphaned results
			results, err := resultsRepo.ListByAlertID(ctx, alertID)
			require.NoError(t, err)
			require.Len(t, results, 1)
			assert.Nil(t, results[0].JobID, "JobID should be nil for orphaned result")
			assert.Equal(t, model.JobTypeAlert, results[0].JobType)
		})
	})
}

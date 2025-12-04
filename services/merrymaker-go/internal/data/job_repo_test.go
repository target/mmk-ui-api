package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/data/pgxutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func TestJobRepo_Create(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	tests := []struct {
		name    string
		req     *model.CreateJobRequest
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid job creation",
			req: &model.CreateJobRequest{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(`{"url": "https://example.com"}`),
				Priority: 50,
			},
			wantErr: false,
		},
		{
			name: "job with metadata and session",
			req: &model.CreateJobRequest{
				Type:      model.JobTypeRules,
				Payload:   json.RawMessage(`{"rules": ["rule1", "rule2"]}`),
				Metadata:  json.RawMessage(`{"source": "api"}`),
				Priority:  75,
				SessionID: stringPtr("550e8400-e29b-41d4-a716-446655440000"),
			},
			wantErr: false,
		},
		{
			name: "job with scheduled time",
			req: &model.CreateJobRequest{
				Type:        model.JobTypeBrowser,
				Payload:     json.RawMessage(`{"url": "https://scheduled.com"}`),
				Priority:    25,
				ScheduledAt: timePtr(time.Now().Add(time.Hour)),
				MaxRetries:  5,
			},
			wantErr: false,
		},
		{
			name: "invalid job type",
			req: &model.CreateJobRequest{
				Type:    "invalid",
				Payload: json.RawMessage(`{"test": true}`),
			},
			wantErr: true,
			errMsg:  "invalid job type",
		},
		{
			name: "empty payload",
			req: &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(``),
			},
			wantErr: true,
			errMsg:  "payload is required",
		},
		{
			name: "invalid priority",
			req: &model.CreateJobRequest{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(`{"test": true}`),
				Priority: 150,
			},
			wantErr: true,
			errMsg:  "priority must be between 0 and 100",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.WithAutoDB(t, func(db *sql.DB) {
				repo := NewJobRepo(db, RepoConfig{})

				job, err := repo.Create(context.Background(), tt.req)

				if tt.wantErr {
					require.Error(t, err)
					assert.Contains(t, err.Error(), tt.errMsg)
					assert.Nil(t, job)
					return
				}

				require.NoError(t, err)
				require.NotNil(t, job)

				// Verify job fields
				assert.NotEmpty(t, job.ID)
				assert.Equal(t, tt.req.Type, job.Type)
				assert.Equal(t, model.JobStatusPending, job.Status)
				assert.Equal(t, tt.req.Priority, job.Priority)
				assert.Equal(t, tt.req.Payload, job.Payload)
				assert.Equal(t, 0, job.RetryCount)
				assert.NotZero(t, job.CreatedAt)
				assert.NotZero(t, job.UpdatedAt)

				// Verify optional fields
				if tt.req.SessionID != nil {
					assert.Equal(t, tt.req.SessionID, job.SessionID)
				}
				if tt.req.Metadata != nil {
					assert.Equal(t, tt.req.Metadata, job.Metadata)
				} else {
					assert.JSONEq(t, `{}`, string(job.Metadata))
				}
				if tt.req.MaxRetries > 0 {
					assert.Equal(t, tt.req.MaxRetries, job.MaxRetries)
				} else {
					assert.Equal(t, 3, job.MaxRetries) // default
				}
			})
		})
	}
}

func TestJobRepo_ReserveNext(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	tests := []struct {
		name         string
		jobType      model.JobType
		leaseSeconds int
		setupJobs    []*model.CreateJobRequest
		wantJob      bool
		wantErr      bool
	}{
		{
			name:         "reserve available job",
			jobType:      model.JobTypeBrowser,
			leaseSeconds: 30,
			setupJobs: []*model.CreateJobRequest{
				{
					Type:     model.JobTypeBrowser,
					Payload:  json.RawMessage(`{"url": "https://example.com"}`),
					Priority: 50,
				},
			},
			wantJob: true,
			wantErr: false,
		},
		{
			name:         "no jobs available",
			jobType:      model.JobTypeBrowser,
			leaseSeconds: 30,
			setupJobs:    []*model.CreateJobRequest{},
			wantJob:      false,
			wantErr:      true,
		},
		{
			name:         "reserve highest priority job",
			jobType:      model.JobTypeBrowser,
			leaseSeconds: 30,
			setupJobs: []*model.CreateJobRequest{
				{
					Type:     model.JobTypeBrowser,
					Payload:  json.RawMessage(`{"priority": "low"}`),
					Priority: 25,
				},
				{
					Type:     model.JobTypeBrowser,
					Payload:  json.RawMessage(`{"priority": "high"}`),
					Priority: 75,
				},
			},
			wantJob: true,
			wantErr: false,
		},
		{
			name:         "invalid job type",
			jobType:      "invalid",
			leaseSeconds: 30,
			setupJobs:    []*model.CreateJobRequest{},
			wantJob:      false,
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.WithAutoDB(t, func(db *sql.DB) {
				repo := NewJobRepo(db, RepoConfig{})

				// Setup test jobs
				var createdJobs []*model.Job
				for _, req := range tt.setupJobs {
					job, err := repo.Create(context.Background(), req)
					require.NoError(t, err)
					createdJobs = append(createdJobs, job)
				}

				// Test ReserveNext
				job, err := repo.ReserveNext(context.Background(), tt.jobType, tt.leaseSeconds)

				if tt.wantErr {
					require.Error(t, err)
					if !tt.wantJob && tt.name != "invalid job type" {
						require.ErrorIs(t, err, model.ErrNoJobsAvailable)
					}
					return
				}

				require.NoError(t, err)
				require.NotNil(t, job)

				// Verify job was reserved
				assert.Equal(t, model.JobStatusRunning, job.Status)
				assert.NotNil(t, job.StartedAt)
				assert.NotNil(t, job.LeaseExpiresAt)

				// Verify lease duration
				expectedLease := time.Duration(tt.leaseSeconds) * time.Second
				actualLease := job.LeaseExpiresAt.Sub(*job.StartedAt)
				assert.InDelta(t, expectedLease.Seconds(), actualLease.Seconds(), 1.0)

				// If multiple jobs, verify highest priority was selected
				if len(createdJobs) > 1 {
					maxPriority := 0
					for _, created := range createdJobs {
						if created.Priority > maxPriority {
							maxPriority = created.Priority
						}
					}
					assert.Equal(t, maxPriority, job.Priority)
				}
			})
		})
	}
}

func TestJobRepo_Complete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Create and reserve a job
		req := &model.CreateJobRequest{
			Type:    model.JobTypeBrowser,
			Payload: json.RawMessage(`{"url": "https://example.com"}`),
		}
		job, err := repo.Create(context.Background(), req)
		require.NoError(t, err)

		_, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)

		// Test completing the job
		success, err := repo.Complete(context.Background(), job.ID)
		require.NoError(t, err)
		assert.True(t, success)

		// Test completing non-existent job (use valid UUID format)
		success, err = repo.Complete(context.Background(), "00000000-0000-0000-0000-000000000000")
		require.NoError(t, err)
		assert.False(t, success)
	})
}

func TestJobRepo_Fail(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithTestDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{RetryDelaySeconds: 10})

		// Create and reserve a job
		req := &model.CreateJobRequest{
			Type:       model.JobTypeBrowser,
			Payload:    json.RawMessage(`{"url": "https://example.com"}`),
			MaxRetries: 2,
		}
		job, err := repo.Create(context.Background(), req)
		require.NoError(t, err)

		_, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)

		// Test failing the job (should retry)
		success, err := repo.Fail(context.Background(), job.ID, "test error")
		require.NoError(t, err)
		assert.True(t, success)

		// Test failing non-existent job (use valid UUID format)
		success, err = repo.Fail(context.Background(), "00000000-0000-0000-0000-000000000000", "error")
		require.NoError(t, err)
		assert.False(t, success)
	})
}

func TestJobRepo_Heartbeat(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	tests := []struct {
		name         string
		setupJob     bool
		reserveJob   bool
		jobID        string
		leaseSeconds int
		wantSuccess  bool
	}{
		{
			name:         "successful heartbeat",
			setupJob:     true,
			reserveJob:   true,
			leaseSeconds: 60,
			wantSuccess:  true,
		},
		{
			name:         "heartbeat non-existent job",
			setupJob:     false,
			reserveJob:   false,
			jobID:        "00000000-0000-0000-0000-000000000000",
			leaseSeconds: 60,
			wantSuccess:  false,
		},
		{
			name:         "heartbeat pending job",
			setupJob:     true,
			reserveJob:   false,
			leaseSeconds: 60,
			wantSuccess:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testutil.WithAutoDB(t, func(db *sql.DB) {
				repo := NewJobRepo(db, RepoConfig{})
				jobID := tt.jobID

				if tt.setupJob {
					req := &model.CreateJobRequest{
						Type:    model.JobTypeBrowser,
						Payload: json.RawMessage(`{"url": "https://example.com"}`),
					}
					job, err := repo.Create(context.Background(), req)
					require.NoError(t, err)
					jobID = job.ID

					if tt.reserveJob {
						_, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
						require.NoError(t, err)
					}
				}

				success, err := repo.Heartbeat(context.Background(), jobID, tt.leaseSeconds)
				require.NoError(t, err)
				assert.Equal(t, tt.wantSuccess, success)
			})
		})
	}
}

func TestJobRepo_Stats(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})

		// Create jobs with different priorities to control reservation order
		// ReserveNext picks jobs by priority (DESC), so we set priorities to control which job gets reserved first
		jobs := []struct {
			req    *model.CreateJobRequest
			action string
		}{
			{
				req: &model.CreateJobRequest{
					Type:     model.JobTypeBrowser,
					Payload:  json.RawMessage(`{"url": "https://pending.com"}`),
					Priority: 10, // Lowest priority - will be reserved last
				},
				action: "none", // stays pending
			},
			{
				req: &model.CreateJobRequest{
					Type:     model.JobTypeBrowser,
					Payload:  json.RawMessage(`{"url": "https://running.com"}`),
					Priority: 40, // Second highest - will be reserved second
				},
				action: "reserve",
			},
			{
				req: &model.CreateJobRequest{
					Type:     model.JobTypeBrowser,
					Payload:  json.RawMessage(`{"url": "https://completed.com"}`),
					Priority: 50, // Highest priority - will be reserved first
				},
				action: "complete",
			},
			{
				req: &model.CreateJobRequest{
					Type:       model.JobTypeBrowser,
					Payload:    json.RawMessage(`{"url": "https://failed.com"}`),
					Priority:   30, // Third highest - will be reserved third
					MaxRetries: 1,
				},
				action: "fail",
			},
		}

		// Create all jobs first
		var createdJobs []*model.Job
		for _, jobSetup := range jobs {
			job, err := repo.Create(context.Background(), jobSetup.req)
			require.NoError(t, err)
			createdJobs = append(createdJobs, job)
		}

		// Process jobs in the order they will be reserved (by priority: highest first)
		// Priority order: complete(50) -> reserve(40) -> fail(30) -> none(10)

		// 1. Complete job (priority 50) - will be reserved first
		reserved, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		require.Equal(
			t,
			createdJobs[2].ID,
			reserved.ID,
			"Expected to reserve the complete job first (highest priority)",
		)
		_, err = repo.Complete(context.Background(), reserved.ID)
		require.NoError(t, err)

		// 2. Reserve job (priority 40) - will be reserved second
		reserved, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		require.Equal(t, createdJobs[1].ID, reserved.ID, "Expected to reserve the reserve job second")
		// Leave this job in running state

		// 3. Fail job (priority 30) - will be reserved third
		reserved, err = repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		require.Equal(t, createdJobs[3].ID, reserved.ID, "Expected to reserve the fail job third")
		// With MaxRetries=1, the first failure should immediately mark it as failed
		_, err = repo.Fail(context.Background(), reserved.ID, "failure that exceeds max retries")
		require.NoError(t, err)

		// 4. Pending job (priority 10) - leave it pending (don't reserve it)

		// Get stats
		stats, err := repo.Stats(context.Background(), model.JobTypeBrowser)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 1, stats.Pending)
		assert.Equal(t, 1, stats.Running)
		assert.Equal(t, 1, stats.Completed)
		assert.Equal(t, 1, stats.Failed)
	})
}

func TestJobRepo_RequeueExpired(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Use a fixed time for testing
		fixedTime := testutil.TestTime()
		timeProvider := NewFixedTimeProvider(fixedTime)
		repo := NewJobRepo(db, RepoConfig{TimeProvider: timeProvider})

		// Create a job
		req := &model.CreateJobRequest{
			Type:    model.JobTypeBrowser,
			Payload: json.RawMessage(`{"url": "https://example.com"}`),
		}
		job, err := repo.Create(context.Background(), req)
		require.NoError(t, err)

		// Reserve it with a short lease
		reserved, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 1)
		require.NoError(t, err)
		assert.Equal(t, job.ID, reserved.ID)

		// Simulate time passing beyond lease expiration
		timeProvider.AddTime(2 * time.Second)

		// Requeue expired jobs
		count, err := repo.requeueExpired(context.Background(), model.JobTypeBrowser)
		require.NoError(t, err)
		assert.Equal(t, int64(1), count)

		// Verify job is back to pending
		requeued, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		assert.Equal(t, job.ID, requeued.ID)
		assert.Equal(t, model.JobStatusRunning, requeued.Status)
	})
}

// TestPgxConversionFunctions tests the pgx transaction option conversion utilities.
func TestPgxConversionFunctions(t *testing.T) {
	t.Run("toPgxTxOptions", func(t *testing.T) {
		tests := []struct {
			name     string
			input    *sql.TxOptions
			expected pgx.TxOptions
		}{
			{
				name:  "nil options",
				input: nil,
				expected: pgx.TxOptions{
					IsoLevel:   pgx.TxIsoLevel(""),
					AccessMode: pgx.TxAccessMode(""),
				},
			},
			{
				name: "read committed, read-write",
				input: &sql.TxOptions{
					Isolation: sql.LevelReadCommitted,
					ReadOnly:  false,
				},
				expected: pgx.TxOptions{
					IsoLevel:   pgx.ReadCommitted,
					AccessMode: pgx.ReadWrite,
				},
			},
			{
				name: "serializable, read-only",
				input: &sql.TxOptions{
					Isolation: sql.LevelSerializable,
					ReadOnly:  true,
				},
				expected: pgx.TxOptions{
					IsoLevel:   pgx.Serializable,
					AccessMode: pgx.ReadOnly,
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				result := pgxutil.ToPgxTxOptions(tt.input)
				assert.Equal(t, tt.expected.IsoLevel, result.IsoLevel)
				assert.Equal(t, tt.expected.AccessMode, result.AccessMode)
			})
		}
	})

	t.Run("toPgxIsoLevel", func(t *testing.T) {
		tests := []struct {
			input    sql.IsolationLevel
			expected pgx.TxIsoLevel
		}{
			{sql.LevelDefault, pgx.TxIsoLevel("")},
			{sql.LevelSerializable, pgx.Serializable},
			{sql.LevelLinearizable, pgx.Serializable},
			{sql.LevelRepeatableRead, pgx.RepeatableRead},
			{sql.LevelSnapshot, pgx.RepeatableRead},
			{sql.LevelReadCommitted, pgx.ReadCommitted},
			{sql.LevelWriteCommitted, pgx.ReadCommitted},
			{sql.LevelReadUncommitted, pgx.ReadUncommitted},
		}

		for _, tt := range tests {
			t.Run(string(tt.expected), func(t *testing.T) {
				result := pgxutil.ToPgxIsoLevel(tt.input)
				assert.Equal(t, tt.expected, result)
			})
		}
	})

	t.Run("toPgxAccessMode", func(t *testing.T) {
		assert.Equal(t, pgx.ReadWrite, pgxutil.ToPgxAccessMode(false))
		assert.Equal(t, pgx.ReadOnly, pgxutil.ToPgxAccessMode(true))
	})
}

func TestJobRepo_List(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := NewJobRepo(db, RepoConfig{})
		ctx := context.Background()

		// Create test jobs with different types and statuses
		browserJob, err := repo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url": "https://example.com"}`),
			Priority: 50,
			IsTest:   false,
		})
		require.NoError(t, err)

		rulesJob, err := repo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeRules,
			Payload:  json.RawMessage(`{"rules": ["rule1"]}`),
			Priority: 75,
			IsTest:   true,
		})
		require.NoError(t, err)

		alertJob, err := repo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeAlert,
			Payload:  json.RawMessage(`{"alert": "test"}`),
			Priority: 25,
			IsTest:   false,
		})
		require.NoError(t, err)

		// Reserve and complete one job to test status filtering
		_, err = repo.ReserveNext(ctx, model.JobTypeAlert, 30)
		require.NoError(t, err)

		success, err := repo.Complete(ctx, alertJob.ID)
		require.NoError(t, err)
		require.True(t, success, "job should be successfully completed")

		// Verify the job is actually completed
		completedJob, err := repo.GetByID(ctx, alertJob.ID)
		require.NoError(t, err)
		require.Equal(t, model.JobStatusCompleted, completedJob.Status)

		tests := []struct {
			name     string
			opts     *model.JobListOptions
			wantLen  int
			checkJob func(t *testing.T, jobs []*model.JobWithEventCount)
		}{
			{
				name: "list all jobs",
				opts: &model.JobListOptions{
					Limit: 10,
				},
				wantLen: 3,
				checkJob: func(t *testing.T, jobs []*model.JobWithEventCount) {
					// Should be ordered by created_at DESC
					assert.Equal(t, alertJob.ID, jobs[0].ID)
					assert.Equal(t, rulesJob.ID, jobs[1].ID)
					assert.Equal(t, browserJob.ID, jobs[2].ID)
				},
			},
			{
				name: "filter by type",
				opts: &model.JobListOptions{
					Type:  jobTypePtr(model.JobTypeBrowser),
					Limit: 10,
				},
				wantLen: 1,
				checkJob: func(t *testing.T, jobs []*model.JobWithEventCount) {
					assert.Equal(t, browserJob.ID, jobs[0].ID)
					assert.Equal(t, model.JobTypeBrowser, jobs[0].Type)
				},
			},
			{
				name: "filter by status",
				opts: &model.JobListOptions{
					Status: jobStatusPtr(model.JobStatusCompleted),
					Limit:  10,
				},
				wantLen: 1,
				checkJob: func(t *testing.T, jobs []*model.JobWithEventCount) {
					assert.Equal(t, alertJob.ID, jobs[0].ID)
					assert.Equal(t, model.JobStatusCompleted, jobs[0].Status)
				},
			},
			{
				name: "filter by is_test",
				opts: &model.JobListOptions{
					IsTest: jobBoolPtr(true),
					Limit:  10,
				},
				wantLen: 1,
				checkJob: func(t *testing.T, jobs []*model.JobWithEventCount) {
					assert.Equal(t, rulesJob.ID, jobs[0].ID)
					assert.True(t, jobs[0].IsTest)
				},
			},
			{
				name: "sort by type ascending",
				opts: &model.JobListOptions{
					SortBy:    "type",
					SortOrder: "asc",
					Limit:     10,
				},
				wantLen: 3,
				checkJob: func(t *testing.T, jobs []*model.JobWithEventCount) {
					// Should be ordered by type ASC: alert, browser, rules
					assert.Equal(t, model.JobTypeAlert, jobs[0].Type)
					assert.Equal(t, model.JobTypeBrowser, jobs[1].Type)
					assert.Equal(t, model.JobTypeRules, jobs[2].Type)
				},
			},
			{
				name: "pagination with limit",
				opts: &model.JobListOptions{
					Limit: 2,
				},
				wantLen: 2,
				checkJob: func(t *testing.T, jobs []*model.JobWithEventCount) {
					// Should get first 2 jobs ordered by created_at DESC
					assert.Equal(t, alertJob.ID, jobs[0].ID)
					assert.Equal(t, rulesJob.ID, jobs[1].ID)
				},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				jobs, err := repo.List(ctx, tt.opts)
				require.NoError(t, err)
				assert.Len(t, jobs, tt.wantLen)

				if tt.checkJob != nil {
					tt.checkJob(t, jobs)
				}

				// Verify all jobs have event counts (should be 0 for new jobs)
				for _, job := range jobs {
					assert.GreaterOrEqual(t, job.EventCount, 0)
				}
			})
		}
	})
}

func TestJobRepo_Delete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	t.Run("delete pending job without lease", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a pending job
			req := &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			}
			job, err := repo.Create(ctx, req)
			require.NoError(t, err)
			require.Equal(t, model.JobStatusPending, job.Status)
			require.Nil(t, job.LeaseExpiresAt)

			// Delete should succeed
			err = repo.Delete(ctx, job.ID)
			require.NoError(t, err)

			// Verify job is deleted
			_, err = repo.GetByID(ctx, job.ID)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrJobNotFound)
		})
	})

	t.Run("delete non-existent job", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Try to delete a non-existent job
			err := repo.Delete(ctx, "00000000-0000-0000-0000-000000000000")
			require.Error(t, err)
			require.ErrorIs(t, err, ErrJobNotFound)
		})
	})

	t.Run("delete running job", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create and reserve a job (makes it running)
			req := &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			}
			job, err := repo.Create(ctx, req)
			require.NoError(t, err)

			// Reserve the job (transitions to running)
			_, err = repo.ReserveNext(ctx, model.JobTypeBrowser, 30)
			require.NoError(t, err)

			// Verify job is running
			runningJob, err := repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
			require.Equal(t, model.JobStatusRunning, runningJob.Status)

			// Delete should fail
			err = repo.Delete(ctx, job.ID)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrJobNotDeletable)

			// Verify job still exists
			_, err = repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
		})
	})

	t.Run("delete completed job", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create, reserve, and complete a job
			req := &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			}
			job, err := repo.Create(ctx, req)
			require.NoError(t, err)

			// Reserve and complete the job
			_, err = repo.ReserveNext(ctx, model.JobTypeBrowser, 30)
			require.NoError(t, err)
			_, err = repo.Complete(ctx, job.ID)
			require.NoError(t, err)

			// Verify job is completed
			completedJob, err := repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
			require.Equal(t, model.JobStatusCompleted, completedJob.Status)

			// Delete should succeed for completed jobs
			err = repo.Delete(ctx, job.ID)
			require.NoError(t, err)

			// Verify job was deleted
			_, err = repo.GetByID(ctx, job.ID)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrJobNotFound)
		})
	})

	t.Run("delete failed job", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a job with 1 max retry (allows 1 attempt, fails immediately on first failure)
			req := &model.CreateJobRequest{
				Type:       model.JobTypeBrowser,
				Payload:    json.RawMessage(`{"url": "https://example.com"}`),
				MaxRetries: 1,
			}
			job, err := repo.Create(ctx, req)
			require.NoError(t, err)

			// Reserve and fail the job (will mark as failed since retry_count+1 >= max_retries)
			_, err = repo.ReserveNext(ctx, model.JobTypeBrowser, 30)
			require.NoError(t, err)
			_, err = repo.Fail(ctx, job.ID, "test error")
			require.NoError(t, err)

			// Verify job is failed
			failedJob, err := repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
			require.Equal(t, model.JobStatusFailed, failedJob.Status)

			// Delete should succeed for failed jobs
			err = repo.Delete(ctx, job.ID)
			require.NoError(t, err)

			// Verify job was deleted
			_, err = repo.GetByID(ctx, job.ID)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrJobNotFound)
		})
	})

	t.Run("delete pending job with active lease", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a pending job
			req := &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			}
			job, err := repo.Create(ctx, req)
			require.NoError(t, err)

			// Manually set a lease on the pending job to simulate race condition
			// This simulates the job being reserved between check and delete
			_, err = db.ExecContext(ctx, `
				UPDATE jobs
				SET lease_expires_at = NOW() + INTERVAL '30 seconds'
				WHERE id = $1
			`, job.ID)
			require.NoError(t, err)

			// Verify job has lease
			jobWithLease, err := repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
			require.NotNil(t, jobWithLease.LeaseExpiresAt)

			// Delete should fail
			err = repo.Delete(ctx, job.ID)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrJobReserved)

			// Verify job still exists
			_, err = repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
		})
	})

	t.Run("delete pending job with expired lease", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a pending job
			req := &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			}
			job, err := repo.Create(ctx, req)
			require.NoError(t, err)

			// Manually set an expired lease on the pending job
			expiredTime := time.Now().Add(-1 * time.Hour)
			_, err = db.ExecContext(ctx, `
				UPDATE jobs
				SET lease_expires_at = $2
				WHERE id = $1
			`, job.ID, expiredTime)
			require.NoError(t, err)

			// Verify job has expired lease
			jobWithExpiredLease, err := repo.GetByID(ctx, job.ID)
			require.NoError(t, err)
			require.NotNil(t, jobWithExpiredLease.LeaseExpiresAt)
			require.True(t, jobWithExpiredLease.LeaseExpiresAt.Before(time.Now()))

			// Delete should succeed (expired lease is allowed)
			err = repo.Delete(ctx, job.ID)
			require.NoError(t, err)

			// Verify job is deleted
			_, err = repo.GetByID(ctx, job.ID)
			require.Error(t, err)
			require.ErrorIs(t, err, ErrJobNotFound)
		})
	})

	t.Run("delete job with events - FK cascade", func(t *testing.T) {
		testutil.WithAutoDB(t, func(db *sql.DB) {
			repo := NewJobRepo(db, RepoConfig{})
			ctx := context.Background()

			// Create a pending job
			req := &model.CreateJobRequest{
				Type:    model.JobTypeBrowser,
				Payload: json.RawMessage(`{"url": "https://example.com"}`),
			}
			job, err := repo.Create(ctx, req)
			require.NoError(t, err)

			// Create an event associated with this job
			var eventID string
			err = db.QueryRowContext(ctx, `
				INSERT INTO events (session_id, source_job_id, event_type, event_data)
				VALUES ($1, $2, $3, $4)
				RETURNING id
			`, "550e8400-e29b-41d4-a716-446655440000", job.ID, "test_event", json.RawMessage(`{"test": true}`)).Scan(&eventID)
			require.NoError(t, err)

			// Verify event has source_job_id set
			var sourceJobID *string
			err = db.QueryRowContext(ctx, `
				SELECT source_job_id FROM events WHERE id = $1
			`, eventID).Scan(&sourceJobID)
			require.NoError(t, err)
			require.NotNil(t, sourceJobID)
			require.Equal(t, job.ID, *sourceJobID)

			// Delete the job
			err = repo.Delete(ctx, job.ID)
			require.NoError(t, err)

			// Verify event still exists but source_job_id is NULL (FK cascade)
			err = db.QueryRowContext(ctx, `
				SELECT source_job_id FROM events WHERE id = $1
			`, eventID).Scan(&sourceJobID)
			require.NoError(t, err)
			require.Nil(t, sourceJobID, "source_job_id should be NULL after job deletion")
		})
	})
}

// Helper functions.
func stringPtr(s string) *string {
	return &s
}

func timePtr(t time.Time) *time.Time {
	return &t
}

func jobTypePtr(jt model.JobType) *model.JobType {
	return &jt
}

func jobStatusPtr(js model.JobStatus) *model.JobStatus {
	return &js
}

func jobBoolPtr(b bool) *bool {
	return &b
}

func TestJobRepo_ListRecentByTypeWithSiteNames(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()
		jobRepo := NewJobRepo(db, RepoConfig{})
		siteRepo := NewSiteRepo(db)
		sourceRepo := NewSourceRepo(db)

		// Create test source
		source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
			Name:  "Test Source",
			Value: "console.log('test');",
		})
		require.NoError(t, err)

		// Create test sites
		site1, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
			Name:            "Test Site 1",
			RunEveryMinutes: 60,
			SourceID:        source.ID,
		})
		require.NoError(t, err)

		site2, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
			Name:            "Test Site 2",
			RunEveryMinutes: 60,
			SourceID:        source.ID,
		})
		require.NoError(t, err)

		// Create jobs with different types and sites
		job1, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url": "https://site1.example.com"}`),
			SiteID:   &site1.ID,
			Priority: 50,
		})
		require.NoError(t, err)

		job2, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url": "https://site2.example.com"}`),
			SiteID:   &site2.ID,
			Priority: 50,
		})
		require.NoError(t, err)

		// Create a job without a site
		job3, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url": "https://nositetest.example.com"}`),
			Priority: 50,
		})
		require.NoError(t, err)

		// Create a test job (should be excluded from results)
		_, err = jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"url": "https://testjob.example.com"}`),
			SiteID:   &site1.ID,
			IsTest:   true,
			Priority: 50,
		})
		require.NoError(t, err)

		// Create a job with a different type (should not be returned)
		_, err = jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeRules,
			Payload:  json.RawMessage(`{"rules": ["rule1"]}`),
			SiteID:   &site1.ID,
			Priority: 50,
		})
		require.NoError(t, err)

		// Test: List recent browser jobs with site names (excludes test jobs)
		jobs, err := jobRepo.ListRecentByTypeWithSiteNames(ctx, model.JobTypeBrowser, 10)
		require.NoError(t, err)
		require.Len(t, jobs, 3, "should return 3 non-test browser jobs")

		// Verify jobs are ordered by created_at DESC
		assert.Equal(t, job3.ID, jobs[0].ID, "most recent job should be first")
		assert.Equal(t, job2.ID, jobs[1].ID)
		assert.Equal(t, job1.ID, jobs[2].ID)

		// Verify site names are populated correctly
		assert.Empty(t, jobs[0].SiteName, "job without site should have empty site name")
		assert.Equal(t, site2.Name, jobs[1].SiteName)
		assert.Equal(t, site1.Name, jobs[2].SiteName)

		// Verify event count is 0 (no events created)
		assert.Equal(t, 0, jobs[0].EventCount)
		assert.Equal(t, 0, jobs[1].EventCount)
		assert.Equal(t, 0, jobs[2].EventCount)

		// Verify test jobs are excluded
		for _, job := range jobs {
			assert.False(t, job.IsTest, "test jobs should be excluded")
		}

		// Test: Limit works correctly
		limitedJobs, err := jobRepo.ListRecentByTypeWithSiteNames(ctx, model.JobTypeBrowser, 2)
		require.NoError(t, err)
		require.Len(t, limitedJobs, 2, "should respect limit parameter")

		// Test: Different job type returns only matching jobs (also excludes test jobs)
		rulesJobs, err := jobRepo.ListRecentByTypeWithSiteNames(ctx, model.JobTypeRules, 10)
		require.NoError(t, err)
		require.Len(t, rulesJobs, 1, "should return 1 non-test rules job")
		assert.Equal(t, site1.Name, rulesJobs[0].SiteName)
		assert.False(t, rulesJobs[0].IsTest)
	})
}

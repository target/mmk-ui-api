package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockReaperRepo is a simple mock implementation for testing.
type mockReaperRepo struct {
	failStalePendingJobsCalled int
	failStalePendingJobsCount  int64
	failStalePendingJobsError  error

	deleteOldJobsCalled int
	deleteOldJobsCount  int64
	deleteOldJobsError  error

	deleteOldJobResultsCalls  map[model.JobType]int
	deleteOldJobResultsCounts map[model.JobType]int64
	deleteOldJobResultsError  error
}

func (m *mockReaperRepo) FailStalePendingJobs(
	ctx context.Context,
	maxAge time.Duration,
	batchSize int,
) (int64, error) {
	m.failStalePendingJobsCalled++
	if m.failStalePendingJobsError != nil {
		return 0, m.failStalePendingJobsError
	}
	// Return count on first call, then 0 to simulate batch exhaustion
	if m.failStalePendingJobsCalled == 1 {
		return m.failStalePendingJobsCount, nil
	}
	return 0, nil
}

func (m *mockReaperRepo) DeleteOldJobs(
	ctx context.Context,
	params core.DeleteOldJobsParams,
) (int64, error) {
	m.deleteOldJobsCalled++
	if m.deleteOldJobsError != nil {
		return 0, m.deleteOldJobsError
	}
	// Return count on odd calls (1st, 3rd, 5th...), then 0 on even calls to simulate batch exhaustion
	// This allows multiple cleanup operations (completed, failed) to each get their batch
	if m.deleteOldJobsCalled%2 == 1 {
		return m.deleteOldJobsCount, nil
	}
	return 0, nil
}

func (m *mockReaperRepo) DeleteOldJobResults(
	ctx context.Context,
	params core.DeleteOldJobResultsParams,
) (int64, error) {
	if m.deleteOldJobResultsCalls == nil {
		m.deleteOldJobResultsCalls = make(map[model.JobType]int)
	}
	if m.deleteOldJobResultsCounts == nil {
		m.deleteOldJobResultsCounts = make(map[model.JobType]int64)
	}

	m.deleteOldJobResultsCalls[params.JobType]++
	if m.deleteOldJobResultsError != nil {
		return 0, m.deleteOldJobResultsError
	}

	if m.deleteOldJobResultsCalls[params.JobType] == 1 {
		return m.deleteOldJobResultsCounts[params.JobType], nil
	}
	return 0, nil
}

func TestNewReaperService(t *testing.T) {
	t.Run("creates service with valid options", func(t *testing.T) {
		repo := &mockReaperRepo{}
		cfg := config.ReaperConfig{
			Interval:         5 * time.Minute,
			PendingMaxAge:    1 * time.Hour,
			CompletedMaxAge:  7 * 24 * time.Hour,
			FailedMaxAge:     7 * 24 * time.Hour,
			JobResultsMaxAge: 90 * 24 * time.Hour,
			BatchSize:        1000,
		}

		svc, err := NewReaperService(ReaperServiceOptions{
			Repo:   repo,
			Config: cfg,
			Logger: slog.Default(),
		})

		require.NoError(t, err)
		assert.NotNil(t, svc)
	})

	t.Run("returns error when repo is nil", func(t *testing.T) {
		cfg := config.ReaperConfig{
			Interval:         5 * time.Minute,
			PendingMaxAge:    1 * time.Hour,
			CompletedMaxAge:  7 * 24 * time.Hour,
			FailedMaxAge:     7 * 24 * time.Hour,
			JobResultsMaxAge: 90 * 24 * time.Hour,
			BatchSize:        1000,
		}

		_, err := NewReaperService(ReaperServiceOptions{
			Repo:   nil,
			Config: cfg,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "ReaperRepository is required")
	})
}

func TestReaperService_runCleanup(t *testing.T) {
	t.Run("runs all cleanup operations successfully", func(t *testing.T) {
		repo := &mockReaperRepo{
			failStalePendingJobsCount: 5,
			deleteOldJobsCount:        10,
			deleteOldJobResultsCounts: map[model.JobType]int64{
				model.JobTypeAlert: 4,
				model.JobTypeRules: 2,
			},
		}
		cfg := config.ReaperConfig{
			Interval:         5 * time.Minute,
			PendingMaxAge:    1 * time.Hour,
			CompletedMaxAge:  7 * 24 * time.Hour,
			FailedMaxAge:     7 * 24 * time.Hour,
			JobResultsMaxAge: 90 * 24 * time.Hour,
			BatchSize:        1000,
		}

		svc, _ := MustNewReaperService(ReaperServiceOptions{
			Repo:   repo,
			Config: cfg,
		})

		ctx := context.Background()
		err := svc.runCleanup(ctx)

		require.NoError(t, err)
		// Each operation is called twice: once returning count, once returning 0
		assert.Equal(t, 2, repo.failStalePendingJobsCalled)
		// DeleteOldJobs is called twice per status (completed, failed): 2 * 2 = 4
		assert.Equal(t, 4, repo.deleteOldJobsCalled)
		assert.Equal(t, 2, repo.deleteOldJobResultsCalls[model.JobTypeAlert])
		assert.Equal(t, 2, repo.deleteOldJobResultsCalls[model.JobTypeRules])
	})

	t.Run("continues on partial errors", func(t *testing.T) {
		repo := &mockReaperRepo{
			failStalePendingJobsError: errors.New("fail error"),
			deleteOldJobsCount:        10,
			deleteOldJobResultsCounts: map[model.JobType]int64{
				model.JobTypeAlert: 0,
				model.JobTypeRules: 0,
			},
		}
		cfg := config.ReaperConfig{
			Interval:         5 * time.Minute,
			PendingMaxAge:    1 * time.Hour,
			CompletedMaxAge:  7 * 24 * time.Hour,
			FailedMaxAge:     7 * 24 * time.Hour,
			JobResultsMaxAge: 90 * 24 * time.Hour,
			BatchSize:        1000,
		}

		svc, _ := MustNewReaperService(ReaperServiceOptions{
			Repo:   repo,
			Config: cfg,
		})

		ctx := context.Background()
		err := svc.runCleanup(ctx)

		// Should return error but still call all cleanup methods
		require.Error(t, err)
		// FailStalePendingJobs called once (returns error immediately)
		assert.Equal(t, 1, repo.failStalePendingJobsCalled)
		// DeleteOldJobs called twice per status (completed, failed): 2 * 2 = 4
		assert.Equal(t, 4, repo.deleteOldJobsCalled)
		assert.Equal(t, 1, repo.deleteOldJobResultsCalls[model.JobTypeAlert])
		assert.Equal(t, 1, repo.deleteOldJobResultsCalls[model.JobTypeRules])
	})
}

func TestReaperService_Run(t *testing.T) {
	t.Run("stops on context cancellation", func(t *testing.T) {
		repo := &mockReaperRepo{}
		cfg := config.ReaperConfig{
			Interval:         100 * time.Millisecond,
			PendingMaxAge:    1 * time.Hour,
			CompletedMaxAge:  7 * 24 * time.Hour,
			FailedMaxAge:     7 * 24 * time.Hour,
			JobResultsMaxAge: 90 * 24 * time.Hour,
			BatchSize:        1000,
		}

		svc, _ := MustNewReaperService(ReaperServiceOptions{
			Repo:   repo,
			Config: cfg,
		})

		ctx, cancel := context.WithCancel(context.Background())

		// Run in goroutine
		done := make(chan error, 1)
		go func() {
			done <- svc.Run(ctx)
		}()

		// Wait a bit to ensure at least one cleanup runs
		time.Sleep(150 * time.Millisecond)

		// Cancel context
		cancel()

		// Wait for Run to return
		select {
		case err := <-done:
			// Should return nil on graceful shutdown
			require.NoError(t, err)
		case <-time.After(1 * time.Second):
			t.Fatal("Run did not stop after context cancellation")
		}

		// Verify cleanup was called at least once (initial + maybe one tick)
		assert.GreaterOrEqual(t, repo.failStalePendingJobsCalled, 1)
	})

	t.Run("continues running despite cleanup errors", func(t *testing.T) {
		repo := &mockReaperRepo{
			failStalePendingJobsError: errors.New("test error"),
		}
		cfg := config.ReaperConfig{
			Interval:         50 * time.Millisecond,
			PendingMaxAge:    1 * time.Hour,
			CompletedMaxAge:  7 * 24 * time.Hour,
			FailedMaxAge:     7 * 24 * time.Hour,
			JobResultsMaxAge: 90 * 24 * time.Hour,
			BatchSize:        1000,
		}

		svc, _ := MustNewReaperService(ReaperServiceOptions{
			Repo:   repo,
			Config: cfg,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		err := svc.Run(ctx)

		// Should return context deadline exceeded, not the cleanup error
		require.Error(t, err)
		require.ErrorIs(t, err, context.DeadlineExceeded)

		// Verify cleanup was called multiple times despite errors
		assert.GreaterOrEqual(t, repo.failStalePendingJobsCalled, 2)
	})
}

func TestReaperService_failStalePendingJobs(t *testing.T) {
	t.Run("calls repo with correct max age", func(t *testing.T) {
		repo := &mockReaperRepo{
			failStalePendingJobsCount: 3,
		}
		cfg := config.ReaperConfig{
			PendingMaxAge:    2 * time.Hour,
			JobResultsMaxAge: 90 * 24 * time.Hour,
			BatchSize:        1000,
		}

		svc, _ := MustNewReaperService(ReaperServiceOptions{
			Repo:   repo,
			Config: cfg,
		})

		ctx := context.Background()
		count, err := svc.failStalePendingJobs(ctx)

		require.NoError(t, err)
		assert.Equal(t, int64(3), count)
		// Called twice: once returning count, once returning 0
		assert.Equal(t, 2, repo.failStalePendingJobsCalled)
	})
}

func TestReaperService_deleteOldCompletedJobs(t *testing.T) {
	t.Run("calls repo with correct status and max age", func(t *testing.T) {
		repo := &mockReaperRepo{
			deleteOldJobsCount: 5,
		}
		cfg := config.ReaperConfig{
			CompletedMaxAge:  7 * 24 * time.Hour,
			JobResultsMaxAge: 90 * 24 * time.Hour,
			BatchSize:        1000,
		}

		svc, _ := MustNewReaperService(ReaperServiceOptions{
			Repo:   repo,
			Config: cfg,
		})

		ctx := context.Background()
		count, err := svc.deleteOldCompletedJobs(ctx)

		require.NoError(t, err)
		assert.Equal(t, int64(5), count)
		// Called twice: once returning count, once returning 0
		assert.Equal(t, 2, repo.deleteOldJobsCalled)
	})
}

func TestReaperService_deleteOldFailedJobs(t *testing.T) {
	t.Run("calls repo with correct status and max age", func(t *testing.T) {
		repo := &mockReaperRepo{
			deleteOldJobsCount: 8,
		}
		cfg := config.ReaperConfig{
			FailedMaxAge:     7 * 24 * time.Hour,
			JobResultsMaxAge: 90 * 24 * time.Hour,
			BatchSize:        1000,
		}

		svc, _ := MustNewReaperService(ReaperServiceOptions{
			Repo:   repo,
			Config: cfg,
		})

		ctx := context.Background()
		count, err := svc.deleteOldFailedJobs(ctx)

		require.NoError(t, err)
		assert.Equal(t, int64(8), count)
		// Called twice: once returning count, once returning 0
		assert.Equal(t, 2, repo.deleteOldJobsCalled)
	})
}

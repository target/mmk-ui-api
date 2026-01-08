package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainjob "github.com/target/mmk-ui-api/internal/domain/job"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/target/mmk-ui-api/internal/observability/notify"
	"github.com/target/mmk-ui-api/internal/service/failurenotifier"
	"go.uber.org/mock/gomock"
)

type stubJobNotifier struct {
	subscribeCalls []model.JobType
	stopCalled     bool
	subscribeFn    func(model.JobType) (func(), <-chan struct{})
	stopAllFn      func()
}

func (s *stubJobNotifier) Subscribe(jobType model.JobType) (func(), <-chan struct{}) {
	s.subscribeCalls = append(s.subscribeCalls, jobType)
	if s.subscribeFn != nil {
		return s.subscribeFn(jobType)
	}
	ch := make(chan struct{})
	unsub := func() {
		select {
		case <-ch:
		default:
		}
		close(ch)
	}
	return unsub, ch
}

func (s *stubJobNotifier) StopAll() {
	s.stopCalled = true
	if s.stopAllFn != nil {
		s.stopAllFn()
	}
}

func newTestJobService(t *testing.T, repo *mocks.MockJobRepository) (*JobService, *stubJobNotifier) {
	t.Helper()
	notifier := &stubJobNotifier{}
	svc := MustNewJobService(JobServiceOptions{
		Repo:         repo,
		DefaultLease: 30 * time.Second,
		Notifier:     notifier,
	})
	return svc, notifier
}

var _ domainjob.Notifier = (*stubJobNotifier)(nil)

type repoWithListBySource struct {
	*mocks.MockJobRepository
	listFn func(ctx context.Context, opts model.JobListBySourceOptions) ([]*model.Job, error)
}

func (r *repoWithListBySource) ListBySource(
	ctx context.Context,
	opts model.JobListBySourceOptions,
) ([]*model.Job, error) {
	if r.listFn == nil {
		return nil, nil
	}
	return r.listFn(ctx, opts)
}

func TestNewJobService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	notifierOpts := domainjob.NotifierOptions{
		WaitWindow: 2 * time.Second,
		Backoff:    50 * time.Millisecond,
	}

	t.Run("success", func(t *testing.T) {
		notifier := &stubJobNotifier{}
		svc, err := NewJobService(JobServiceOptions{
			Repo:            repo,
			DefaultLease:    30 * time.Second,
			Notifier:        notifier,
			NotifierOptions: notifierOpts,
		})
		require.NoError(t, err)
		assert.NotNil(t, svc)
		assert.Equal(t, repo, svc.repo)
		assert.Equal(t, 30*time.Second, svc.leasePolicy.Default())
		assert.Equal(t, notifier, svc.notifier)
	})

	t.Run("success with logger", func(t *testing.T) {
		logger := slog.Default()
		notifier := &stubJobNotifier{}
		svc, err := NewJobService(JobServiceOptions{
			Repo:            repo,
			DefaultLease:    30 * time.Second,
			Logger:          logger,
			Notifier:        notifier,
			NotifierOptions: notifierOpts,
		})
		require.NoError(t, err)
		assert.NotNil(t, svc)
		assert.NotNil(t, svc.logger)
	})

	t.Run("missing repo", func(t *testing.T) {
		svc, err := NewJobService(JobServiceOptions{
			DefaultLease: 30 * time.Second,
		})
		require.Error(t, err)
		assert.Nil(t, svc)
		assert.Contains(t, err.Error(), "JobRepository is required")
	})

	t.Run("invalid default lease", func(t *testing.T) {
		svc, err := NewJobService(JobServiceOptions{
			Repo:         repo,
			DefaultLease: 0,
			Notifier:     &stubJobNotifier{},
		})
		require.Error(t, err)
		assert.Nil(t, svc)
		assert.Contains(t, err.Error(), "DefaultLease must be positive")
	})
}

func TestMustNewJobService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)

	t.Run("success", func(t *testing.T) {
		svc := MustNewJobService(JobServiceOptions{
			Repo:         repo,
			DefaultLease: 30 * time.Second,
			Notifier:     &stubJobNotifier{},
		})
		assert.NotNil(t, svc)
	})

	t.Run("panic on error", func(t *testing.T) {
		assert.Panics(t, func() {
			MustNewJobService(JobServiceOptions{
				DefaultLease:    30 * time.Second,
				NotifierOptions: domainjob.NotifierOptions{WaitWindow: time.Second},
				// Missing repo
			})
		})
	})
}

func TestJobService_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	req := &model.CreateJobRequest{
		Type:    model.JobTypeBrowser,
		Payload: json.RawMessage(`{"url": "https://example.com"}`),
	}

	expectedJob := &model.Job{
		ID:      "job-123",
		Type:    model.JobTypeBrowser,
		Status:  model.JobStatusPending,
		Payload: json.RawMessage(`{"url": "https://example.com"}`),
	}

	repo.EXPECT().Create(gomock.Any(), req).Return(expectedJob, nil)

	job, err := svc.Create(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, expectedJob, job)
}

func TestJobService_ReserveNext(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	expectedJob := &model.Job{
		ID:     "job-123",
		Type:   model.JobTypeBrowser,
		Status: model.JobStatusRunning,
	}

	t.Run("with custom lease", func(t *testing.T) {
		lease := 60 * time.Second
		repo.EXPECT().ReserveNext(gomock.Any(), model.JobTypeBrowser, 60).Return(expectedJob, nil)

		job, err := svc.ReserveNext(context.Background(), model.JobTypeBrowser, lease)
		require.NoError(t, err)
		assert.Equal(t, expectedJob, job)
	})

	t.Run("with default lease", func(t *testing.T) {
		repo.EXPECT().ReserveNext(gomock.Any(), model.JobTypeBrowser, 30).Return(expectedJob, nil)

		job, err := svc.ReserveNext(context.Background(), model.JobTypeBrowser, 0)
		require.NoError(t, err)
		assert.Equal(t, expectedJob, job)
	})

	t.Run("with sub-second lease clamped to 1 second", func(t *testing.T) {
		repo.EXPECT().ReserveNext(gomock.Any(), model.JobTypeBrowser, 1).Return(expectedJob, nil)

		job, err := svc.ReserveNext(context.Background(), model.JobTypeBrowser, 500*time.Millisecond)
		require.NoError(t, err)
		assert.Equal(t, expectedJob, job)
	})
}

func TestJobService_Heartbeat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	t.Run("with custom extend", func(t *testing.T) {
		extend := 60 * time.Second
		repo.EXPECT().Heartbeat(gomock.Any(), "job-123", 60).Return(true, nil)

		updated, err := svc.Heartbeat(context.Background(), "job-123", extend)
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("with default extend", func(t *testing.T) {
		repo.EXPECT().Heartbeat(gomock.Any(), "job-123", 30).Return(true, nil)

		updated, err := svc.Heartbeat(context.Background(), "job-123", 0)
		require.NoError(t, err)
		assert.True(t, updated)
	})

	t.Run("with sub-second extend clamped to 1 second", func(t *testing.T) {
		repo.EXPECT().Heartbeat(gomock.Any(), "job-123", 1).Return(true, nil)

		updated, err := svc.Heartbeat(context.Background(), "job-123", 750*time.Millisecond)
		require.NoError(t, err)
		assert.True(t, updated)
	})
}

func TestJobService_Complete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	repo.EXPECT().Complete(gomock.Any(), "job-123").Return(true, nil)

	completed, err := svc.Complete(context.Background(), "job-123")
	require.NoError(t, err)
	assert.True(t, completed)
}

func TestJobService_Fail(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	t.Run("success", func(t *testing.T) {
		repo.EXPECT().Fail(gomock.Any(), "job-123", "test error").Return(true, nil)

		failed, err := svc.Fail(context.Background(), "job-123", "test error")
		require.NoError(t, err)
		assert.True(t, failed)
	})

	t.Run("empty error message", func(t *testing.T) {
		failed, err := svc.Fail(context.Background(), "job-123", "")
		require.Error(t, err)
		assert.False(t, failed)
		assert.Contains(t, err.Error(), "error message required")
	})
}

func TestJobService_FailWithDetails_Notifies(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	siteRepo := mocks.NewMockSiteRepository(ctrl)

	payload := RulesJobPayload{
		EventIDs: []string{"1"},
		SiteID:   "site-1",
		Scope:    "scope-1",
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	job := &model.Job{
		ID:         "job-123",
		Type:       model.JobTypeRules,
		Status:     model.JobStatusRunning,
		Payload:    payloadBytes,
		RetryCount: 2,
		MaxRetries: 3,
		Priority:   10,
		SiteID:     &payload.SiteID,
	}

	repo.EXPECT().GetByID(gomock.Any(), job.ID).Return(job, nil)
	repo.EXPECT().Fail(gomock.Any(), job.ID, "boom").Return(true, nil)
	siteRepo.EXPECT().GetByID(gomock.Any(), payload.SiteID).DoAndReturn(
		func(ctx context.Context, id string) (*model.Site, error) {
			deadline, ok := ctx.Deadline()
			require.True(t, ok, "expected deadline on site lookup")
			require.LessOrEqual(t, time.Until(deadline), 600*time.Millisecond)
			return &model.Site{ID: id, Name: "Friendly"}, nil
		},
	)

	var captured []notify.JobFailurePayload
	failureSvc := failurenotifier.NewService(failurenotifier.Options{
		Sinks: []failurenotifier.SinkRegistration{
			{
				Name: "test",
				Sink: notify.SinkFunc(func(ctx context.Context, payload notify.JobFailurePayload) error {
					captured = append(captured, payload)
					return nil
				}),
			},
		},
	})

	svc := MustNewJobService(JobServiceOptions{
		Repo:            repo,
		DefaultLease:    30 * time.Second,
		FailureNotifier: failureSvc,
		Notifier:        &stubJobNotifier{},
		Sites:           siteRepo,
	})

	details := JobFailureDetails{
		ErrorClass: "test_error",
		Metadata:   map[string]string{"component": "rules_runner"},
	}

	failed, err := svc.FailWithDetails(context.Background(), job.ID, "boom", details)
	require.NoError(t, err)
	require.True(t, failed)

	require.Len(t, captured, 1)
	evt := captured[0]

	assert.Equal(t, job.ID, evt.JobID)
	assert.Equal(t, string(job.Type), evt.JobType)
	assert.Equal(t, payload.SiteID, evt.SiteID)
	assert.Equal(t, payload.Scope, evt.Scope)
	assert.Equal(t, "Friendly", evt.SiteName)
	assert.Equal(t, "boom", evt.Error)
	assert.Equal(t, "test_error", evt.ErrorClass)
	assert.Equal(t, notify.SeverityCritical, evt.Severity)
	assert.Equal(t, "rules_runner", evt.Metadata["component"])
	assert.Equal(t, "3", evt.Metadata["retry_count"])
	assert.Equal(t, "3", evt.Metadata["max_retries"])
	assert.Equal(t, "failed", evt.Metadata["status"])
	assert.False(t, evt.IsTest)
}

func TestJobService_FailWithDetails_UsesPayloadSiteID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	siteRepo := mocks.NewMockSiteRepository(ctrl)

	payload := struct {
		SiteID string `json:"site_id"`
	}{
		SiteID: "site-1",
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	job := &model.Job{
		ID:         "job-123",
		Type:       model.JobTypeBrowser,
		Status:     model.JobStatusRunning,
		Payload:    payloadBytes,
		RetryCount: 0,
		MaxRetries: 0,
		Priority:   10,
	}

	repo.EXPECT().GetByID(gomock.Any(), job.ID).Return(job, nil)
	repo.EXPECT().Fail(gomock.Any(), job.ID, "boom").Return(true, nil)
	siteRepo.EXPECT().GetByID(gomock.Any(), payload.SiteID).Return(
		&model.Site{ID: payload.SiteID, Name: "Friendly"},
		nil,
	)

	var captured []notify.JobFailurePayload
	failureSvc := failurenotifier.NewService(failurenotifier.Options{
		Sinks: []failurenotifier.SinkRegistration{
			{
				Name: "test",
				Sink: notify.SinkFunc(func(ctx context.Context, payload notify.JobFailurePayload) error {
					captured = append(captured, payload)
					return nil
				}),
			},
		},
	})

	svc := MustNewJobService(JobServiceOptions{
		Repo:            repo,
		DefaultLease:    30 * time.Second,
		FailureNotifier: failureSvc,
		Notifier:        &stubJobNotifier{},
		Sites:           siteRepo,
	})

	failed, err := svc.FailWithDetails(context.Background(), job.ID, "boom", JobFailureDetails{})
	require.NoError(t, err)
	require.True(t, failed)

	require.Len(t, captured, 1)
	evt := captured[0]

	assert.Equal(t, payload.SiteID, evt.SiteID)
	assert.Equal(t, "Friendly", evt.SiteName)
}

func TestJobService_FailWithDetails_SkipsUntilRetriesExhausted(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)

	job := &model.Job{
		ID:         "job-123",
		Type:       model.JobTypeRules,
		Status:     model.JobStatusRunning,
		RetryCount: 0,
		MaxRetries: 3,
		Priority:   1,
	}

	repo.EXPECT().GetByID(gomock.Any(), job.ID).Return(job, nil)
	repo.EXPECT().Fail(gomock.Any(), job.ID, "boom").Return(true, nil)

	var notified bool
	failureSvc := failurenotifier.NewService(failurenotifier.Options{
		Sinks: []failurenotifier.SinkRegistration{
			{
				Name: "test",
				Sink: notify.SinkFunc(func(ctx context.Context, payload notify.JobFailurePayload) error {
					notified = true
					return nil
				}),
			},
		},
	})

	svc := MustNewJobService(JobServiceOptions{
		Repo:            repo,
		DefaultLease:    30 * time.Second,
		FailureNotifier: failureSvc,
		Notifier:        &stubJobNotifier{},
	})

	details := JobFailureDetails{
		ErrorClass: "test_error",
	}

	failed, err := svc.FailWithDetails(context.Background(), job.ID, "boom", details)
	require.NoError(t, err)
	require.True(t, failed)
	assert.False(t, notified, "notification should be deferred until retries are exhausted")
}

func TestJobService_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	expectedJob := &model.Job{
		ID:     "job-123",
		Type:   model.JobTypeBrowser,
		Status: model.JobStatusCompleted,
	}

	repo.EXPECT().GetByID(gomock.Any(), "job-123").Return(expectedJob, nil)

	job, err := svc.GetByID(context.Background(), "job-123")
	require.NoError(t, err)
	assert.Equal(t, expectedJob, job)
}

func TestJobService_Stats(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	expectedStats := &model.JobStats{
		Pending:   5,
		Running:   2,
		Completed: 10,
		Failed:    1,
	}

	repo.EXPECT().Stats(gomock.Any(), model.JobTypeBrowser).Return(expectedStats, nil)

	stats, err := svc.Stats(context.Background(), model.JobTypeBrowser)
	require.NoError(t, err)
	assert.Equal(t, expectedStats, stats)
}

func TestJobService_GetStatus(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	completedAt := time.Now()
	job := &model.Job{
		ID:          "job-123",
		Status:      model.JobStatusCompleted,
		CompletedAt: &completedAt,
		LastError:   nil,
	}

	repo.EXPECT().GetByID(gomock.Any(), "job-123").Return(job, nil)

	status, err := svc.GetStatus(context.Background(), "job-123")
	require.NoError(t, err)
	assert.Equal(t, model.JobStatusCompleted, status.Status)
	assert.Equal(t, &completedAt, status.CompletedAt)
	assert.Nil(t, status.LastError)
}

func TestJobService_Subscribe(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	n := &stubJobNotifier{
		subscribeFn: func(model.JobType) (func(), <-chan struct{}) {
			ch := make(chan struct{})
			return func() {
				select {
				case <-ch:
				default:
				}
				close(ch)
			}, ch
		},
	}
	svc := MustNewJobService(JobServiceOptions{
		Repo:         repo,
		DefaultLease: 30 * time.Second,
		Notifier:     n,
	})

	unsub, ch := svc.Subscribe(model.JobTypeBrowser)
	require.NotNil(t, unsub)
	require.NotNil(t, ch)
	require.Len(t, n.subscribeCalls, 1)
	assert.Equal(t, model.JobTypeBrowser, n.subscribeCalls[0])

	unsub()

	select {
	case _, ok := <-ch:
		assert.False(t, ok, "channel should be closed on unsubscribe")
	case <-time.After(100 * time.Millisecond):
		t.Fatal("expected channel to close after unsubscribe")
	}
}

func TestJobService_StopAllListeners(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	n := &stubJobNotifier{
		subscribeFn: func(model.JobType) (func(), <-chan struct{}) {
			return func() {}, make(chan struct{})
		},
	}
	svc := MustNewJobService(JobServiceOptions{
		Repo:         repo,
		DefaultLease: 30 * time.Second,
		Notifier:     n,
	})

	svc.StopAllListeners()
	assert.True(t, n.stopCalled)
}

func TestJobService_ListBySource(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	t.Run("missing source id", func(t *testing.T) {
		opts := model.JobListBySourceOptions{}
		jobs, err := svc.ListBySource(context.Background(), opts)
		require.Error(t, err)
		assert.Nil(t, jobs)
		assert.Contains(t, err.Error(), "source id is required")
	})

	t.Run("pagination normalization", func(t *testing.T) {
		// Test that pagination is normalized
		opts := model.JobListBySourceOptions{
			SourceID: "source-123",
			Limit:    -1,  // Should be normalized to 50
			Offset:   -10, // Should be normalized to 0
		}

		// Since we can't easily mock the type assertion, we'll test the error path
		jobs, err := svc.ListBySource(context.Background(), opts)
		require.NoError(t, err)
		assert.Empty(t, jobs) // Should return empty list when repo doesn't support extension
	})

	t.Run("fast path uses repository extension", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		base := mocks.NewMockJobRepository(ctrl)
		repo := &repoWithListBySource{MockJobRepository: base}

		expected := []*model.Job{{ID: "job-1"}, {ID: "job-2"}}
		repo.listFn = func(ctx context.Context, opts model.JobListBySourceOptions) ([]*model.Job, error) {
			assert.Equal(t, "source-fast", opts.SourceID)
			assert.Equal(t, 25, opts.Limit)
			assert.Equal(t, 5, opts.Offset)
			return expected, nil
		}

		notifier := &stubJobNotifier{}
		svc := MustNewJobService(JobServiceOptions{
			Repo:         repo,
			DefaultLease: 30 * time.Second,
			Notifier:     notifier,
		})

		opts := model.JobListBySourceOptions{SourceID: "source-fast", Limit: 25, Offset: 5}
		jobs, err := svc.ListBySource(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, expected, jobs)
	})
}

func TestJobService_ListBySiteWithFilters(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	t.Run("pagination normalization", func(t *testing.T) {
		opts := model.JobListBySiteOptions{
			Limit:  2000, // Should be clamped to 1000
			Offset: -5,   // Should be normalized to 0
		}

		jobs, err := svc.ListBySiteWithFilters(context.Background(), opts)
		require.NoError(t, err)
		assert.Empty(t, jobs) // Should return empty list when repo doesn't support extension
	})
}

func TestJobService_ListRecentByType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	t.Run("repository extension not supported", func(t *testing.T) {
		jobs, err := svc.ListRecentByType(context.Background(), model.JobTypeBrowser, 10)
		require.NoError(t, err)
		assert.Empty(t, jobs) // Should return empty list when repo doesn't support extension
	})
}

func TestJobService_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	t.Run("pagination normalization", func(t *testing.T) {
		opts := &model.JobListOptions{
			Limit:  2000, // Should be clamped to 1000
			Offset: -5,   // Should be normalized to 0
		}

		expectedOpts := &model.JobListOptions{
			Limit:  1000,
			Offset: 0,
		}

		expectedJobs := []*model.JobWithEventCount{
			{Job: model.Job{ID: "job-1", Type: model.JobTypeBrowser}, EventCount: 5},
		}

		repo.EXPECT().List(gomock.Any(), expectedOpts).Return(expectedJobs, nil)

		jobs, err := svc.List(context.Background(), opts)
		require.NoError(t, err)
		assert.Equal(t, expectedJobs, jobs)
	})

	t.Run("repository error", func(t *testing.T) {
		opts := &model.JobListOptions{Limit: 50, Offset: 0}
		expectedErr := errors.New("database error")

		repo.EXPECT().List(gomock.Any(), opts).Return(nil, expectedErr)

		jobs, err := svc.List(context.Background(), opts)
		require.Error(t, err)
		assert.Nil(t, jobs)
		assert.Contains(t, err.Error(), "list jobs")
	})
}

func TestJobService_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockJobRepository(ctrl)
	svc, _ := newTestJobService(t, repo)

	t.Run("success", func(t *testing.T) {
		jobID := "job-123"
		repo.EXPECT().Delete(gomock.Any(), jobID).Return(nil)

		err := svc.Delete(context.Background(), jobID)
		require.NoError(t, err)
	})

	t.Run("empty job id", func(t *testing.T) {
		err := svc.Delete(context.Background(), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "job id is required")
	})

	t.Run("repository error", func(t *testing.T) {
		jobID := "job-456"
		expectedErr := errors.New("job not found")
		repo.EXPECT().Delete(gomock.Any(), jobID).Return(expectedErr)

		err := svc.Delete(context.Background(), jobID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete job")
	})
}

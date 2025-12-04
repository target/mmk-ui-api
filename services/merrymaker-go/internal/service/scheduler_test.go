package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
	domainscheduler "github.com/target/mmk-ui-api/internal/domain/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

const (
	testTaskID      = "task-1"
	testPayloadJSON = `{"test": true}`
)

// Mock implementations for testing.
type mockScheduledJobsRepo struct {
	mock.Mock
}

func (m *mockScheduledJobsRepo) FindDue(ctx context.Context, now time.Time, limit int) ([]domain.ScheduledTask, error) {
	args := m.Called(ctx, now, limit)
	return args.Get(0).([]domain.ScheduledTask), args.Error(1)
}

func (m *mockScheduledJobsRepo) FindDueTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.FindDueParams,
) ([]domain.ScheduledTask, error) {
	args := m.Called(ctx, tx, p)
	return args.Get(0).([]domain.ScheduledTask), args.Error(1)
}

func (m *mockScheduledJobsRepo) MarkQueued(ctx context.Context, id string, now time.Time) (bool, error) {
	args := m.Called(ctx, id, now)
	return args.Bool(0), args.Error(1)
}

func (m *mockScheduledJobsRepo) MarkQueuedTx(ctx context.Context, tx *sql.Tx, p domain.MarkQueuedParams) (bool, error) {
	args := m.Called(ctx, tx, p)
	return args.Bool(0), args.Error(1)
}

func (m *mockScheduledJobsRepo) TryWithTaskLock(
	ctx context.Context,
	taskName string,
	fn func(context.Context, *sql.Tx) error,
) (bool, error) {
	args := m.Called(ctx, taskName, fn)
	if args.Bool(0) {
		// Simulate successful lock acquisition by calling the function
		return true, fn(ctx, nil) // Pass nil tx for unit tests
	}
	return false, args.Error(1)
}

func (m *mockScheduledJobsRepo) UpdateActiveFireKeyTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.UpdateActiveFireKeyParams,
) error {
	args := m.Called(ctx, tx, p)
	return args.Error(0)
}

type mockJobRepository struct {
	mock.Mock
}

func (m *mockJobRepository) Create(ctx context.Context, req *model.CreateJobRequest) (*model.Job, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Job), args.Error(1)
}

func (m *mockJobRepository) CreateInTx(
	ctx context.Context,
	tx *sql.Tx,
	req *model.CreateJobRequest,
) (*model.Job, error) {
	args := m.Called(ctx, tx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Job), args.Error(1)
}

func (m *mockJobRepository) GetByID(ctx context.Context, id string) (*model.Job, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Job), args.Error(1)
}

func (m *mockJobRepository) ReserveNext(
	ctx context.Context,
	jobType model.JobType,
	leaseSeconds int,
) (*model.Job, error) {
	args := m.Called(ctx, jobType, leaseSeconds)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Job), args.Error(1)
}

func (m *mockJobRepository) WaitForNotification(ctx context.Context, jobType model.JobType) error {
	args := m.Called(ctx, jobType)
	return args.Error(0)
}

func (m *mockJobRepository) Heartbeat(ctx context.Context, jobID string, leaseSeconds int) (bool, error) {
	args := m.Called(ctx, jobID, leaseSeconds)
	return args.Bool(0), args.Error(1)
}

func (m *mockJobRepository) Complete(ctx context.Context, id string) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *mockJobRepository) Fail(ctx context.Context, id, errMsg string) (bool, error) {
	args := m.Called(ctx, id, errMsg)
	return args.Bool(0), args.Error(1)
}

func (m *mockJobRepository) Stats(ctx context.Context, jobType model.JobType) (*model.JobStats, error) {
	args := m.Called(ctx, jobType)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.JobStats), args.Error(1)
}

func (m *mockJobRepository) List(ctx context.Context, opts *model.JobListOptions) ([]*model.JobWithEventCount, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.JobWithEventCount), args.Error(1)
}

func (m *mockJobRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockJobRepository) DeleteByPayloadField(
	ctx context.Context,
	params core.DeleteByPayloadFieldParams,
) (int, error) {
	args := m.Called(ctx, params)
	return args.Int(0), args.Error(1)
}

type mockJobIntrospector struct {
	mock.Mock
}

func (m *mockJobIntrospector) RunningJobExistsByTaskName(
	ctx context.Context,
	taskName string,
	now time.Time,
) (bool, error) {
	args := m.Called(ctx, taskName, now)
	return args.Bool(0), args.Error(1)
}

func (m *mockJobIntrospector) JobStatesByTaskName(
	ctx context.Context,
	taskName string,
	now time.Time,
) (domain.OverrunStateMask, error) {
	args := m.Called(ctx, taskName, now)
	mask, _ := args.Get(0).(domain.OverrunStateMask)
	return mask, args.Error(1)
}

func TestSchedulerService_Tick_NoTasks(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	// Mock FindDue to return empty slice
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{}, nil)

	processed, err := scheduler.Tick(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, processed)
	mockRepo.AssertExpectations(t)
}

func TestSchedulerService_Tick_SingleTask_QueuePolicy(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicyQueue

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	// Mock FindDue to return one task
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to succeed
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)

	// Mock job creation
	expectedJob := &model.Job{ID: "job-1", Type: model.JobTypeBrowser}
	mockJobs.On("Create", ctx, mock.MatchedBy(func(req *model.CreateJobRequest) bool {
		return req.Type == model.JobTypeBrowser &&
			req.Priority == 0 &&
			req.MaxRetries == 3 &&
			string(req.Payload) == testPayloadJSON
	})).Return(expectedJob, nil)

	// Mock MarkQueuedTx for Queue policy (called after enqueue)
	mockRepo.On("MarkQueuedTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.MarkQueuedParams) bool {
		return p.ID == testTaskID && p.Now.Equal(now) && p.ActiveFireKey != nil && *p.ActiveFireKey != "" &&
			p.ActiveFireKeySetAt != nil
	})).Return(true, nil)

	processed, err := scheduler.Tick(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	mockRepo.AssertExpectations(t)
	mockJobs.AssertExpectations(t)
}

func TestSchedulerService_Tick_SingleTask_SkipPolicy_NoRunningJob(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicySkip

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	// Mock FindDue to return one task
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to succeed
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)

	// Mock no outstanding job states
	mockJobq.On("JobStatesByTaskName", ctx, "test-task", now).Return(domain.OverrunStateMask(0), nil)

	// Mock MarkQueuedTx for Skip policy (called before enqueue)
	mockRepo.On("MarkQueuedTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.MarkQueuedParams) bool {
		return p.ID == testTaskID && p.Now.Equal(now)
	})).Return(true, nil)
	mockRepo.On("UpdateActiveFireKeyTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.UpdateActiveFireKeyParams) bool {
		return p.ID == testTaskID && p.FireKey != nil && *p.FireKey != ""
	})).
		Return(nil)

	// Mock job creation
	expectedJob := &model.Job{ID: "job-1", Type: model.JobTypeBrowser}
	mockJobs.On("Create", ctx, mock.AnythingOfType("*model.CreateJobRequest")).Return(expectedJob, nil)

	processed, err := scheduler.Tick(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	mockRepo.AssertExpectations(t)
	mockJobs.AssertExpectations(t)
	mockJobq.AssertExpectations(t)
}

func TestSchedulerService_Tick_SkipPolicy_SetActiveFireKeyError(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicySkip

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)
	mockJobq.On("JobStatesByTaskName", ctx, "test-task", now).Return(domain.OverrunStateMask(0), nil)
	mockRepo.On("MarkQueuedTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.MarkQueuedParams) bool {
		return p.ID == testTaskID && p.Now.Equal(now)
	})).Return(true, nil)
	mockJobs.On("Create", ctx, mock.AnythingOfType("*model.CreateJobRequest")).Return(&model.Job{ID: "job-1"}, nil)
	mockRepo.On("UpdateActiveFireKeyTx", ctx, (*sql.Tx)(nil), mock.AnythingOfType("domain.UpdateActiveFireKeyParams")).
		Return(errors.New("set key failed"))

	processed, err := scheduler.Tick(ctx, now)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "set active fire key")
	assert.Equal(t, 0, processed)
	mockRepo.AssertExpectations(t)
	mockJobs.AssertExpectations(t)
	mockJobq.AssertExpectations(t)
}

func TestSchedulerService_Tick_SingleTask_SkipPolicy_RunningJobExists(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicySkip

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	// Mock FindDue to return one task
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to succeed
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)

	// Mock running job exists - should skip enqueue
	mockJobq.On("JobStatesByTaskName", ctx, "test-task", now).Return(domain.OverrunStateRunning, nil)

	// Mock MarkQueuedTx for Skip policy (called before enqueue check)
	mockRepo.On("MarkQueuedTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.MarkQueuedParams) bool {
		return p.ID == testTaskID && p.Now.Equal(now)
	})).Return(true, nil)

	// Job creation should NOT be called since we skip

	processed, err := scheduler.Tick(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	mockRepo.AssertExpectations(t)
	mockJobs.AssertExpectations(t)
	mockJobq.AssertExpectations(t)
}

func TestSchedulerService_Tick_SkipPolicy_PendingStateBlocks(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicySkip

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	stateMask := domain.OverrunStateRunning | domain.OverrunStatePending | domain.OverrunStateRetrying
	task := domain.ScheduledTask{
		ID:            testTaskID,
		TaskName:      "test-task",
		Payload:       json.RawMessage(testPayloadJSON),
		Interval:      5 * time.Minute,
		OverrunStates: &stateMask,
	}

	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)
	mockJobq.On("JobStatesByTaskName", ctx, "test-task", now).Return(domain.OverrunStatePending, nil)
	mockRepo.On("MarkQueuedTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.MarkQueuedParams) bool {
		return p.ID == testTaskID && p.Now.Equal(now)
	})).Return(true, nil)

	processed, err := scheduler.Tick(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	mockJobs.AssertNotCalled(t, "Create", mock.Anything)
	mockRepo.AssertExpectations(t)
	mockJobq.AssertExpectations(t)
}

func TestSchedulerService_Tick_SingleTask_ReschedulePolicy(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicyReschedule

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	// Mock FindDue to return one task
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to succeed
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)

	// Mock MarkQueuedTx for Reschedule policy (called before enqueue check)
	mockRepo.On("MarkQueuedTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.MarkQueuedParams) bool {
		return p.ID == testTaskID && p.Now.Equal(now)
	})).Return(true, nil)

	// Job creation should NOT be called since we reschedule without enqueue

	processed, err := scheduler.Tick(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	mockRepo.AssertExpectations(t)
	mockJobs.AssertExpectations(t)
}

func TestSchedulerService_Tick_LockNotAcquired(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(`{"test": true}`),
		Interval: 5 * time.Minute,
	}

	// Mock FindDue to return one task
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to fail (another replica has the lock)
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(false, nil)

	processed, err := scheduler.Tick(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, processed) // No tasks processed since lock not acquired
	mockRepo.AssertExpectations(t)
}

func TestSchedulerService_Tick_FindDueError(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	// Mock FindDue to return error
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{}, errors.New("database error"))

	processed, err := scheduler.Tick(ctx, now)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "find due tasks")
	assert.Equal(t, 0, processed)
	mockRepo.AssertExpectations(t)
}

func TestSchedulerService_Tick_JobCreationError(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicyQueue

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	// Mock FindDue to return one task
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to succeed
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)

	// Mock job creation to fail
	mockJobs.On("Create", ctx, mock.AnythingOfType("*model.CreateJobRequest")).
		Return(nil, errors.New("job creation failed"))

	processed, err := scheduler.Tick(ctx, now)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "process task test-task")
	assert.Contains(t, err.Error(), "enqueue job")
	assert.Equal(t, 0, processed)
	mockRepo.AssertExpectations(t)
	mockJobs.AssertExpectations(t)
}

func TestSchedulerService_Tick_JobIntrospectorError(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicySkip

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	// Mock FindDue to return one task
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to succeed
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)

	// Mock job introspector to fail
	mockJobq.On("JobStatesByTaskName", ctx, "test-task", now).
		Return(domain.OverrunStateMask(0), errors.New("introspector error"))

	processed, err := scheduler.Tick(ctx, now)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "process task test-task")
	assert.Contains(t, err.Error(), "check overrun policy")
	assert.Equal(t, 0, processed)
	mockRepo.AssertExpectations(t)
	mockJobq.AssertExpectations(t)
}

func TestSchedulerService_Tick_MarkQueuedError(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicySkip

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	// Mock FindDue to return one task
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to succeed
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)

	// Mock no outstanding job states
	mockJobq.On("JobStatesByTaskName", ctx, "test-task", now).Return(domain.OverrunStateMask(0), nil)

	// Mock MarkQueuedTx to fail
	mockRepo.On("MarkQueuedTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.MarkQueuedParams) bool {
		return p.ID == testTaskID && p.Now.Equal(now)
	})).Return(false, errors.New("mark queued failed"))

	processed, err := scheduler.Tick(ctx, now)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "process task test-task")
	assert.Contains(t, err.Error(), "mark task queued")
	assert.Equal(t, 0, processed)
	mockRepo.AssertExpectations(t)
	mockJobq.AssertExpectations(t)
}

func TestSchedulerService_Tick_DefensiveRecheck_TaskNoLongerDue(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}

	// Use a fixed time provider that we can control
	fixedTime := time.Now()
	timeProvider := data.NewFixedTimeProvider(fixedTime)

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := fixedTime

	// Create a task that was due when FindDue was called, but is no longer due
	// when the defensive recheck happens (simulating a race condition)
	task := domain.ScheduledTask{
		ID:           testTaskID,
		TaskName:     "test-task",
		Payload:      json.RawMessage(testPayloadJSON),
		Interval:     5 * time.Minute,
		LastQueuedAt: &fixedTime, // Just queued, so not due anymore
	}

	// Mock FindDue to return the task (as if it was due at query time)
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to succeed
	mockRepo.On("TryWithTaskLock", ctx, "test-task", mock.Anything).Return(true, nil)

	// No other mocks should be called since the defensive recheck should skip the task

	processed, err := scheduler.Tick(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 0, processed) // No-op due to defensive recheck; no state change performed
	mockRepo.AssertExpectations(t)
	mockJobs.AssertExpectations(t)
	mockJobq.AssertExpectations(t)
}

func TestSchedulerService_Tick_TimeBoundaryEdgeCase(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}

	// Test with a task that becomes due exactly at the boundary
	baseTime := time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC)
	timeProvider := data.NewFixedTimeProvider(baseTime)

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicyQueue

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := baseTime

	// Task was last queued exactly 5 minutes ago, with 5-minute interval
	lastQueued := baseTime.Add(-5 * time.Minute)
	task := domain.ScheduledTask{
		ID:           testTaskID,
		TaskName:     "boundary-task",
		Payload:      json.RawMessage(testPayloadJSON),
		Interval:     5 * time.Minute,
		LastQueuedAt: &lastQueued,
	}

	// Mock FindDue to return the task
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task}, nil)

	// Mock TryWithTaskLock to succeed
	mockRepo.On("TryWithTaskLock", ctx, "boundary-task", mock.Anything).Return(true, nil)

	// Mock job creation
	expectedJob := &model.Job{ID: "job-1", Type: model.JobTypeBrowser}
	mockJobs.On("Create", ctx, mock.AnythingOfType("*model.CreateJobRequest")).Return(expectedJob, nil)

	// Mock MarkQueuedTx for Queue policy
	mockRepo.On("MarkQueuedTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.MarkQueuedParams) bool {
		return p.ID == testTaskID && p.Now.Equal(now)
	})).Return(true, nil)

	processed, err := scheduler.Tick(ctx, now)

	require.NoError(t, err)
	assert.Equal(t, 1, processed)
	mockRepo.AssertExpectations(t)
	mockJobs.AssertExpectations(t)
}

func TestSchedulerService_Tick_MultipleTasks_PartialFailure(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	cfg := core.DefaultSchedulerConfig()
	cfg.Strategy.Overrun = domain.OverrunPolicyQueue

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	now := timeProvider.Now()

	task1 := domain.ScheduledTask{
		ID:       "task-1",
		TaskName: "success-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	task2 := domain.ScheduledTask{
		ID:       "task-2",
		TaskName: "failure-task",
		Payload:  json.RawMessage(testPayloadJSON),
		Interval: 5 * time.Minute,
	}

	// Mock FindDue to return both tasks
	mockRepo.On("FindDue", ctx, now, 25).Return([]domain.ScheduledTask{task1, task2}, nil)

	// Mock first task to succeed
	mockRepo.On("TryWithTaskLock", ctx, "success-task", mock.Anything).Return(true, nil)
	expectedJob := &model.Job{ID: "job-1", Type: model.JobTypeBrowser}
	mockJobs.On("Create", ctx, mock.MatchedBy(func(req *model.CreateJobRequest) bool {
		return string(req.Payload) == testPayloadJSON
	})).Return(expectedJob, nil).Once()
	mockRepo.On("MarkQueuedTx", ctx, (*sql.Tx)(nil), mock.MatchedBy(func(p domain.MarkQueuedParams) bool {
		return p.ID == "task-1"
	})).Return(true, nil)

	// Mock second task to fail during job creation
	mockRepo.On("TryWithTaskLock", ctx, "failure-task", mock.Anything).Return(true, nil)
	mockJobs.On("Create", ctx, mock.MatchedBy(func(req *model.CreateJobRequest) bool {
		return string(req.Payload) == testPayloadJSON
	})).Return(nil, errors.New("job creation failed")).Once()

	processed, err := scheduler.Tick(ctx, now)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "process task failure-task")
	assert.Equal(t, 1, processed) // First task was processed successfully
	mockRepo.AssertExpectations(t)
	mockJobs.AssertExpectations(t)
}

func TestSchedulerService_Configuration_Defaults(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}

	// Test with nil config - should use defaults
	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          nil, // Should use defaults
		TimeProvider:    nil, // Should use real time provider
	})

	// Verify defaults are applied
	assert.Equal(t, 25, scheduler.cfg.BatchSize)
	assert.Equal(t, model.JobTypeBrowser, scheduler.cfg.DefaultJobType)
	assert.Equal(t, 0, scheduler.cfg.DefaultPriority)
	assert.Equal(t, 3, scheduler.cfg.MaxRetries)
	assert.Equal(t, domain.OverrunPolicySkip, scheduler.cfg.Strategy.Overrun)
	assert.NotNil(t, scheduler.timeProvider)
}

func TestSchedulerService_Configuration_CustomValues(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	// Test with custom config
	cfg := core.SchedulerConfig{
		BatchSize:       50,
		DefaultJobType:  model.JobTypeRules,
		DefaultPriority: 10,
		MaxRetries:      5,
		Strategy: domain.StrategyOptions{
			Overrun: domain.OverrunPolicyQueue,
		},
	}

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		Config:          &cfg,
		TimeProvider:    timeProvider,
	})

	// Verify custom values are used
	assert.Equal(t, 50, scheduler.cfg.BatchSize)
	assert.Equal(t, model.JobTypeRules, scheduler.cfg.DefaultJobType)
	assert.Equal(t, 10, scheduler.cfg.DefaultPriority)
	assert.Equal(t, 5, scheduler.cfg.MaxRetries)
	assert.Equal(t, domain.OverrunPolicyQueue, scheduler.cfg.Strategy.Overrun)
	assert.Equal(t, timeProvider, scheduler.timeProvider)
}

func TestSchedulerService_EnqueueJob_SiteRunAssociations(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	siteID := "550e8400-e29b-41d4-a716-446655440000"
	sourceID := "660f9500-f39c-52e5-b827-557766551111"

	// Create task payload with site_id and source_id
	payload := struct {
		SiteID   string `json:"site_id"`
		SourceID string `json:"source_id"`
	}{
		SiteID:   siteID,
		SourceID: sourceID,
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "site:" + siteID,
		Payload:  payloadBytes,
		Interval: 5 * time.Minute,
	}

	// Mock job creation with expected SiteID and SourceID
	mockJobs.On("Create", ctx, mock.MatchedBy(func(req *model.CreateJobRequest) bool {
		return req.SiteID != nil && *req.SiteID == siteID &&
			req.SourceID != nil && *req.SourceID == sourceID &&
			req.IsTest == false
	})).Return(&model.Job{ID: "job-123"}, nil)

	fireKey := domainscheduler.ComputeFireKey(task, timeProvider.Now())

	// Execute
	created, err := scheduler.enqueueJob(ctx, enqueueJobParams{
		Task:    task,
		FireKey: fireKey,
	})

	// Assert
	require.NoError(t, err)
	require.True(t, created)
	mockJobs.AssertExpectations(t)
}

func TestSchedulerService_EnqueueJob_UsesTransactionalRepository(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()
	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "test-task",
		Payload:  json.RawMessage(`{"foo": "bar"}`),
		Interval: time.Minute,
	}

	var dummyTx sql.Tx
	mockJobs.On("CreateInTx", ctx, &dummyTx, mock.AnythingOfType("*model.CreateJobRequest")).
		Return(&model.Job{ID: "job-456"}, nil)

	fireKey := domainscheduler.ComputeFireKey(task, timeProvider.Now())

	created, err := scheduler.enqueueJob(ctx, enqueueJobParams{
		Tx:      &dummyTx,
		Task:    task,
		FireKey: fireKey,
	})

	require.NoError(t, err)
	assert.True(t, created)
	mockJobs.AssertCalled(t, "CreateInTx", ctx, &dummyTx, mock.AnythingOfType("*model.CreateJobRequest"))
	mockJobs.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestSchedulerService_EnqueueJob_InvalidUUIDs(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()

	// Create task payload with invalid UUIDs
	payload := struct {
		SiteID   string `json:"site_id"`
		SourceID string `json:"source_id"`
	}{
		SiteID:   "invalid-site-id",
		SourceID: "invalid-source-id",
	}
	payloadBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "site:invalid",
		Payload:  payloadBytes,
		Interval: 5 * time.Minute,
	}

	// Mock job creation - should not have SiteID or SourceID set due to invalid format
	mockJobs.On("Create", ctx, mock.MatchedBy(func(req *model.CreateJobRequest) bool {
		return req.SiteID == nil && req.SourceID == nil && req.IsTest == false
	})).Return(&model.Job{ID: "job-123"}, nil)

	fireKey := domainscheduler.ComputeFireKey(task, timeProvider.Now())

	// Execute
	created, err := scheduler.enqueueJob(ctx, enqueueJobParams{
		Task:    task,
		FireKey: fireKey,
	})

	// Assert
	require.NoError(t, err)
	require.True(t, created)
	mockJobs.AssertExpectations(t)
}

func TestSchedulerService_EnqueueJob_InvalidPayload(t *testing.T) {
	mockRepo := &mockScheduledJobsRepo{}
	mockJobs := &mockJobRepository{}
	mockJobq := &mockJobIntrospector{}
	timeProvider := data.NewFixedTimeProvider(time.Now())

	scheduler := NewSchedulerService(SchedulerServiceOptions{
		Repo:            mockRepo,
		Jobs:            mockJobs,
		JobIntrospector: mockJobq,
		TimeProvider:    timeProvider,
	})

	ctx := context.Background()

	// Create task with invalid JSON payload
	task := domain.ScheduledTask{
		ID:       testTaskID,
		TaskName: "invalid-task",
		Payload:  json.RawMessage(`{invalid json`),
		Interval: 5 * time.Minute,
	}

	fireKey := domainscheduler.ComputeFireKey(task, timeProvider.Now())

	// Execute - should fail with invalid payload (no more legacy support)
	created, err := scheduler.enqueueJob(ctx, enqueueJobParams{
		Task:    task,
		FireKey: fireKey,
	})

	// Assert - should fail due to invalid JSON
	require.Error(t, err)
	require.False(t, created)
	require.Contains(t, err.Error(), "parse task payload")
}

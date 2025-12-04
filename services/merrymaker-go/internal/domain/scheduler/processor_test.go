package scheduler_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/scheduler"
)

type stubTaskStore struct {
	markParams   []domain.MarkQueuedParams
	markResults  []bool
	markErrors   []error
	updateParams []domain.UpdateActiveFireKeyParams
	updateErr    error
}

func (s *stubTaskStore) MarkQueued(ctx context.Context, params domain.MarkQueuedParams) (bool, error) {
	s.markParams = append(s.markParams, params)
	var result bool
	if len(s.markResults) > 0 {
		result = s.markResults[0]
		s.markResults = s.markResults[1:]
	}
	var err error
	if len(s.markErrors) > 0 {
		err = s.markErrors[0]
		s.markErrors = s.markErrors[1:]
	}
	return result, err
}

func (s *stubTaskStore) UpdateActiveFireKey(ctx context.Context, params domain.UpdateActiveFireKeyParams) error {
	s.updateParams = append(s.updateParams, params)
	return s.updateErr
}

type stubJobStateReader struct {
	mask domain.OverrunStateMask
	err  error
}

func (s *stubJobStateReader) JobStatesByTaskName(
	ctx context.Context,
	taskName string,
	now time.Time,
) (domain.OverrunStateMask, error) {
	return s.mask, s.err
}

type stubJobEnqueuer struct {
	created bool
	err     error
	calls   []struct {
		task    domain.ScheduledTask
		fireKey string
	}
}

func (s *stubJobEnqueuer) Enqueue(
	ctx context.Context,
	task domain.ScheduledTask,
	fireKey string,
) (bool, error) {
	s.calls = append(s.calls, struct {
		task    domain.ScheduledTask
		fireKey string
	}{task: task, fireKey: fireKey})
	return s.created, s.err
}

func TestTaskProcessor_TaskNotDue(t *testing.T) {
	now := time.Now()
	last := now.Add(-30 * time.Second)
	task := domain.ScheduledTask{
		ID:           "task-1",
		Interval:     time.Minute,
		LastQueuedAt: &last,
	}

	reader := &stubJobStateReader{}
	store := &stubTaskStore{}

	processor := scheduler.NewTaskProcessor(scheduler.TaskProcessorOptions{
		StateReader: reader,
	})

	result, err := processor.Process(context.Background(), scheduler.ProcessParams{
		Task:  task,
		Now:   now,
		Store: store,
	})
	require.NoError(t, err)
	assert.False(t, result.Worked)
	assert.Empty(t, store.markParams)
}

func TestTaskProcessor_SkipPolicyBlocked(t *testing.T) {
	now := time.Now()
	task := domain.ScheduledTask{
		ID:       "skip-blocked",
		TaskName: "alerts",
		Interval: time.Minute,
	}

	reader := &stubJobStateReader{mask: domain.OverrunStateRunning}
	store := &stubTaskStore{
		markResults: []bool{true},
	}

	processor := scheduler.NewTaskProcessor(scheduler.TaskProcessorOptions{
		StateReader: reader,
	})

	result, err := processor.Process(context.Background(), scheduler.ProcessParams{
		Task:  task,
		Now:   now,
		Store: store,
	})
	require.NoError(t, err)
	assert.True(t, result.MarkedQueued)
	assert.True(t, result.Worked)
	assert.False(t, result.Enqueued)
	assert.Len(t, store.markParams, 1)
}

func TestTaskProcessor_SkipPolicyEnqueues(t *testing.T) {
	now := time.Now()
	task := domain.ScheduledTask{
		ID:       "skip-ok",
		TaskName: "alerts",
		Interval: time.Minute,
	}

	reader := &stubJobStateReader{}
	store := &stubTaskStore{
		markResults: []bool{true},
	}
	enqueuer := &stubJobEnqueuer{created: true}

	processor := scheduler.NewTaskProcessor(scheduler.TaskProcessorOptions{
		StateReader: reader,
	})

	result, err := processor.Process(context.Background(), scheduler.ProcessParams{
		Task:     task,
		Now:      now,
		Store:    store,
		Enqueuer: enqueuer,
	})
	require.NoError(t, err)
	require.True(t, result.Enqueued)
	require.True(t, result.Worked)
	assert.Len(t, store.markParams, 1)
	require.Len(t, store.updateParams, 1)
	assert.Equal(t, task.ID, store.updateParams[0].ID)
	assert.Equal(t, result.FireKey, *store.updateParams[0].FireKey)
	require.Len(t, enqueuer.calls, 1)
	assert.Equal(t, result.FireKey, enqueuer.calls[0].fireKey)
}

func TestTaskProcessor_QueuePolicy(t *testing.T) {
	now := time.Now()
	task := domain.ScheduledTask{
		ID:       "queue",
		TaskName: "queue-task",
		Interval: 2 * time.Minute,
	}

	store := &stubTaskStore{
		markResults: []bool{true},
	}
	enqueuer := &stubJobEnqueuer{created: true}

	processor := scheduler.NewTaskProcessor(scheduler.TaskProcessorOptions{
		DefaultPolicy: domain.OverrunPolicyQueue,
		DefaultStates: domain.OverrunStatesDefault,
	})

	result, err := processor.Process(context.Background(), scheduler.ProcessParams{
		Task:     task,
		Now:      now,
		Store:    store,
		Enqueuer: enqueuer,
	})
	require.NoError(t, err)
	require.True(t, result.Enqueued)
	assert.False(t, result.MarkedQueued)
	require.Len(t, store.markParams, 1)
	assert.NotNil(t, store.markParams[0].ActiveFireKey)
	assert.Equal(t, result.FireKey, *store.markParams[0].ActiveFireKey)
	if assert.NotNil(t, store.markParams[0].ActiveFireKeySetAt) {
		assert.True(t, now.Equal(*store.markParams[0].ActiveFireKeySetAt))
	}
}

func TestTaskProcessor_SkipPolicyMissingStateReader(t *testing.T) {
	now := time.Now()
	task := domain.ScheduledTask{
		ID:       "missing-reader",
		TaskName: "alerts",
		Interval: time.Minute,
	}

	store := &stubTaskStore{}
	processor := scheduler.NewTaskProcessor(scheduler.TaskProcessorOptions{})

	_, err := processor.Process(context.Background(), scheduler.ProcessParams{
		Task:  task,
		Now:   now,
		Store: store,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "job state reader is not configured")
}

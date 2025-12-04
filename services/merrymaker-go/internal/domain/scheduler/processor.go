package scheduler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/target/mmk-ui-api/internal/domain"
)

// TaskStore executes scheduler persistence operations within the ambient transaction.
type TaskStore interface {
	MarkQueued(ctx context.Context, params domain.MarkQueuedParams) (bool, error)
	UpdateActiveFireKey(ctx context.Context, params domain.UpdateActiveFireKeyParams) error
}

// JobStateReader reports the current overrun states for a scheduled task.
type JobStateReader interface {
	JobStatesByTaskName(ctx context.Context, taskName string, now time.Time) (domain.OverrunStateMask, error)
}

// JobEnqueuer creates a job for the provided scheduled task using the supplied fire key.
type JobEnqueuer interface {
	Enqueue(ctx context.Context, task domain.ScheduledTask, fireKey string) (bool, error)
}

// TaskProcessorOptions configures TaskProcessor defaults.
type TaskProcessorOptions struct {
	DefaultPolicy domain.OverrunPolicy
	DefaultStates domain.OverrunStateMask
	StateReader   JobStateReader
}

// TaskProcessor owns the overrun policy flow for scheduled tasks.
type TaskProcessor struct {
	defaultPolicy domain.OverrunPolicy
	defaultStates domain.OverrunStateMask
	stateReader   JobStateReader
}

type shouldEnqueueParams struct {
	Task     domain.ScheduledTask
	Strategy taskStrategy
	FireKey  string
	Now      time.Time
}

type finalizeEnqueueParams struct {
	Policy  domain.OverrunPolicy
	TaskID  string
	FireKey string
	Now     time.Time
}

// NewTaskProcessor constructs a TaskProcessor with sane defaults.
func NewTaskProcessor(opts TaskProcessorOptions) *TaskProcessor {
	policy := opts.DefaultPolicy
	if policy == "" {
		policy = domain.OverrunPolicySkip
	}
	states := opts.DefaultStates
	if states == 0 {
		states = domain.OverrunStatesDefault
	}
	return &TaskProcessor{
		defaultPolicy: policy,
		defaultStates: states,
		stateReader:   opts.StateReader,
	}
}

// ProcessParams supplies the per-invocation collaborators for Process.
type ProcessParams struct {
	Task     domain.ScheduledTask
	Now      time.Time
	Store    TaskStore
	Enqueuer JobEnqueuer
}

// ProcessResult captures the outcome of processing a scheduled task.
type ProcessResult struct {
	Worked        bool
	Enqueued      bool
	MarkedQueued  bool
	FireKey       string
	ShouldEnqueue bool
}

// Process evaluates a scheduled task and applies overrun policy updates via the provided collaborators.
func (p *TaskProcessor) Process(ctx context.Context, params ProcessParams) (*ProcessResult, error) {
	if params.Store == nil {
		return nil, errors.New("task store is required")
	}

	now := params.Now
	if now.IsZero() {
		now = time.Now()
	}

	task := params.Task
	result := &ProcessResult{}

	if !isTaskDue(task, now) {
		return result, nil
	}

	return p.processDueTask(ctx, processDueParams{
		Task:     task,
		Store:    params.Store,
		Enqueuer: params.Enqueuer,
		Now:      now,
	})
}

type processDueParams struct {
	Task     domain.ScheduledTask
	Store    TaskStore
	Enqueuer JobEnqueuer
	Now      time.Time
}

func (p *TaskProcessor) processDueTask(ctx context.Context, params processDueParams) (*ProcessResult, error) {
	result := &ProcessResult{}
	strategy := p.resolveStrategy(params.Task)
	fireKey := ComputeFireKey(params.Task, params.Now)
	result.FireKey = fireKey
	shouldEnqueue, err := p.shouldEnqueue(ctx, shouldEnqueueParams{
		Task:     params.Task,
		Strategy: strategy,
		FireKey:  fireKey,
		Now:      params.Now,
	})
	if err != nil {
		return nil, fmt.Errorf("check overrun policy: %w", err)
	}
	result.ShouldEnqueue = shouldEnqueue
	marked, markErr := p.markIfRequired(ctx, params.Store, markIfRequiredParams{
		strategy: strategy,
		markParams: domain.MarkQueuedParams{
			ID:  params.Task.ID,
			Now: params.Now,
		},
	})
	if markErr != nil {
		return nil, markErr
	}
	if marked {
		result.MarkedQueued = true
		result.Worked = true
	}
	if !shouldEnqueue {
		return result, nil
	}
	if params.Enqueuer == nil {
		return nil, errors.New("job enqueuer is required")
	}
	created, enqueueErr := p.enqueueTask(ctx, params.Enqueuer, enqueueTaskParams{
		task:    params.Task,
		fireKey: fireKey,
	})
	if enqueueErr != nil {
		return nil, enqueueErr
	}
	if !created {
		return result, nil
	}
	result.Enqueued = true
	result.Worked = true
	if finalizeErr := p.finalizeEnqueue(ctx, params.Store, finalizeEnqueueParams{
		Policy:  strategy.policy,
		TaskID:  params.Task.ID,
		FireKey: fireKey,
		Now:     params.Now,
	}); finalizeErr != nil {
		return nil, finalizeErr
	}

	return result, nil
}

type markIfRequiredParams struct {
	strategy   taskStrategy
	markParams domain.MarkQueuedParams
}

func (p *TaskProcessor) markIfRequired(
	ctx context.Context,
	store TaskStore,
	params markIfRequiredParams,
) (bool, error) {
	if params.strategy.policy == domain.OverrunPolicyQueue {
		return false, nil
	}

	return p.markQueuedPreEnqueue(ctx, store, params.markParams)
}

type enqueueTaskParams struct {
	task    domain.ScheduledTask
	fireKey string
}

func (p *TaskProcessor) enqueueTask(
	ctx context.Context,
	enqueuer JobEnqueuer,
	params enqueueTaskParams,
) (bool, error) {
	created, err := enqueuer.Enqueue(ctx, params.task, params.fireKey)
	if err != nil {
		return false, fmt.Errorf("enqueue job: %w", err)
	}
	return created, nil
}

type taskStrategy struct {
	policy domain.OverrunPolicy
	states domain.OverrunStateMask
}

func (p *TaskProcessor) resolveStrategy(task domain.ScheduledTask) taskStrategy {
	policy := p.defaultPolicy
	states := p.defaultStates

	if task.OverrunPolicy != nil {
		policy = *task.OverrunPolicy
	}
	if task.OverrunStates != nil {
		if overrides := *task.OverrunStates; overrides != 0 {
			states = overrides
		} else {
			states = domain.OverrunStatesDefault
		}
	}
	if states == 0 {
		states = domain.OverrunStatesDefault
	}

	return taskStrategy{policy: policy, states: states}
}

func (p *TaskProcessor) markQueuedPreEnqueue(
	ctx context.Context,
	store TaskStore,
	params domain.MarkQueuedParams,
) (bool, error) {
	marked, err := store.MarkQueued(ctx, params)
	if err != nil {
		return false, fmt.Errorf("mark task queued: %w", err)
	}
	return marked, nil
}

func (p *TaskProcessor) finalizeEnqueue(ctx context.Context, store TaskStore, params finalizeEnqueueParams) error {
	switch params.Policy {
	case domain.OverrunPolicyQueue:
		setAt := params.Now
		_, markErr := store.MarkQueued(ctx, domain.MarkQueuedParams{
			ID:                 params.TaskID,
			Now:                params.Now,
			ActiveFireKey:      &params.FireKey,
			ActiveFireKeySetAt: &setAt,
		})
		if markErr != nil {
			return fmt.Errorf("mark task queued after enqueue: %w", markErr)
		}
	case domain.OverrunPolicySkip, domain.OverrunPolicyReschedule:
		updateErr := store.UpdateActiveFireKey(ctx, domain.UpdateActiveFireKeyParams{
			ID:      params.TaskID,
			FireKey: &params.FireKey,
			SetAt:   params.Now,
		})
		if updateErr != nil {
			return fmt.Errorf("set active fire key: %w", updateErr)
		}
	default:
		return fmt.Errorf("unknown overrun policy: %s", params.Policy)
	}
	return nil
}

func (p *TaskProcessor) shouldEnqueue(ctx context.Context, params shouldEnqueueParams) (bool, error) {
	switch params.Strategy.policy {
	case domain.OverrunPolicyQueue:
		return true, nil
	case domain.OverrunPolicyReschedule:
		return false, nil
	case domain.OverrunPolicySkip:
		mask := params.Strategy.states
		if mask == 0 {
			mask = domain.OverrunStatesDefault
		}
		if p.stateReader == nil {
			return false, errors.New("job state reader is not configured")
		}

		states, err := p.stateReader.JobStatesByTaskName(ctx, params.Task.TaskName, params.Now)
		if err != nil {
			return false, fmt.Errorf("check job states: %w", err)
		}
		if states&mask != 0 {
			return false, nil
		}
		if params.Task.ActiveFireKey != nil && *params.Task.ActiveFireKey != "" &&
			*params.Task.ActiveFireKey == params.FireKey {
			return false, nil
		}
		return true, nil
	default:
		return false, fmt.Errorf("unknown overrun policy: %s", params.Strategy.policy)
	}
}

func isTaskDue(task domain.ScheduledTask, now time.Time) bool {
	if task.LastQueuedAt == nil {
		return true
	}
	return !task.LastQueuedAt.Add(task.Interval).After(now)
}

// ComputeFireKey derives an idempotent fire key for the provided task at the given time.
func ComputeFireKey(task domain.ScheduledTask, now time.Time) string {
	intervalSec := int64(task.Interval / time.Second)
	if intervalSec <= 0 {
		return fmt.Sprintf("%s:%d", task.ID, now.Unix())
	}
	slot := now.Unix() / intervalSec
	return fmt.Sprintf("%s:%d", task.ID, slot)
}

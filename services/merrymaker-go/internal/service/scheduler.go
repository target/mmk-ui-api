// Package service provides business logic services for the merrymaker job system.
package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
	domainscheduler "github.com/target/mmk-ui-api/internal/domain/scheduler"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
)

// siteSourcePayload represents the payload structure for site run tasks.
type siteSourcePayload struct {
	SiteID   string `json:"site_id"`
	SourceID string `json:"source_id"`
}

// SchedulerService implements the JobScheduler interface.
// It processes due scheduled tasks, applies overrun strategy, enqueues jobs, and updates last_queued_at.
// Safe under concurrent replicas through database-level concurrency controls.
type SchedulerService struct {
	repo         core.ScheduledJobsRepository
	jobs         core.JobRepository
	jobq         core.JobIntrospector
	cfg          core.SchedulerConfig
	timeProvider data.TimeProvider
	sourceCache  *core.SourceCacheService // Optional: for caching source content
	logger       *slog.Logger

	taskProcessor *domainscheduler.TaskProcessor
}

// NewSchedulerService creates a new SchedulerService with the given dependencies.
func NewSchedulerService(opts SchedulerServiceOptions) *SchedulerService {
	if opts.TimeProvider == nil {
		opts.TimeProvider = &data.RealTimeProvider{}
	}
	if opts.Config == nil {
		defaultCfg := core.DefaultSchedulerConfig()
		opts.Config = &defaultCfg
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}

	return &SchedulerService{
		repo:         opts.Repo,
		jobs:         opts.Jobs,
		jobq:         opts.JobIntrospector,
		cfg:          *opts.Config,
		timeProvider: opts.TimeProvider,
		sourceCache:  opts.SourceCache,
		logger:       opts.Logger,
		taskProcessor: domainscheduler.NewTaskProcessor(domainscheduler.TaskProcessorOptions{
			DefaultPolicy: opts.Config.Strategy.Overrun,
			DefaultStates: opts.Config.Strategy.OverrunStates,
			StateReader:   opts.JobIntrospector,
		}),
	}
}

// SchedulerServiceOptions holds the dependencies for creating a SchedulerService.
// Uses an options struct to keep parameter count ≤ 3 as per project conventions.
type SchedulerServiceOptions struct {
	Repo            core.ScheduledJobsRepository
	Jobs            core.JobRepository
	JobIntrospector core.JobIntrospector
	Config          *core.SchedulerConfig
	TimeProvider    data.TimeProvider
	SourceCache     *core.SourceCacheService // Optional: for caching source content
	Logger          *slog.Logger
}

// Tick processes due scheduled tasks and enqueues jobs according to strategy.
// Returns the number of tasks processed.
//
// Algorithm:
// 1. Find due tasks using batch size limit
// 2. For each task, try to acquire advisory lock by task name
// 3. If lock acquired, apply overrun policy and potentially enqueue job
// 4. Update last_queued_at timestamp
//
// Concurrency safety:
// - FindDue uses FOR UPDATE SKIP LOCKED to prevent double-processing
// - TryWithTaskLock uses advisory locks to ensure only one replica processes each task.
func (s *SchedulerService) Tick(ctx context.Context, now time.Time) (int, error) {
	// Find due tasks
	due, err := s.repo.FindDue(ctx, now, s.cfg.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("find due tasks: %w", err)
	}

	processed := 0
	for _, task := range due {
		worked := false
		// Try to acquire advisory lock for this task
		lockOK, lockErr := s.repo.TryWithTaskLock(ctx, task.TaskName, func(ctx context.Context, tx *sql.Tx) error {
			w, processErr := s.processTask(ctx, tx, task)
			if w {
				worked = true
			}
			return processErr
		})
		if lockErr != nil {
			return processed, fmt.Errorf("process task %s: %w", task.TaskName, lockErr)
		}
		if lockOK && worked {
			processed++
		}
		// If ok==false, another replica is handling this task; skip
	}

	return processed, nil
}

// processTask handles a single scheduled task within a transaction.
// Returns worked=true if this invocation actually made a change (updated last_queued_at or created a job).
// This function is called within TryWithTaskLock, so it has exclusive access to the task during execution.
func (s *SchedulerService) processTask(
	ctx context.Context,
	tx *sql.Tx,
	task domain.ScheduledTask,
) (bool, error) {
	now := s.timeProvider.Now()

	if s.taskProcessor == nil {
		return false, errors.New("task processor is not configured")
	}

	result, err := s.taskProcessor.Process(ctx, domainscheduler.ProcessParams{
		Task: task,
		Now:  now,
		Store: taskStoreAdapter{
			repo: s.repo,
			tx:   tx,
		},
		Enqueuer: taskEnqueuer{
			service: s,
			tx:      tx,
		},
	})
	if err != nil {
		return false, err
	}
	if result == nil {
		return false, nil
	}
	return result.Worked, nil
}

type taskStoreAdapter struct {
	repo core.ScheduledJobsRepository
	tx   *sql.Tx
}

func (a taskStoreAdapter) MarkQueued(ctx context.Context, params domain.MarkQueuedParams) (bool, error) {
	return a.repo.MarkQueuedTx(ctx, a.tx, params)
}

func (a taskStoreAdapter) UpdateActiveFireKey(ctx context.Context, params domain.UpdateActiveFireKeyParams) error {
	return a.repo.UpdateActiveFireKeyTx(ctx, a.tx, params)
}

type taskEnqueuer struct {
	service *SchedulerService
	tx      *sql.Tx
}

func (e taskEnqueuer) Enqueue(ctx context.Context, task domain.ScheduledTask, fireKey string) (bool, error) {
	return e.service.enqueueJob(ctx, enqueueJobParams{
		Tx:      e.tx,
		Task:    task,
		FireKey: fireKey,
	})
}

type enqueueJobParams struct {
	Tx      *sql.Tx
	Task    domain.ScheduledTask
	FireKey string
}

// enqueueJob creates a new job for the scheduled task.
// Returns created=true if a new job was inserted (not a duplicate), otherwise false.
func (s *SchedulerService) enqueueJob(ctx context.Context, params enqueueJobParams) (bool, error) {
	task := params.Task
	fireKey := params.FireKey

	// Parse payload to extract site_id and source_id for site runs
	payloadData, parseErr := s.parseTaskPayload(task.Payload)
	if parseErr != nil {
		return false, fmt.Errorf("parse task payload: %w", parseErr)
	}

	// Cache source content for browser jobs if source cache is available
	if cacheErr := s.cacheSourceIfNeeded(ctx, task.Payload); cacheErr != nil {
		return false, fmt.Errorf("cache source content: %w", cacheErr)
	}

	// Prepare job request with metadata and associations
	req, err := s.buildJobRequest(ctx, task, payloadData, fireKey)
	if err != nil {
		return false, fmt.Errorf("build job request: %w", err)
	}

	// Create the job (idempotent via unique fire key)
	created, err := s.createJobWithRetry(ctx, params.Tx, req)
	if err != nil {
		return false, err
	}
	return created, nil
}

// parseTaskPayload extracts site_id and source_id from task payload.
func (s *SchedulerService) parseTaskPayload(payload json.RawMessage) (siteSourcePayload, error) {
	var payloadData siteSourcePayload
	err := json.Unmarshal(payload, &payloadData)
	return payloadData, err
}

// cacheSourceIfNeeded caches source content for browser jobs if caching is enabled.
func (s *SchedulerService) cacheSourceIfNeeded(ctx context.Context, payload json.RawMessage) error {
	if s.cfg.DefaultJobType == model.JobTypeBrowser && s.sourceCache != nil {
		return s.cacheSourceContentFromPayload(ctx, payload)
	}
	return nil
}

// buildJobRequest creates a CreateJobRequest with metadata and associations.
func (s *SchedulerService) buildJobRequest(
	ctx context.Context,
	task domain.ScheduledTask,
	payloadData siteSourcePayload,
	fireKey string,
) (*model.CreateJobRequest, error) {
	meta, err := s.buildSchedulerMetadata(task, fireKey)
	if err != nil {
		return nil, err
	}

	// Determine job type from task name, falling back to default
	jobType := s.cfg.DefaultJobType
	if specificType, ok := determineJobTypeFromTaskName(task.TaskName); ok {
		jobType = specificType
	}

	payload := task.Payload
	if jobType == model.JobTypeBrowser {
		payload = s.resolveBrowserPayload(ctx, task.Payload, payloadData)
	}
	req := s.makeJobRequest(makeJobRequestParams{
		Payload:        payload,
		Meta:           meta,
		SiteSourceData: payloadData,
		JobType:        jobType,
	})
	return req, nil
}

// createJobWithRetry creates a job with idempotency handling.
// Returns created=true if a new job row was inserted; false if it was a duplicate/no-op.
func (s *SchedulerService) createJobWithRetry(
	ctx context.Context,
	tx *sql.Tx,
	req *model.CreateJobRequest,
) (bool, error) {
	err := s.insertJob(ctx, tx, req)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			// Duplicate due to unique fire key; treat as success/no-op
			return false, nil
		}
		return false, fmt.Errorf("create job: %w", err)
	}
	return true, nil
}

func (s *SchedulerService) insertJob(ctx context.Context, tx *sql.Tx, req *model.CreateJobRequest) error {
	if tx == nil {
		_, err := s.jobs.Create(ctx, req)
		return err
	}

	if creator, ok := s.jobs.(core.JobRepositoryTx); ok {
		_, err := creator.CreateInTx(ctx, tx, req)
		return err
	}

	if s.logger != nil {
		s.logger.WarnContext(
			ctx,
			"job repository missing transactional support; falling back to non-transactional create",
		)
	}

	_, err := s.jobs.Create(ctx, req)
	return err
}

// cacheSourceContentFromPayload extracts source_id from task payload and caches the source content.
func (s *SchedulerService) cacheSourceContentFromPayload(ctx context.Context, payload json.RawMessage) error {
	// Parse payload to extract source_id
	var p struct {
		SourceID string `json:"source_id"`
	}
	if err := json.Unmarshal(payload, &p); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}

	if p.SourceID == "" {
		return nil // No source ID to cache
	}

	// Cache the source content
	return s.sourceCache.CacheSourceContent(ctx, p.SourceID)
}

// buildSchedulerMetadata prepares scheduler metadata with idempotent fire key.
func (s *SchedulerService) buildSchedulerMetadata(task domain.ScheduledTask, fireKey string) (json.RawMessage, error) {
	m := map[string]any{
		"scheduler.task_name": task.TaskName,
		"scheduler.interval":  task.Interval.String(),
		"scheduler.fire_key":  fireKey,
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}
	return json.RawMessage(b), nil
}

// resolveBrowserPayload resolves a browser job payload, preserving script/url if present,
// otherwise attempting to fetch script content via the optional source cache.
func (s *SchedulerService) resolveBrowserPayload(
	ctx context.Context,
	schedPayload json.RawMessage,
	p siteSourcePayload,
) json.RawMessage {
	if pl, ok := tryExistingBrowserPayload(schedPayload, p); ok {
		return pl
	}
	if pl, ok := s.tryCacheBrowserPayload(ctx, p); ok {
		return pl
	}
	return schedPayload
}

// tryExistingBrowserPayload returns an existing browser payload with context attached if present.
func tryExistingBrowserPayload(schedPayload json.RawMessage, p siteSourcePayload) (json.RawMessage, bool) {
	var candidate map[string]any
	if err := json.Unmarshal(schedPayload, &candidate); err != nil || candidate == nil {
		return nil, false
	}
	if _, ok := candidate["script"]; ok {
		pl, err := serializeWithContext(candidate, p)
		if err != nil {
			return nil, false
		}
		return pl, true
	}
	if _, ok := candidate["url"]; ok {
		pl, err := serializeWithContext(candidate, p)
		if err != nil {
			return nil, false
		}
		return pl, true
	}
	return nil, false
}

// tryCacheBrowserPayload attempts to resolve a browser payload from the source cache.
func (s *SchedulerService) tryCacheBrowserPayload(ctx context.Context, p siteSourcePayload) (json.RawMessage, bool) {
	if s.sourceCache == nil || p.SourceID == "" {
		return nil, false
	}
	b, getErr := s.sourceCache.GetCachedSourceContent(ctx, p.SourceID)
	if getErr != nil {
		s.logger.WarnContext(
			ctx,
			"scheduler: get cached source content failed",
			"error",
			getErr,
			"source_id",
			p.SourceID,
		)
	}
	if len(b) == 0 {
		b = s.refreshSourceCache(ctx, p.SourceID)
	}
	if len(b) == 0 {
		return nil, false
	}
	mp := map[string]any{"script": string(b)}
	pl, err := serializeWithContext(mp, p)
	if err != nil {
		return nil, false
	}
	return pl, true
}

// serializeWithContext attaches site/source IDs (when present) and marshals to JSON.
func serializeWithContext(bp map[string]any, p siteSourcePayload) (json.RawMessage, error) {
	if p.SiteID != "" {
		bp["site_id"] = p.SiteID
	}
	if p.SourceID != "" {
		bp["source_id"] = p.SourceID
	}
	b, err := json.Marshal(bp)
	if err != nil {
		return nil, fmt.Errorf("marshal browser payload: %w", err)
	}
	return json.RawMessage(b), nil
}

func (s *SchedulerService) refreshSourceCache(ctx context.Context, sourceID string) []byte {
	if err := s.sourceCache.CacheSourceContent(ctx, sourceID); err != nil {
		s.logger.WarnContext(ctx, "scheduler: cache source content failed", "error", err, "source_id", sourceID)
		return nil
	}

	b, err := s.sourceCache.GetCachedSourceContent(ctx, sourceID)
	if err != nil {
		s.logger.WarnContext(
			ctx,
			"scheduler: second get cached source content failed",
			"error",
			err,
			"source_id",
			sourceID,
		)
		return nil
	}

	return b
}

// makeJobRequestParams groups parameters for makeJobRequest.
type makeJobRequestParams struct {
	Payload        json.RawMessage
	Meta           json.RawMessage
	SiteSourceData siteSourcePayload
	JobType        model.JobType
}

// makeJobRequest constructs the final CreateJobRequest and applies associations.
func (s *SchedulerService) makeJobRequest(params makeJobRequestParams) *model.CreateJobRequest {
	req := &model.CreateJobRequest{
		Type:       params.JobType,
		Priority:   s.cfg.DefaultPriority,
		Payload:    params.Payload,
		Metadata:   params.Meta,
		MaxRetries: s.cfg.MaxRetries,
		IsTest:     false,
	}
	if params.SiteSourceData.SiteID != "" {
		if id, err := uuid.Parse(params.SiteSourceData.SiteID); err == nil {
			siteIDStr := id.String()
			req.SiteID = &siteIDStr
		}
	}
	if params.SiteSourceData.SourceID != "" {
		if id, err := uuid.Parse(params.SiteSourceData.SourceID); err == nil {
			sourceIDStr := id.String()
			req.SourceID = &sourceIDStr
		}
	}
	return req
}

// determineJobTypeFromTaskName determines the job type based on the task name prefix.
// This allows different types of scheduled tasks to create jobs of the appropriate type.
// Returns the job type and whether a specific type was determined.
func determineJobTypeFromTaskName(taskName string) (model.JobType, bool) {
	// Secret refresh tasks use the prefix "secret-refresh:"
	if strings.HasPrefix(taskName, "secret-refresh:") {
		return model.JobTypeSecretRefresh, true
	}
	// Add other task name prefixes here as needed
	// e.g., "alert-check:" -> JobTypeAlert

	return "", false
}

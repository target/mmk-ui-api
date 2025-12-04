package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	domainjob "github.com/target/mmk-ui-api/internal/domain/job"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/observability/notify"
	"github.com/target/mmk-ui-api/internal/service/failurenotifier"
)

// JobServiceOptions groups dependencies for JobService.
type JobServiceOptions struct {
	Repo            core.JobRepository        // Required: job repository
	DefaultLease    time.Duration             // Required: default lease duration for jobs
	Logger          *slog.Logger              // Optional: structured logger
	FailureNotifier *failurenotifier.Service  // Optional: failure notification fan-out
	LeasePolicy     *domainjob.LeasePolicy    // Optional: override default lease policy
	Notifier        domainjob.Notifier        // Optional: custom job availability notifier
	NotifierOptions domainjob.NotifierOptions // Optional: configure default notifier behaviour
}

// JobService provides business logic for job operations including pub/sub notifications.
//
// This service manages:
// - CRUD operations for jobs
// - Job reservation and lease management
// - Pub/sub notification system for job availability
// - Goroutine management for background listeners
// - Graceful shutdown of all listeners.
type JobService struct {
	repo            core.JobRepository
	leasePolicy     *domainjob.LeasePolicy
	notifier        domainjob.Notifier
	logger          *slog.Logger
	failureNotifier *failurenotifier.Service
}

// NewJobService constructs a new JobService.
func NewJobService(opts JobServiceOptions) (*JobService, error) {
	if opts.Repo == nil {
		return nil, errors.New("JobRepository is required")
	}

	var leasePolicy *domainjob.LeasePolicy
	switch {
	case opts.LeasePolicy != nil:
		leasePolicy = opts.LeasePolicy
	case opts.DefaultLease > 0:
		var err error
		leasePolicy, err = domainjob.NewLeasePolicy(opts.DefaultLease)
		if err != nil {
			return nil, fmt.Errorf("create lease policy: %w", err)
		}
	default:
		return nil, errors.New("DefaultLease must be positive")
	}

	notifier := opts.Notifier
	if notifier == nil {
		options := opts.NotifierOptions
		if options.Waiter == nil {
			options.Waiter = opts.Repo
		}
		var err error
		notifier, err = domainjob.NewNotifier(options)
		if err != nil {
			return nil, fmt.Errorf("create job notifier: %w", err)
		}
	}

	var logger *slog.Logger
	if opts.Logger != nil {
		logger = opts.Logger.With("component", "job_service")
		logger.Debug("JobService initialized",
			"default_lease", leasePolicy.Default(),
		)
	}

	return &JobService{
		repo:            opts.Repo,
		leasePolicy:     leasePolicy,
		notifier:        notifier,
		logger:          logger,
		failureNotifier: opts.FailureNotifier,
	}, nil
}

// MustNewJobService constructs a new JobService and panics on error.
// Use this when you're certain the options are valid (e.g., in main.go).
func MustNewJobService(opts JobServiceOptions) *JobService {
	svc, err := NewJobService(opts)
	if err != nil {
		//nolint:forbidigo // Must constructor fails fast when dependencies are invalid during startup
		panic(fmt.Sprintf("failed to create JobService: %v", err))
	}
	return svc
}

// Create creates a new job with the given request parameters.
func (s *JobService) Create(ctx context.Context, req *model.CreateJobRequest) (*model.Job, error) {
	job, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create job: %w", err)
	}

	if s.logger != nil {
		s.logger.DebugContext(
			ctx,
			"job created",
			"id",
			job.ID,
			"type",
			job.Type,
			"status",
			job.Status,
		)
	}

	return job, nil
}

// ReserveNext reserves the next available job of the given type for processing.
func (s *JobService) ReserveNext(
	ctx context.Context,
	jobType model.JobType,
	lease time.Duration,
) (*model.Job, error) {
	decision := s.leasePolicy.Resolve(lease)
	if decision.Clamped() && s.logger != nil {
		s.logger.DebugContext(ctx, "clamped sub-second lease duration to 1 second",
			"requested_duration", decision.Requested,
			"job_type", jobType)
	}

	job, err := s.repo.ReserveNext(ctx, jobType, decision.Seconds)
	if err != nil {
		return nil, fmt.Errorf("reserve next job: %w", err)
	}

	if s.logger != nil && job != nil {
		s.logger.DebugContext(
			ctx,
			"job reserved",
			"id",
			job.ID,
			"type",
			jobType,
			"lease_seconds",
			decision.Seconds,
		)
	}

	return job, nil
}

// Subscribe creates a subscription for job notifications of the given type.
// Returns an unsubscribe function and a channel that receives notifications.
func (s *JobService) Subscribe(jobType model.JobType) (func(), <-chan struct{}) {
	if s.notifier == nil {
		ch := make(chan struct{})
		close(ch)
		return func() {}, ch
	}
	return s.notifier.Subscribe(jobType)
}

// WaitForNotification waits for a notification indicating new jobs are available.
func (s *JobService) WaitForNotification(ctx context.Context, jobType model.JobType) error {
	return s.repo.WaitForNotification(ctx, jobType)
}

// Heartbeat extends the lease on a job to indicate it's still being processed.
func (s *JobService) Heartbeat(ctx context.Context, id string, extend time.Duration) (bool, error) {
	decision := s.leasePolicy.Resolve(extend)
	if decision.Clamped() && s.logger != nil {
		s.logger.DebugContext(ctx, "clamped sub-second heartbeat duration to 1 second",
			"requested_duration", decision.Requested,
			"job_id", id)
	}

	updated, err := s.repo.Heartbeat(ctx, id, decision.Seconds)
	if err != nil {
		return false, fmt.Errorf("heartbeat job %s: %w", id, err)
	}

	if s.logger != nil && updated {
		s.logger.DebugContext(ctx, "job heartbeat updated", "id", id, "extend_seconds", decision.Seconds)
	}

	return updated, nil
}

// Complete marks a job as completed successfully.
func (s *JobService) Complete(ctx context.Context, id string) (bool, error) {
	completed, err := s.repo.Complete(ctx, id)
	if err != nil {
		return false, fmt.Errorf("complete job %s: %w", id, err)
	}

	if s.logger != nil && completed {
		s.logger.DebugContext(ctx, "job completed", "id", id)
	}

	return completed, nil
}

// Fail marks a job as failed with the given error message.
func (s *JobService) Fail(ctx context.Context, id, errMsg string) (bool, error) {
	return s.FailWithDetails(ctx, id, errMsg, JobFailureDetails{})
}

// JobFailureDetails captures optional context for failure notifications.
type JobFailureDetails struct {
	Scope      string
	ErrorClass string
	Metadata   map[string]string
	Severity   string
	OccurredAt time.Time
}

// FailWithDetails marks a job as failed and propagates optional metadata to the notifier.
func (s *JobService) FailWithDetails(
	ctx context.Context,
	id, errMsg string,
	details JobFailureDetails,
) (bool, error) {
	if errMsg == "" {
		return false, errors.New("error message required")
	}

	var job *model.Job
	if s.failureNotifier != nil {
		var err error
		job, err = s.repo.GetByID(ctx, id)
		if err != nil && s.logger != nil {
			s.logger.WarnContext(ctx, "failed to load job for failure notification", "job_id", id, "error", err)
		}
	}

	failed, err := s.repo.Fail(ctx, id, errMsg)
	if err != nil {
		return false, fmt.Errorf("fail job %s: %w", id, err)
	}

	if s.logger != nil && failed {
		s.logger.DebugContext(ctx, "job failed", "id", id, "error", errMsg)
	}

	if failed && s.failureNotifier != nil {
		payload := buildJobFailurePayload(jobFailurePayloadInput{
			ID:      id,
			Job:     job,
			ErrMsg:  errMsg,
			Details: details,
		})
		s.failureNotifier.NotifyJobFailure(ctx, payload)
	}

	return failed, nil
}

type jobFailurePayloadInput struct {
	ID      string
	Job     *model.Job
	ErrMsg  string
	Details JobFailureDetails
}

func buildJobFailurePayload(input jobFailurePayloadInput) notify.JobFailurePayload {
	payload := baseFailurePayload(input.ID, input.ErrMsg, input.Details)
	payload.Scope = deriveFailureScope(input.Details.Scope, input.Job)
	if input.Job != nil {
		applyJobContext(&payload, input.Job)
	}
	if payload.ErrorClass != "" {
		payload.Metadata = mergeMetadata(payload.Metadata, map[string]string{
			"error_class": payload.ErrorClass,
		})
	}

	if len(payload.Metadata) == 0 {
		payload.Metadata = nil
	}

	return payload
}

func baseFailurePayload(id, errMsg string, details JobFailureDetails) notify.JobFailurePayload {
	payload := notify.JobFailurePayload{
		JobID:      id,
		Error:      errMsg,
		ErrorClass: details.ErrorClass,
		Severity:   details.Severity,
		OccurredAt: details.OccurredAt,
		Metadata:   copyMetadata(details.Metadata),
	}

	if payload.Severity == "" {
		payload.Severity = notify.SeverityCritical
	}
	if payload.OccurredAt.IsZero() {
		payload.OccurredAt = time.Now()
	}

	return payload
}

func deriveFailureScope(explicit string, job *model.Job) string {
	if explicit != "" {
		return explicit
	}
	return extractScopeFromJob(job)
}

func applyJobContext(payload *notify.JobFailurePayload, job *model.Job) {
	payload.JobType = string(job.Type)
	payload.IsTest = job.IsTest
	if job.SiteID != nil {
		payload.SiteID = *job.SiteID
	}

	newRetryCount := job.RetryCount + 1
	if newRetryCount < 0 {
		newRetryCount = 0
	}

	finalStatus := model.JobStatusPending
	switch {
	case job.MaxRetries == 0:
		finalStatus = model.JobStatusFailed
	case newRetryCount >= job.MaxRetries:
		finalStatus = model.JobStatusFailed
	}

	metadata := map[string]string{
		"retry_count": strconv.Itoa(newRetryCount),
		"max_retries": strconv.Itoa(job.MaxRetries),
		"priority":    strconv.Itoa(job.Priority),
		"status":      string(finalStatus),
	}
	payload.Metadata = mergeMetadata(payload.Metadata, metadata)
}

func extractScopeFromJob(job *model.Job) string {
	if job == nil || len(job.Payload) == 0 {
		return ""
	}
	if job.Type == model.JobTypeRules {
		var payload RulesJobPayload
		if err := json.Unmarshal(job.Payload, &payload); err == nil {
			return payload.Scope
		}
	}
	return ""
}

func copyMetadata(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		if strings.TrimSpace(k) == "" {
			continue
		}
		if strings.TrimSpace(v) == "" {
			continue
		}
		dst[k] = v
	}
	return dst
}

func mergeMetadata(base, extra map[string]string) map[string]string {
	out := copyMetadata(base)
	if out == nil && len(extra) == 0 {
		return nil
	}
	if out == nil {
		out = make(map[string]string, len(extra))
	}
	for k, v := range extra {
		key := strings.TrimSpace(k)
		val := strings.TrimSpace(v)
		if key == "" || val == "" {
			continue
		}
		out[key] = val
	}
	return out
}

// Stats returns statistics about jobs of the given type in different states.
func (s *JobService) Stats(ctx context.Context, jobType model.JobType) (*model.JobStats, error) {
	stats, err := s.repo.Stats(ctx, jobType)
	if err != nil {
		return nil, fmt.Errorf("get job stats for type %s: %w", jobType, err)
	}
	return stats, nil
}

// GetStatus returns the status information for a specific job.
func (s *JobService) GetStatus(ctx context.Context, id string) (*model.JobStatusResponse, error) {
	job, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get job %s: %w", id, err)
	}

	return &model.JobStatusResponse{
		Status:      job.Status,
		CompletedAt: job.CompletedAt,
		LastError:   job.LastError,
	}, nil
}

// GetByID returns a job by its ID.
func (s *JobService) GetByID(ctx context.Context, id string) (*model.Job, error) {
	job, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get job by id %s: %w", id, err)
	}
	return job, nil
}

// ListRecentByType returns the most recent jobs of the given type.
// This uses an optional repository extension if available; otherwise returns an empty list.
func (s *JobService) ListRecentByType(
	ctx context.Context,
	jobType model.JobType,
	limit int,
) ([]*model.Job, error) {
	type lister interface {
		ListRecentByType(
			ctx context.Context,
			jobType model.JobType,
			limit int,
		) ([]*model.Job, error)
	}
	if lr, ok := s.repo.(lister); ok {
		jobs, err := lr.ListRecentByType(ctx, jobType, limit)
		if err != nil {
			return nil, fmt.Errorf("list recent jobs by type %s: %w", jobType, err)
		}
		return jobs, nil
	}

	if s.logger != nil {
		s.logger.DebugContext(ctx,
			"repository does not support ListRecentByType, returning empty list",
			"type",
			jobType,
		)
	}
	return []*model.Job{}, nil
}

// ListRecentByTypeWithSiteNames returns the most recent jobs of the given type with site names.
// This uses an optional repository extension if available; otherwise falls back to ListRecentByType
// and maps results to JobWithEventCount with empty site names.
// The repository implementation uses a JOIN query to eliminate N+1 lookups.
func (s *JobService) ListRecentByTypeWithSiteNames(
	ctx context.Context,
	jobType model.JobType,
	limit int,
) ([]*model.JobWithEventCount, error) {
	type lister interface {
		ListRecentByTypeWithSiteNames(
			ctx context.Context,
			jobType model.JobType,
			limit int,
		) ([]*model.JobWithEventCount, error)
	}
	if lr, ok := s.repo.(lister); ok {
		jobs, err := lr.ListRecentByTypeWithSiteNames(ctx, jobType, limit)
		if err != nil {
			return nil, fmt.Errorf("list recent jobs by type with site names %s: %w", jobType, err)
		}
		return jobs, nil
	}

	// Fallback: use ListRecentByType and map to JobWithEventCount
	if s.logger != nil {
		s.logger.DebugContext(
			ctx,
			"repository does not support ListRecentByTypeWithSiteNames, falling back to ListRecentByType",
			"type",
			jobType,
		)
	}

	jobs, err := s.ListRecentByType(ctx, jobType, limit)
	if err != nil {
		return nil, fmt.Errorf("fallback list recent jobs by type %s: %w", jobType, err)
	}

	result := make([]*model.JobWithEventCount, len(jobs))
	for i, j := range jobs {
		result[i] = &model.JobWithEventCount{
			Job:        *j,
			EventCount: 0,
			SiteName:   "",
		}
	}
	return result, nil
}

// paginationParams holds normalized pagination parameters.
type paginationParams struct {
	Limit  int
	Offset int
}

// normalizePagination clamps pagination parameters to safe defaults.
// Default limit: 50, max limit: 1000, min offset: 0.
func normalizePagination(limit, offset int) paginationParams {
	if limit <= 0 {
		limit = 50
	}
	if limit > 1000 {
		limit = 1000
	}
	if offset < 0 {
		offset = 0
	}
	return paginationParams{Limit: limit, Offset: offset}
}

// ListBySource returns jobs for a given source with pagination.
// This uses an optional repository extension if available; otherwise returns an empty list.
// Pagination defaults are normalized here to avoid drift across layers.
func (s *JobService) ListBySource(
	ctx context.Context,
	opts model.JobListBySourceOptions,
) ([]*model.Job, error) {
	if opts.SourceID == "" {
		return nil, errors.New("source id is required")
	}

	// Clamp/default pagination like EventService.ListByJob
	p := normalizePagination(opts.Limit, opts.Offset)
	opts.Limit = p.Limit
	opts.Offset = p.Offset

	type lister interface {
		ListBySource(ctx context.Context, opts model.JobListBySourceOptions) ([]*model.Job, error)
	}
	if lr, ok := s.repo.(lister); ok {
		jobs, err := lr.ListBySource(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list jobs by source %s: %w", opts.SourceID, err)
		}
		return jobs, nil
	}

	if s.logger != nil {
		s.logger.DebugContext(ctx,
			"repository does not support ListBySource, returning empty list",
			"source_id",
			opts.SourceID,
		)
	}
	return []*model.Job{}, nil
}

// ListBySiteWithFilters returns jobs with optional filters and event counts.
// This uses an optional repository extension if available; otherwise returns an empty list.
// Pagination defaults are normalized here to avoid drift across layers.
func (s *JobService) ListBySiteWithFilters(
	ctx context.Context,
	opts model.JobListBySiteOptions,
) ([]*model.JobWithEventCount, error) {
	// Clamp/default pagination
	p := normalizePagination(opts.Limit, opts.Offset)
	opts.Limit = p.Limit
	opts.Offset = p.Offset

	type filteredLister interface {
		ListBySiteWithFilters(
			ctx context.Context,
			opts model.JobListBySiteOptions,
		) ([]*model.JobWithEventCount, error)
	}
	if lr, ok := s.repo.(filteredLister); ok {
		jobs, err := lr.ListBySiteWithFilters(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list jobs by site with filters: %w", err)
		}
		return jobs, nil
	}

	if s.logger != nil {
		s.logger.DebugContext(
			ctx,
			"repository does not support ListBySiteWithFilters, returning empty list",
		)
	}
	return []*model.JobWithEventCount{}, nil
}

// List returns all jobs with optional filtering and event counts for admin view.
// This uses an optional repository extension if available; otherwise returns an empty list.
// Pagination defaults are normalized here to avoid drift across layers.
func (s *JobService) List(
	ctx context.Context,
	opts *model.JobListOptions,
) ([]*model.JobWithEventCount, error) {
	// Clamp/default pagination
	p := normalizePagination(opts.Limit, opts.Offset)
	opts.Limit = p.Limit
	opts.Offset = p.Offset

	type lister interface {
		List(ctx context.Context, opts *model.JobListOptions) ([]*model.JobWithEventCount, error)
	}
	if lr, ok := s.repo.(lister); ok {
		jobs, err := lr.List(ctx, opts)
		if err != nil {
			return nil, fmt.Errorf("list jobs: %w", err)
		}
		return jobs, nil
	}

	if s.logger != nil {
		s.logger.DebugContext(ctx, "repository does not support List, returning empty list")
	}
	return []*model.JobWithEventCount{}, nil
}

// Delete safely deletes a job by ID with state machine safety checks.
// Only jobs in pending status without an active lease can be deleted.
// Returns an error if the job cannot be deleted due to state constraints.
func (s *JobService) Delete(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("job id is required")
	}

	if s.logger != nil {
		s.logger.DebugContext(ctx, "attempting to delete job", "id", id)
	}

	err := s.repo.Delete(ctx, id)
	if err != nil {
		if s.logger != nil {
			s.logger.DebugContext(ctx, "failed to delete job", "id", id, "error", err)
		}
		return fmt.Errorf("delete job %s: %w", id, err)
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx, "job deleted successfully", "id", id)
	}

	return nil
}

// StopAllListeners stops all active job notification listeners.
// This should be called during graceful shutdown to clean up goroutines.
func (s *JobService) StopAllListeners() {
	if s.logger != nil {
		s.logger.Info("stopping all job listeners")
	}

	if s.notifier != nil {
		s.notifier.StopAll()
	}
}

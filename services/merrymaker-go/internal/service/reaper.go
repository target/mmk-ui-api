package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	obserrors "github.com/target/mmk-ui-api/internal/observability/errors"
	"github.com/target/mmk-ui-api/internal/observability/metrics"
	"github.com/target/mmk-ui-api/internal/observability/statsd"
)

// ReaperServiceOptions groups dependencies for ReaperService.
type ReaperServiceOptions struct {
	Repo    core.ReaperRepository // Required: reaper repository
	Config  config.ReaperConfig   // Required: reaper configuration
	Logger  *slog.Logger          // Optional: structured logger
	Metrics statsd.Sink           // Optional: metrics sink (StatsD-compatible)
}

// ReaperService provides job cleanup operations.
//
// This service manages:
// - Failing stale pending jobs that were never picked up.
// - Deleting old completed jobs to prevent database bloat.
// - Deleting old failed jobs to prevent database bloat.
type ReaperService struct {
	repo    core.ReaperRepository
	config  config.ReaperConfig
	logger  *slog.Logger
	metrics statsd.Sink
}

// NewReaperService constructs a new ReaperService.
func NewReaperService(opts ReaperServiceOptions) (*ReaperService, error) {
	if opts.Repo == nil {
		return nil, errors.New("ReaperRepository is required")
	}

	var logger *slog.Logger
	if opts.Logger != nil {
		logger = opts.Logger.With("component", "reaper_service")
		logger.Debug("ReaperService initialized",
			"interval", opts.Config.Interval,
			"pending_max_age", opts.Config.PendingMaxAge,
			"completed_max_age", opts.Config.CompletedMaxAge,
			"failed_max_age", opts.Config.FailedMaxAge,
		)
	}

	return &ReaperService{
		repo:    opts.Repo,
		config:  opts.Config,
		logger:  logger,
		metrics: opts.Metrics,
	}, nil
}

// MustNewReaperService constructs a new ReaperService and panics on error.
// Use this when you're certain the options are valid (e.g., in main.go).
func MustNewReaperService(opts ReaperServiceOptions) (*ReaperService, error) {
	svc, err := NewReaperService(opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create ReaperService: %w", err)
	}
	return svc, nil
}

// Run starts the reaper loop and runs until the context is cancelled.
// It performs cleanup operations at the configured interval.
// Returns nil on graceful shutdown (context.Canceled), error otherwise.
func (s *ReaperService) Run(ctx context.Context) error {
	if s.logger != nil {
		s.logger.InfoContext(ctx, "starting reaper service", "interval", s.config.Interval)
	}

	// Add jitter to prevent thundering herd if multiple instances start together
	s.waitWithJitter(ctx)

	ticker := time.NewTicker(s.config.Interval)
	defer ticker.Stop()

	// Run cleanup immediately after jitter
	if err := s.runCleanup(ctx); err != nil {
		s.logCleanupError(err, "initial cleanup")
	}

	return s.runLoop(ctx, ticker)
}

// waitWithJitter adds a random delay up to 10% of the interval to prevent thundering herd.
func (s *ReaperService) waitWithJitter(ctx context.Context) {
	maxJitter := int64(s.config.Interval / 10)
	if maxJitter <= 0 {
		return
	}

	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		// If crypto/rand fails, skip jitter rather than failing startup
		if s.logger != nil {
			s.logger.WarnContext(ctx, "failed to generate jitter, skipping", "error", err)
		}
		return
	}

	// Use modulo on uint64 before converting to avoid overflow
	jitterNanos := binary.BigEndian.Uint64(buf[:]) % uint64(maxJitter)
	jitter := time.Duration(int64(jitterNanos)) // #nosec G115 - bounded by maxJitter which is int64

	select {
	case <-time.After(jitter):
	case <-ctx.Done():
		// Graceful shutdown during jitter
	}
}

// runLoop runs the cleanup loop until context is cancelled.
func (s *ReaperService) runLoop(ctx context.Context, ticker *time.Ticker) error {
	for {
		select {
		case <-ctx.Done():
			if s.logger != nil {
				s.logger.InfoContext(ctx, "reaper service stopping", "reason", ctx.Err())
			}
			// Return nil on graceful shutdown to avoid treating it as a failure
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()

		case <-ticker.C:
			if err := s.runCleanup(ctx); err != nil {
				s.logCleanupError(err, "cleanup")
				if isContextCancellation(err) {
					continue
				}
				// Continue running despite errors
			}
		}
	}
}

// runCleanup performs all cleanup operations.
func (s *ReaperService) runCleanup(ctx context.Context) error {
	start := time.Now()
	var (
		errs               []error
		allContextCanceled = true
		metricsData        = cleanupMetrics{}
	)

	steps := []cleanupStep{
		{
			fn:        s.failStalePendingJobs,
			label:     "fail stale pending jobs",
			count:     &metricsData.PendingCount,
			metricErr: &metricsData.PendingErr,
		},
		{
			fn:        s.deleteOldCompletedJobs,
			label:     "delete old completed jobs",
			count:     &metricsData.CompletedCount,
			metricErr: &metricsData.CompletedErr,
		},
		{
			fn:        s.deleteOldFailedJobs,
			label:     "delete old failed jobs",
			count:     &metricsData.FailedCount,
			metricErr: &metricsData.FailedErr,
		},
		{
			fn:        s.deleteOldJobResults,
			label:     "delete old job results",
			count:     &metricsData.JobResultsCount,
			metricErr: &metricsData.JobResultsErr,
		},
	}

	for _, step := range steps {
		outcome := s.executeCleanupStep(ctx, step.fn, step.label)
		*step.count = outcome.count
		*step.metricErr = outcome.metricErr
		if outcome.aggregateErr != nil {
			errs = append(errs, outcome.aggregateErr)
			allContextCanceled = allContextCanceled && outcome.canceled
		}
	}

	metricsData.Elapsed = time.Since(start)
	s.emitCleanupMetrics(metricsData)

	if len(errs) > 0 {
		joined := errors.Join(errs...)
		if allContextCanceled && isContextCancellation(joined) {
			return context.Canceled
		}
		return fmt.Errorf("cleanup failed: %w", joined)
	}

	return nil
}

type cleanupFunc func(context.Context) (int64, error)

type cleanupStep struct {
	fn        cleanupFunc
	label     string
	count     *int64
	metricErr *error
}

type cleanupStepOutcome struct {
	count        int64
	metricErr    error
	aggregateErr error
	canceled     bool
}

func (s *ReaperService) executeCleanupStep(
	ctx context.Context,
	fn cleanupFunc,
	label string,
) cleanupStepOutcome {
	count, err := fn(ctx)
	outcome := cleanupStepOutcome{
		count:     count,
		metricErr: suppressContextCancellation(err),
		canceled:  isContextCancellation(err),
	}
	if err != nil {
		outcome.aggregateErr = fmt.Errorf("%s: %w", label, err)
	}
	return outcome
}

// failStalePendingJobs marks pending jobs older than the configured max age as failed.
// Loops until no more rows are affected to handle large datasets in batches.
func (s *ReaperService) failStalePendingJobs(ctx context.Context) (int64, error) {
	var totalCount int64
	for {
		count, err := s.repo.FailStalePendingJobs(ctx, s.config.PendingMaxAge, s.config.BatchSize)
		if err != nil {
			return totalCount, err
		}
		totalCount += count
		if count == 0 {
			break
		}
		// Check context between batches
		if ctx.Err() != nil {
			return totalCount, ctx.Err()
		}
	}

	if totalCount > 0 && s.logger != nil {
		s.logger.InfoContext(ctx, "failed stale pending jobs",
			"count", totalCount,
			"max_age", s.config.PendingMaxAge,
		)
	}

	return totalCount, nil
}

// deleteOldCompletedJobs deletes completed jobs older than the configured max age.
// Loops until no more rows are affected to handle large datasets in batches.
func (s *ReaperService) deleteOldCompletedJobs(ctx context.Context) (int64, error) {
	var totalCount int64
	for {
		count, err := s.repo.DeleteOldJobs(ctx, core.DeleteOldJobsParams{
			Status:    model.JobStatusCompleted,
			MaxAge:    s.config.CompletedMaxAge,
			BatchSize: s.config.BatchSize,
		})
		if err != nil {
			return totalCount, err
		}
		totalCount += count
		if count == 0 {
			break
		}
		// Check context between batches
		if ctx.Err() != nil {
			return totalCount, ctx.Err()
		}
	}

	if totalCount > 0 && s.logger != nil {
		s.logger.InfoContext(ctx, "deleted old completed jobs",
			"count", totalCount,
			"max_age", s.config.CompletedMaxAge,
		)
	}

	return totalCount, nil
}

// deleteOldFailedJobs deletes failed jobs older than the configured max age.
// Loops until no more rows are affected to handle large datasets in batches.
func (s *ReaperService) deleteOldFailedJobs(ctx context.Context) (int64, error) {
	var totalCount int64
	for {
		count, err := s.repo.DeleteOldJobs(ctx, core.DeleteOldJobsParams{
			Status:    model.JobStatusFailed,
			MaxAge:    s.config.FailedMaxAge,
			BatchSize: s.config.BatchSize,
		})
		if err != nil {
			return totalCount, err
		}
		totalCount += count
		if count == 0 {
			break
		}
		// Check context between batches
		if ctx.Err() != nil {
			return totalCount, ctx.Err()
		}
	}

	if totalCount > 0 && s.logger != nil {
		s.logger.InfoContext(ctx, "deleted old failed jobs",
			"count", totalCount,
			"max_age", s.config.FailedMaxAge,
		)
	}

	return totalCount, nil
}

// deleteOldJobResults deletes persisted job_results rows older than the configured max age
// for all job types that persist execution summaries.
func (s *ReaperService) deleteOldJobResults(ctx context.Context) (int64, error) {
	var totalCount int64
	jobTypes := []model.JobType{
		model.JobTypeAlert,
		model.JobTypeRules,
	}

	for _, jobType := range jobTypes {
		var typeCount int64
		for {
			count, err := s.repo.DeleteOldJobResults(ctx, core.DeleteOldJobResultsParams{
				JobType:   jobType,
				MaxAge:    s.config.JobResultsMaxAge,
				BatchSize: s.config.BatchSize,
			})
			if err != nil {
				return totalCount, err
			}
			if count == 0 {
				break
			}
			typeCount += count
			totalCount += count

			if ctx.Err() != nil {
				return totalCount, ctx.Err()
			}
		}

		if typeCount > 0 && s.logger != nil {
			s.logger.InfoContext(ctx, "deleted old job results",
				"job_type", jobType,
				"count", typeCount,
				"max_age", s.config.JobResultsMaxAge,
			)
		}
	}

	return totalCount, nil
}

type cleanupMetrics struct {
	PendingCount    int64
	PendingErr      error
	CompletedCount  int64
	CompletedErr    error
	FailedCount     int64
	FailedErr       error
	JobResultsCount int64
	JobResultsErr   error
	Elapsed         time.Duration
}

func (s *ReaperService) emitCleanupMetrics(m cleanupMetrics) {
	if s.metrics == nil {
		return
	}

	totalCount := m.PendingCount + m.CompletedCount + m.FailedCount + m.JobResultsCount
	firstErr := firstError(m.PendingErr, m.CompletedErr, m.FailedErr, m.JobResultsErr)

	result := metrics.ResultSuccess
	if firstErr != nil {
		result = metrics.ResultError
	} else if totalCount == 0 {
		result = metrics.ResultNoop
	}

	tags := map[string]string{
		"result": result,
	}

	if firstErr != nil {
		if class := obserrors.Classify(firstErr); class != "" {
			tags["error_class"] = class
		}
	}

	s.metrics.Count("reaper.cleanup", 1, tags)

	if m.Elapsed > 0 {
		s.metrics.Timing("reaper.cleanup_duration", m.Elapsed, metrics.CloneTags(tags))
	}

	s.emitCleanupOperationMetric("fail_pending", m.PendingCount, m.PendingErr)
	s.emitCleanupOperationMetric("delete_completed", m.CompletedCount, m.CompletedErr)
	s.emitCleanupOperationMetric("delete_failed", m.FailedCount, m.FailedErr)
	s.emitCleanupOperationMetric("delete_job_results", m.JobResultsCount, m.JobResultsErr)

	if firstErr == nil {
		s.metrics.Gauge("reaper.last_success_epoch", float64(time.Now().Unix()), nil)
	}
}

func (s *ReaperService) emitCleanupOperationMetric(operation string, count int64, err error) {
	if s.metrics == nil {
		return
	}

	result := metrics.ResultSuccess
	if err != nil {
		result = metrics.ResultError
	} else if count == 0 {
		result = metrics.ResultNoop
	}

	tags := map[string]string{
		"operation": operation,
		"result":    result,
	}

	if err != nil {
		if class := obserrors.Classify(err); class != "" {
			tags["error_class"] = class
		}
	}

	s.metrics.Count("reaper.cleanup_operation", 1, tags)

	if err == nil && count > 0 {
		s.metrics.Count("reaper.jobs_processed", count, metrics.CloneTags(tags))
	}
}

func (s *ReaperService) logCleanupError(err error, label string) {
	if err == nil || s.logger == nil {
		return
	}

	if isContextCancellation(err) {
		s.logger.Debug(label+" cancelled by context", "error", err)
		return
	}

	s.logger.Error(label+" failed", "error", err)
}

func firstError(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func isContextCancellation(err error) bool {
	if err == nil {
		return false
	}
	return errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
}

func suppressContextCancellation(err error) error {
	if isContextCancellation(err) {
		return nil
	}
	return err
}

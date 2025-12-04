// Package rulesrunner provides a job runner adapter for processing rules engine jobs.
package rulesrunner

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	domainrules "github.com/target/mmk-ui-api/internal/domain/rules"
	obserrors "github.com/target/mmk-ui-api/internal/observability/errors"
	"github.com/target/mmk-ui-api/internal/observability/metrics"
	"github.com/target/mmk-ui-api/internal/observability/statsd"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/service/failurenotifier"
	"github.com/target/mmk-ui-api/internal/service/rules"
	"golang.org/x/sync/errgroup"
)

// RunnerOptions configures the rules job runner adapter.
type RunnerOptions struct {
	DB          *sql.DB
	RedisClient redis.UniversalClient
	Logger      *slog.Logger

	// Job processing settings
	Lease       time.Duration // per-job lease duration; defaults to 30s
	Concurrency int           // number of worker goroutines; defaults to 1

	// Optional dependency injections (useful for tests/decoupling)
	JobsRepo        core.JobRepository
	EventsRepo      core.EventRepository
	AlertRepo       core.AlertRepository
	SeenRepo        core.SeenDomainRepository
	IOCsRepo        core.IOCRepository
	FilesRepo       core.ProcessedFileRepository
	CacheRepo       core.CacheRepository
	AllowlistRepo   core.DomainAllowlistRepository
	AlertSinkRepo   core.HTTPAlertSinkRepository
	SiteRepo        core.SiteRepository
	JobResultRepo   core.JobResultRepository
	CacheMetrics    rules.CacheMetrics
	Metrics         statsd.Sink
	FailureNotifier *failurenotifier.Service
}

// Runner processes rules jobs using the rules orchestration service.
type Runner struct {
	orchestrator *service.RulesOrchestrationService
	jobs         *service.JobService
	logger       *slog.Logger
	lease        time.Duration
	workers      int
	metrics      statsd.Sink
}

// NewRunner creates a new rules job runner with the given options.
func NewRunner(opts RunnerOptions) (*Runner, error) {
	logger := resolveLogger(opts.Logger)

	// Resolve dependencies
	deps := resolveDependencies(opts)
	if err := validateDependencies(opts, deps); err != nil {
		return nil, err
	}

	// Build rules caches
	caches := buildRulesCaches(deps, opts.CacheMetrics)

	// Create alert service with dispatcher
	alerter, err := createAlertService(deps, logger)
	if err != nil {
		return nil, fmt.Errorf("create alert service: %w", err)
	}

	// Create rule evaluators
	unknownDomainEvaluator := createUnknownDomainEvaluator(caches, alerter, deps.allowlistRepo, logger)
	iocEvaluator := createIOCEvaluator(caches, alerter)

	// Create orchestration service
	orchestrator := service.NewRulesOrchestrationService(service.RulesOrchestrationOptions{
		Events:       deps.eventsRepo,
		Jobs:         deps.jobsRepo,
		Sites:        deps.siteRepo,
		Logger:       logger,
		BatchSize:    100,
		DedupeCache:  deps.cacheRepo,
		DedupeTTL:    2 * time.Minute,
		JobResults:   deps.jobResultRepo,
		IOCEvaluator: iocEvaluator,
		Rules:        buildRuleset(unknownDomainEvaluator, iocEvaluator),
	})

	// Create job service
	jobService := service.MustNewJobService(service.JobServiceOptions{
		Repo:            deps.jobsRepo,
		DefaultLease:    resolveLease(opts.Lease),
		FailureNotifier: opts.FailureNotifier,
	})

	return &Runner{
		orchestrator: orchestrator,
		jobs:         jobService,
		logger:       logger,
		lease:        resolveLease(opts.Lease),
		workers:      resolveWorkers(opts.Concurrency),
		metrics:      opts.Metrics,
	}, nil
}

// Run starts the rules job runner and processes jobs until the context is cancelled.
func (r *Runner) Run(ctx context.Context) error {
	r.logger.InfoContext(ctx, "starting rules job runner", "workers", r.workers, "lease", r.lease)

	group, gctx := errgroup.WithContext(ctx)
	for range r.workers {
		group.Go(func() error { return r.runWorkerLoop(gctx) })
	}
	return group.Wait()
}

// runWorkerLoop implements the worker loop for processing rules jobs.
func (r *Runner) runWorkerLoop(ctx context.Context) error {
	// Subscribe for notifications for rules jobs
	unsub, ch := r.jobs.Subscribe(model.JobTypeRules)
	defer unsub()

	for ctx.Err() == nil {
		job, err := r.jobs.ReserveNext(ctx, model.JobTypeRules, r.lease)
		switch {
		case err == nil:
			if job != nil {
				r.processJob(ctx, job)
			}
		case errors.Is(err, model.ErrNoJobsAvailable):
			if !r.waitForNotify(ctx, ch) {
				return nil
			}
		default:
			r.logger.ErrorContext(ctx, "failed to reserve next rules job", "error", err)
			return err
		}
	}
	return ctx.Err()
}

// processJob processes a single rules job.
func (r *Runner) processJob(ctx context.Context, job *model.Job) {
	r.logger.InfoContext(ctx, "processing rules job", "job_id", job.ID)

	stopHB := r.startHeartbeat(ctx, job.ID)
	defer stopHB()

	start := time.Now()

	if err := r.orchestrator.ProcessRulesJob(ctx, job); err != nil {
		r.logger.ErrorContext(ctx, "rules job processing failed", "job_id", job.ID, "error", err)
		if _, ferr := r.jobs.FailWithDetails(ctx, job.ID, err.Error(), service.JobFailureDetails{
			ErrorClass: obserrors.Classify(err),
			Metadata: map[string]string{
				"component": "rules_runner",
			},
		}); ferr != nil {
			r.logger.ErrorContext(ctx, "failed to mark job as failed", "job_id", job.ID, "error", ferr)
		}
		r.emitJobMetric(jobMetricInput{
			Job:        job,
			Transition: "failed",
			Result:     "error",
			Elapsed:    time.Since(start),
			Err:        err,
		})
		return
	}

	if completed, err := r.jobs.Complete(ctx, job.ID); err != nil {
		r.logger.ErrorContext(ctx, "failed to mark job as completed", "job_id", job.ID, "error", err)
		r.emitJobMetric(jobMetricInput{
			Job:        job,
			Transition: "completed",
			Result:     "error",
			Elapsed:    time.Since(start),
			Err:        err,
		})
	} else {
		result := "noop"
		if completed {
			result = "success"
		}
		r.emitJobMetric(jobMetricInput{
			Job:        job,
			Transition: "completed",
			Result:     result,
			Elapsed:    time.Since(start),
		})
	}
}

// startHeartbeat starts a background ticker to extend the job lease periodically.
// It returns a stop function to end the heartbeat.
func (r *Runner) startHeartbeat(ctx context.Context, jobID string) func() {
	interval := r.lease / 2
	if interval <= 0 {
		interval = 15 * time.Second
	}
	ticker := time.NewTicker(interval)
	done := make(chan struct{})
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if ok, err := r.jobs.Heartbeat(ctx, jobID, r.lease); err != nil {
					r.logger.ErrorContext(ctx, "heartbeat failed", "job_id", jobID, "error", err)
				} else if !ok {
					r.logger.WarnContext(ctx, "heartbeat not applied (job may be lost)", "job_id", jobID)
				}
			case <-done:
				return
			case <-ctx.Done():
				return
			}
		}
	}()
	return func() { close(done) }
}

// waitForNotify waits for a job notification or context cancellation.
func (r *Runner) waitForNotify(ctx context.Context, notify <-chan struct{}) bool {
	select {
	case <-ctx.Done():
		return false
	case <-notify:
		return true
	}
}

// Helper functions for dependency resolution and configuration

type runnerDeps struct {
	jobsRepo      core.JobRepository
	eventsRepo    core.EventRepository
	alertRepo     core.AlertRepository
	seenRepo      core.SeenDomainRepository
	alertSinkRepo core.HTTPAlertSinkRepository
	siteRepo      core.SiteRepository
	iocsRepo      core.IOCRepository
	filesRepo     core.ProcessedFileRepository
	cacheRepo     core.CacheRepository
	allowlistRepo core.DomainAllowlistRepository
	jobResultRepo core.JobResultRepository
}

func resolveDependencies(opts RunnerOptions) *runnerDeps {
	deps := &runnerDeps{}
	resolveJobRepo(opts, deps)
	resolveEventRepo(opts, deps)
	resolveAlertRepo(opts, deps)
	resolveAlertSinkRepo(opts, deps)
	resolveSiteRepo(opts, deps)
	resolveSeenRepo(opts, deps)
	resolveIOCsRepo(opts, deps)
	resolveFilesRepo(opts, deps)
	resolveCacheRepo(opts, deps)
	resolveAllowlistRepo(opts, deps)
	resolveJobResultRepo(opts, deps)
	return deps
}

func validateDependencies(opts RunnerOptions, deps *runnerDeps) error {
	if deps == nil {
		return errors.New("dependencies not resolved")
	}

	required := []struct {
		name  string
		value interface{}
	}{
		{"JobRepository", deps.jobsRepo},
		{"EventRepository", deps.eventsRepo},
		{"AlertRepository", deps.alertRepo},
		{"SeenDomainRepository", deps.seenRepo},
		{"IOCRepository", deps.iocsRepo},
	}

	var missing []string
	for _, dep := range required {
		if dep.value == nil {
			missing = append(missing, dep.name)
		}
	}

	if len(missing) > 0 {
		noun := "dependency"
		if len(missing) > 1 {
			noun = "dependencies"
		}
		missingList := strings.Join(missing, ", ")

		if opts.DB == nil {
			return fmt.Errorf(
				"rules runner requires a DB handle or explicit implementations for the following %s: %s",
				noun,
				missingList,
			)
		}

		return fmt.Errorf("rules runner missing required %s: %s", noun, missingList)
	}

	return nil
}

func resolveJobRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.JobsRepo != nil {
		deps.jobsRepo = opts.JobsRepo
		return
	}
	deps.jobsRepo = data.NewJobRepo(opts.DB, data.RepoConfig{})
}

func resolveEventRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.EventsRepo != nil {
		deps.eventsRepo = opts.EventsRepo
		return
	}
	deps.eventsRepo = &data.EventRepo{DB: opts.DB}
}

func resolveAlertRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.AlertRepo != nil {
		deps.alertRepo = opts.AlertRepo
		return
	}
	deps.alertRepo = data.NewAlertRepo(opts.DB)
}

func resolveAlertSinkRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.AlertSinkRepo != nil {
		deps.alertSinkRepo = opts.AlertSinkRepo
		return
	}
	if opts.DB != nil {
		deps.alertSinkRepo = data.NewHTTPAlertSinkRepo(opts.DB)
	}
}

func resolveSiteRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.SiteRepo != nil {
		deps.siteRepo = opts.SiteRepo
		return
	}
	if opts.DB != nil {
		deps.siteRepo = data.NewSiteRepo(opts.DB)
	}
}

func resolveSeenRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.SeenRepo != nil {
		deps.seenRepo = opts.SeenRepo
		return
	}
	deps.seenRepo = data.NewSeenDomainRepo(opts.DB)
}

func resolveIOCsRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.IOCsRepo != nil {
		deps.iocsRepo = opts.IOCsRepo
		return
	}
	if opts.DB != nil {
		deps.iocsRepo = data.NewIOCRepo(opts.DB)
	}
}

func resolveFilesRepo(opts RunnerOptions, deps *runnerDeps) {
	deps.filesRepo = opts.FilesRepo
}

func resolveCacheRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.CacheRepo != nil {
		deps.cacheRepo = opts.CacheRepo
		return
	}
	if opts.RedisClient != nil {
		deps.cacheRepo = data.NewRedisCacheRepo(opts.RedisClient)
	}
}

func resolveAllowlistRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.AllowlistRepo != nil {
		deps.allowlistRepo = opts.AllowlistRepo
		return
	}
	deps.allowlistRepo = data.NewDomainAllowlistRepo(opts.DB)
}

func resolveJobResultRepo(opts RunnerOptions, deps *runnerDeps) {
	if opts.JobResultRepo != nil {
		deps.jobResultRepo = opts.JobResultRepo
		return
	}
	if opts.DB != nil {
		deps.jobResultRepo = data.NewJobResultRepo(opts.DB)
	}
}

func buildRulesCaches(deps *runnerDeps, metrics rules.CacheMetrics) rules.Caches {
	opts := rules.DefaultCachesOptions()
	opts.Redis = deps.cacheRepo
	opts.SeenRepo = deps.seenRepo
	opts.IOCsRepo = deps.iocsRepo
	opts.FilesRepo = deps.filesRepo
	opts.Metrics = metrics

	return rules.BuildCaches(opts)
}

func createAlertService(deps *runnerDeps, logger *slog.Logger) (*service.AlertService, error) {
	if deps == nil {
		return nil, errors.New("alert service dependencies missing")
	}
	if deps.alertRepo == nil {
		return nil, errors.New("alert repository is required for alert service")
	}

	var dispatcher service.AlertDispatcher
	if deps.alertSinkRepo != nil && deps.jobsRepo != nil && deps.siteRepo != nil {
		alertSinkSvc := service.NewAlertSinkService(service.AlertSinkServiceOptions{
			JobRepo: deps.jobsRepo,
		})
		dispatcher = service.NewAlertDispatchService(service.AlertDispatchServiceOptions{
			Sites:     deps.siteRepo,
			Sinks:     deps.alertSinkRepo,
			AlertSink: alertSinkSvc,
			Logger:    logger,
		})
	} else if logger != nil {
		logger.InfoContext(context.Background(), "alert dispatch disabled (missing repository)",
			"have_alert_sink_repo", deps.alertSinkRepo != nil,
			"have_job_repo", deps.jobsRepo != nil,
			"have_site_repo", deps.siteRepo != nil)
	}

	svc, err := service.NewAlertService(service.AlertServiceOptions{
		Repo:       deps.alertRepo,
		Sites:      deps.siteRepo,
		Dispatcher: dispatcher,
		Logger:     logger,
	})
	if err != nil {
		return nil, fmt.Errorf("new alert service: %w", err)
	}
	return svc, nil
}

func createUnknownDomainEvaluator(
	caches rules.Caches,
	alerter rules.AlertCreator,
	allowlistRepo core.DomainAllowlistRepository,
	logger *slog.Logger,
) *rules.UnknownDomainEvaluator {
	var allowlistChecker rules.AllowlistChecker
	if allowlistRepo != nil {
		allowlistService := service.NewDomainAllowlistService(service.DomainAllowlistServiceOptions{
			Repo: allowlistRepo,
		})
		allowlistChecker = rules.NewDomainAllowlistChecker(rules.DomainAllowlistCheckerOptions{
			Service:   allowlistService,
			CacheTTL:  5 * time.Minute,
			CacheSize: 1000,
		})
	}

	return &rules.UnknownDomainEvaluator{
		Caches:    caches,
		Alerter:   alerter,
		Allowlist: allowlistChecker,
		AlertTTL:  24 * time.Hour, // Dedupe alerts for 24 hours
		Logger:    logger,
	}
}

func createIOCEvaluator(caches rules.Caches, alerter rules.AlertCreator) *rules.IOCEvaluator {
	return &rules.IOCEvaluator{
		Caches:   caches,
		Alerter:  alerter,
		AlertTTL: 24 * time.Hour, // Dedupe alerts for 24 hours
	}
}

func buildRuleset(unknown *rules.UnknownDomainEvaluator, globalIOC *rules.IOCEvaluator) []domainrules.Rule {
	var ruleSet []domainrules.Rule
	if unknown != nil {
		ruleSet = append(ruleSet, &domainrules.UnknownDomainRule{Evaluator: unknown})
	}
	if globalIOC != nil {
		ruleSet = append(ruleSet, &domainrules.IOCRule{
			Evaluator: globalIOC,
			Cache:     globalIOC.Caches.IOCs,
		})
	}
	return ruleSet
}

func resolveLogger(l *slog.Logger) *slog.Logger {
	if l != nil {
		return l
	}
	return slog.Default()
}

func resolveLease(lease time.Duration) time.Duration {
	if lease > 0 {
		return lease
	}
	return 30 * time.Second
}

func resolveWorkers(workers int) int {
	if workers > 0 {
		return workers
	}
	return 1
}

type jobMetricInput struct {
	Job        *model.Job
	Transition string
	Result     string
	Elapsed    time.Duration
	Err        error
}

func (r *Runner) emitJobMetric(input jobMetricInput) {
	if r.metrics == nil || input.Job == nil {
		return
	}

	metrics.EmitJobLifecycle(r.metrics, metrics.JobMetric{
		JobType:    string(input.Job.Type),
		Transition: input.Transition,
		Result:     input.Result,
		Duration:   input.Elapsed,
		Err:        input.Err,
	})
}

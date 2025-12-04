package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	domainrules "github.com/target/mmk-ui-api/internal/domain/rules"
	"github.com/target/mmk-ui-api/internal/service/rules"
)

// RulesOrchestrationService coordinates rules processing jobs and manages the rules engine workflow.
type RulesOrchestrationService struct {
	events core.EventRepository
	jobs   core.JobRepository
	logger *slog.Logger

	coordinator   domainrules.JobCoordinator
	results       domainrules.ResultStore
	pipeline      domainrules.Pipeline
	alertResolver domainrules.AlertResolver
	eventFetcher  domainrules.EventFetcher
}

// RulesOrchestrationOptions configures the rules orchestration service.

var (
	// ErrDuplicateEnqueue indicates a rules job enqueue request was a duplicate and was skipped.
	ErrDuplicateEnqueue = domainrules.ErrDuplicateEnqueue
	// ErrRulesResultsNotFound indicates no cached rules results were found for a job.
	ErrRulesResultsNotFound = domainrules.ErrResultsNotFound
	// ErrRuleEvaluationFailed indicates rule evaluation encountered errors that should surface to callers.
	ErrRuleEvaluationFailed = domainrules.ErrEvaluationFailed
)

type EnqueueRulesJobRequest = domainrules.EnqueueJobRequest

// RulesJobPayload represents the payload for a rules processing job.
type RulesJobPayload = domainrules.JobPayload

type (
	RulesProcessingResults = domainrules.ProcessingResults
	UnknownDomainMetrics   = domainrules.UnknownDomainMetrics
	IOCMetrics             = domainrules.IOCMetrics
)

type RulesOrchestrationOptions struct {
	Events     core.EventRepository
	Jobs       core.JobRepository
	Sites      core.SiteRepository
	Caches     rules.Caches
	Logger     *slog.Logger
	BatchSize  int // Maximum events to process per job; defaults to 100
	JobResults core.JobResultRepository

	// Optional: cache for dedupe locks on enqueue (Option A)
	DedupeCache core.CacheRepository
	DedupeTTL   time.Duration

	// Rule evaluators
	UnknownDomainEvaluator *rules.UnknownDomainEvaluator
	IOCEvaluator           *rules.IOCEvaluator
	Rules                  []domainrules.Rule

	// Optional: allow tests or callers to provide a custom pipeline implementation.
	Pipeline      domainrules.Pipeline
	AlertResolver domainrules.AlertResolver
	EventFetcher  domainrules.EventFetcher
}

// NewRulesOrchestrationService creates a new rules orchestration service.
func NewRulesOrchestrationService(opts RulesOrchestrationOptions) *RulesOrchestrationService {
	logger := resolveLogger(opts.Logger)
	dedupeTTL := resolveDedupeTTL(opts.DedupeTTL)
	batchSize := resolveBatchSize(opts.BatchSize)

	coordinator := newRulesJobCoordinator(opts, logger, jobCoordinatorConfig{
		ttl:       dedupeTTL,
		batchSize: batchSize,
	})
	resultStore := newRulesResultStore(opts, logger, dedupeTTL)
	pipeline := opts.Pipeline
	if pipeline == nil {
		ruleset := resolveRules(opts)
		extractor := domainrules.NewNetworkEventExtractor()
		pipeline = domainrules.NewPipeline(domainrules.PipelineOptions{
			Engine: domainrules.NewRuleEngine(ruleset),
			Extractor: domainrules.DomainExtractorFunc(func(event model.RawEvent) (string, bool) {
				return extractor.ExtractDomainFromNetworkEvent(event)
			}),
			Logger: logger,
		})
	}

	alertResolver := opts.AlertResolver
	eventFetcher := opts.EventFetcher
	if alertResolver == nil || eventFetcher == nil {
		preparer := domainrules.NewJobPreparationService(domainrules.JobPreparationOptions{
			Sites:  opts.Sites,
			Events: opts.Events,
			Logger: logger,
		})

		if alertResolver == nil {
			alertResolver = preparer
		}
		if eventFetcher == nil {
			eventFetcher = preparer
		}
	}

	return &RulesOrchestrationService{
		events:        opts.Events,
		jobs:          opts.Jobs,
		logger:        logger,
		coordinator:   coordinator,
		results:       resultStore,
		pipeline:      pipeline,
		alertResolver: alertResolver,
		eventFetcher:  eventFetcher,
	}
}

func resolveLogger(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}

func resolveDedupeTTL(ttl time.Duration) time.Duration {
	if ttl > 0 {
		return ttl
	}
	return 2 * time.Minute
}

func resolveBatchSize(batchSize int) int {
	if batchSize > 0 {
		return batchSize
	}
	return 100
}

func newRulesJobCoordinator(
	opts RulesOrchestrationOptions,
	logger *slog.Logger,
	config jobCoordinatorConfig,
) *domainrules.DefaultJobCoordinator {
	coordinatorOpts := domainrules.JobCoordinatorOptions{
		Cache:     opts.DedupeCache,
		TTL:       config.ttl,
		BatchSize: config.batchSize,
		Logger:    logger,
	}
	return domainrules.NewJobCoordinator(coordinatorOpts)
}

type jobCoordinatorConfig struct {
	ttl       time.Duration
	batchSize int
}

func newRulesResultStore(
	opts RulesOrchestrationOptions,
	logger *slog.Logger,
	ttl time.Duration,
) *domainrules.JobResultStore {
	return domainrules.NewResultStore(domainrules.ResultStoreOptions{
		Cache:      opts.DedupeCache,
		CacheTTL:   ttl,
		Repository: opts.JobResults,
		Logger:     logger,
		JobType:    model.JobTypeRules,
		IsNotFound: func(err error) bool {
			return errors.Is(err, data.ErrJobResultsNotFound)
		},
	})
}

func resolveRules(opts RulesOrchestrationOptions) []domainrules.Rule {
	if len(opts.Rules) > 0 {
		return opts.Rules
	}
	var configured []domainrules.Rule
	if opts.UnknownDomainEvaluator != nil {
		configured = append(configured, &domainrules.UnknownDomainRule{Evaluator: opts.UnknownDomainEvaluator})
	}
	if opts.IOCEvaluator != nil {
		var cache rules.IOCCache
		if opts.IOCEvaluator.Caches.IOCs != nil {
			cache = opts.IOCEvaluator.Caches.IOCs
		}
		configured = append(configured, &domainrules.IOCRule{
			Evaluator: opts.IOCEvaluator,
			Cache:     cache,
		})
	}
	return configured
}

// EnqueueRulesProcessingJob creates a rules processing job for the given events.
func (s *RulesOrchestrationService) EnqueueRulesProcessingJob(
	ctx context.Context,
	req EnqueueRulesJobRequest,
) (*model.Job, error) {
	if validateErr := req.Validate(); validateErr != nil {
		return nil, fmt.Errorf("invalid request: %w", validateErr)
	}

	payloadBytes, err := s.coordinator.BuildPayload(&req)
	if err != nil {
		return nil, err
	}

	shouldProcess, err := s.coordinator.ShouldProcess(ctx, &req)
	if err != nil {
		return nil, err
	}
	if !shouldProcess {
		return nil, ErrDuplicateEnqueue
	}

	jobReq := &model.CreateJobRequest{
		Type:       model.JobTypeRules,
		Payload:    payloadBytes,
		SiteID:     &req.SiteID,
		IsTest:     req.IsTest,
		Priority:   req.Priority,
		MaxRetries: 3, // Default retry policy for rules processing
	}

	job, err := s.jobs.Create(ctx, jobReq)
	if err != nil {
		return nil, fmt.Errorf("create rules job: %w", err)
	}

	s.logger.InfoContext(ctx, "enqueued rules processing job",
		"job_id", job.ID,
		"site_id", req.SiteID,
		"scope", req.Scope,
		"event_count", len(req.EventIDs))

	return job, nil
}

// ProcessRulesJob processes a rules job by evaluating events against configured rules.
func (s *RulesOrchestrationService) ProcessRulesJob(ctx context.Context, job *model.Job) error {
	payload, err := s.coordinator.ParsePayload(job)
	if err != nil {
		return err
	}

	s.logJobStart(job.ID, payload)

	alertMode := s.resolveAlertModeForJob(ctx, job.ID, payload)

	events, err := s.loadJobEvents(ctx, job.ID, payload)
	if err != nil {
		return err
	}

	if len(events) == 0 {
		s.finalizeEmptyRulesJob(ctx, job, alertMode)
		return nil
	}

	results, pipelineErr := s.runPipeline(runPipelineParams{
		Ctx:       ctx,
		Job:       job,
		Payload:   payload,
		Events:    events,
		AlertMode: alertMode,
	})
	if pipelineErr != nil {
		return pipelineErr
	}

	return s.completeJob(completeJobParams{
		Ctx:     ctx,
		Job:     job,
		Events:  events,
		Results: results,
	})
}

func (s *RulesOrchestrationService) finalizeEmptyRulesJob(
	ctx context.Context,
	job *model.Job,
	alertMode model.SiteAlertMode,
) {
	s.logger.WarnContext(ctx, "no events found for rules job", "job_id", job.ID)
	emptyResults := &RulesProcessingResults{IsDryRun: job.IsTest, AlertMode: alertMode}
	s.logJobCompletion(job.ID, 0, emptyResults)
	s.storeResults(ctx, job, emptyResults)
}

// logJobStart logs the start of job processing.
func (s *RulesOrchestrationService) logJobStart(jobID string, payload *RulesJobPayload) {
	s.logger.Info("processing rules job",
		"job_id", jobID,
		"site_id", payload.SiteID,
		"scope", payload.Scope,
		"event_count", len(payload.EventIDs))
}

type markEventsProcessedParams struct {
	Ctx    context.Context
	Events []*model.Event
	JobID  string
}

func (s *RulesOrchestrationService) resolveAlertModeForJob(
	ctx context.Context,
	jobID string,
	payload *RulesJobPayload,
) model.SiteAlertMode {
	if s.alertResolver == nil || payload == nil {
		return model.SiteAlertModeActive
	}

	return s.alertResolver.Resolve(ctx, domainrules.AlertResolutionParams{
		JobID:  jobID,
		SiteID: payload.SiteID,
	})
}

func (s *RulesOrchestrationService) loadJobEvents(
	ctx context.Context,
	jobID string,
	payload *RulesJobPayload,
) ([]*model.Event, error) {
	if payload == nil {
		return nil, nil
	}
	if s.eventFetcher == nil {
		return nil, errors.New("rules event fetcher is not configured")
	}

	eventIDs := s.coordinator.LimitEventIDs(payload.EventIDs, jobID)
	return s.eventFetcher.Fetch(ctx, domainrules.EventFetchParams{
		JobID:    jobID,
		EventIDs: eventIDs,
	})
}

type runPipelineParams struct {
	Ctx       context.Context
	Job       *model.Job
	Payload   *RulesJobPayload
	Events    []*model.Event
	AlertMode model.SiteAlertMode
}

func (s *RulesOrchestrationService) runPipeline(params runPipelineParams) (*RulesProcessingResults, error) {
	if s.pipeline == nil {
		return nil, errors.New("rules pipeline is not configured")
	}

	return s.pipeline.Run(params.Ctx, domainrules.PipelineParams{
		Events:    params.Events,
		Payload:   params.Payload,
		DryRun:    params.Job.IsTest,
		AlertMode: params.AlertMode,
		JobID:     params.Job.ID,
	})
}

type completeJobParams struct {
	Ctx     context.Context
	Job     *model.Job
	Events  []*model.Event
	Results *RulesProcessingResults
}

func (s *RulesOrchestrationService) completeJob(params completeJobParams) error {
	if params.Results == nil {
		return errors.New("rules pipeline returned no results")
	}

	if params.Results.ErrorsEncountered == 0 {
		s.markEventsProcessed(markEventsProcessedParams{
			Ctx:    params.Ctx,
			Events: params.Events,
			JobID:  params.Job.ID,
		})
	} else {
		s.logger.WarnContext(params.Ctx, "skipping event finalization due to rule evaluation errors",
			"job_id", params.Job.ID,
			"errors_encountered", params.Results.ErrorsEncountered)
	}

	s.logJobCompletion(params.Job.ID, len(params.Events), params.Results)
	s.storeResults(params.Ctx, params.Job, params.Results)

	if params.Results.ErrorsEncountered > 0 {
		return fmt.Errorf("%w: %d", ErrRuleEvaluationFailed, params.Results.ErrorsEncountered)
	}
	return nil
}

// markEventsProcessed marks events as processed.
func (s *RulesOrchestrationService) markEventsProcessed(params markEventsProcessedParams) {
	processedIDs := make([]string, 0, len(params.Events))
	for _, e := range params.Events {
		processedIDs = append(processedIDs, e.ID)
	}
	if updated, err := s.events.MarkProcessedByIDs(params.Ctx, processedIDs); err != nil {
		s.logger.Error("failed to mark events processed", "job_id", params.JobID, "error", err)
	} else {
		s.logger.Debug("marked events as processed", "job_id", params.JobID, "updated_count", updated)
	}
}

// logJobCompletion logs the completion of job processing.
func (s *RulesOrchestrationService) logJobCompletion(
	jobID string,
	eventsCount int,
	results *RulesProcessingResults,
) {
	s.logger.Info("completed rules job processing",
		"job_id", jobID,
		"events_processed", eventsCount,
		"events_skipped", results.EventsSkipped,
		"alerts_created", results.AlertsCreated,
		"domains_processed", results.DomainsProcessed,
		"unknown_domains", results.UnknownDomains,
		"ioc_host_matches", results.IOCHostMatches,
		"would_alert_unknown", len(results.WouldAlertUnknown),
		"would_alert_ioc", len(results.WouldAlertIOC),
		"errors_encountered", results.ErrorsEncountered,
		"processing_time", results.ProcessingTime)
}

func (s *RulesOrchestrationService) storeResults(
	ctx context.Context,
	job *model.Job,
	results *RulesProcessingResults,
) {
	if s.results == nil || results == nil {
		return
	}
	jobID := ""
	if job != nil {
		jobID = job.ID
	}
	if jobID != "" {
		if err := s.results.Cache(ctx, jobID, results); err != nil {
			s.logger.WarnContext(ctx, "failed to cache rules job results",
				"job_id", jobID,
				"error", err)
		}
	}
	if job != nil {
		if err := s.results.Persist(ctx, job, results); err != nil {
			s.logger.ErrorContext(ctx, "failed to persist job results",
				"job_id", job.ID,
				"error", err)
		}
	}
}

// GetJobResults retrieves cached rules processing results for a given job ID, if available.
func (s *RulesOrchestrationService) GetJobResults(
	ctx context.Context,
	jobID string,
) (*RulesProcessingResults, error) {
	if jobID == "" {
		return nil, ErrRulesResultsNotFound
	}

	if s.results == nil {
		return nil, ErrRulesResultsNotFound
	}

	return s.results.Get(ctx, jobID)
}

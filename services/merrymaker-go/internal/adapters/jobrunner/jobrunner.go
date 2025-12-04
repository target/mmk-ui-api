// Package jobrunner provides job execution and worker management functionality for the merrymaker system.
package jobrunner

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	obserrors "github.com/target/mmk-ui-api/internal/observability/errors"
	"github.com/target/mmk-ui-api/internal/observability/metrics"
	"github.com/target/mmk-ui-api/internal/observability/statsd"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/service/failurenotifier"
)

// HandlerFunc processes a job and returns error to indicate failure (which will be retried per policy).
type HandlerFunc func(ctx context.Context, job *model.Job) error

const (
	maxResponseBodyBytes = 4 * 1024 // 4KB to avoid storing excessively large payloads
	maxRequestBodyBytes  = 4 * 1024 // Match response truncation limit for request payloads
)

// RunnerOptions configures the job runner adapter.
type RunnerOptions struct {
	DB         *sql.DB
	Logger     *slog.Logger
	HTTPClient *http.Client

	// Job processing settings
	Lease       time.Duration // per-job lease duration; defaults to 30s
	Concurrency int           // number of worker goroutines; defaults to 1
	JobType     model.JobType // which job type to process; defaults to alert

	// Debug mode for secret refresh jobs (logs actual secret values - DANGEROUS)
	SecretRefreshDebugMode bool

	// Encryptor for secrets (if nil, will use NoopEncryptor)
	Encryptor cryptoutil.Encryptor

	// Optional dependency injections (useful for tests/decoupling)
	JobsRepo        core.JobRepository
	SecretsRepo     core.SecretRepository
	AlertSinkRepo   core.HTTPAlertSinkRepository
	JobResultRepo   core.JobResultRepository
	Metrics         statsd.Sink
	FailureNotifier *failurenotifier.Service
}

// Runner pulls jobs and executes them using registered handlers.
type Runner struct {
	jobs             *service.JobService
	alerts           *service.AlertSinkService
	sinks            core.HTTPAlertSinkRepository
	jobResults       core.JobResultRepository
	secretRefreshSvc *service.SecretRefreshService
	http             *http.Client
	logger           *slog.Logger
	lease            time.Duration
	jobType          model.JobType
	workers          int
	handlers         map[model.JobType]HandlerFunc
	metrics          statsd.Sink
}

// internal wiring helpers to keep NewRunner small

type runnerDeps struct {
	jobsRepo         core.JobRepository
	secretsRepo      core.SecretRepository
	sinksRepo        core.HTTPAlertSinkRepository
	jobResultsRepo   core.JobResultRepository
	jobSvc           *service.JobService
	alertSvc         *service.AlertSinkService
	secretRefreshSvc *service.SecretRefreshService
}

func resolveLogger(l *slog.Logger) *slog.Logger {
	if l != nil {
		return l
	}
	return slog.Default()
}

func resolveHTTPClient(hc *http.Client) *http.Client {
	if hc != nil {
		return hc
	}
	return &http.Client{Timeout: 30 * time.Second}
}

func buildRunnerDeps(opts RunnerOptions, lease time.Duration) runnerDeps {
	deps := runnerDeps{}

	if opts.JobsRepo != nil {
		deps.jobsRepo = opts.JobsRepo
	} else {
		deps.jobsRepo = data.NewJobRepo(opts.DB, data.RepoConfig{})
	}
	deps.jobSvc = service.MustNewJobService(service.JobServiceOptions{
		Repo:            deps.jobsRepo,
		DefaultLease:    lease,
		FailureNotifier: opts.FailureNotifier,
	})

	if opts.SecretsRepo != nil {
		deps.secretsRepo = opts.SecretsRepo
	} else if opts.DB != nil {
		enc := opts.Encryptor
		if enc == nil {
			enc = &cryptoutil.NoopEncryptor{}
		}
		deps.secretsRepo = data.NewSecretRepo(opts.DB, enc)
	}

	if opts.AlertSinkRepo != nil {
		deps.sinksRepo = opts.AlertSinkRepo
	} else if opts.DB != nil {
		deps.sinksRepo = data.NewHTTPAlertSinkRepo(opts.DB)
	}

	if opts.JobResultRepo != nil {
		deps.jobResultsRepo = opts.JobResultRepo
	} else if opts.DB != nil {
		deps.jobResultsRepo = data.NewJobResultRepo(opts.DB)
	}

	deps.alertSvc = service.NewAlertSinkService(service.AlertSinkServiceOptions{
		JobRepo:    deps.jobsRepo,
		SecretRepo: deps.secretsRepo,
		Evaluator:  nil,
	})

	deps.secretRefreshSvc = newSecretRefreshService(opts, deps.secretsRepo, deps.jobsRepo)

	return deps
}

func newSecretRefreshService(
	opts RunnerOptions,
	secretRepo core.SecretRepository,
	jobRepo core.JobRepository,
) *service.SecretRefreshService {
	if secretRepo == nil || opts.DB == nil {
		return nil
	}
	scheduledAdmin := data.NewScheduledJobsAdminRepo(opts.DB)
	return service.MustNewSecretRefreshService(service.SecretRefreshServiceOptions{
		SecretRepo: secretRepo,
		AdminRepo:  scheduledAdmin,
		JobRepo:    jobRepo,
		Logger:     opts.Logger,
		DebugMode:  opts.SecretRefreshDebugMode,
	})
}

// NewRunner wires repositories/services and constructs a job runner for a single job type.
func NewRunner(opts RunnerOptions) (*Runner, error) {
	if opts.DB == nil && opts.JobsRepo == nil {
		return nil, errors.New("either DB or JobsRepo must be provided")
	}

	logger := resolveLogger(opts.Logger)
	hc := resolveHTTPClient(opts.HTTPClient)

	lease := opts.Lease
	if lease <= 0 {
		lease = 30 * time.Second
	}
	workers := opts.Concurrency
	if workers <= 0 {
		workers = 1
	}
	jt := opts.JobType
	if !jt.Valid() {
		jt = model.JobTypeAlert
	}

	deps := buildRunnerDeps(opts, lease)

	r := &Runner{
		jobs:             deps.jobSvc,
		alerts:           deps.alertSvc,
		sinks:            deps.sinksRepo,
		jobResults:       deps.jobResultsRepo,
		secretRefreshSvc: deps.secretRefreshSvc,
		http:             hc,
		logger:           logger,
		lease:            lease,
		jobType:          jt,
		workers:          workers,
		handlers:         make(map[model.JobType]HandlerFunc),
		metrics:          opts.Metrics,
	}
	// Register built-in handlers
	r.handlers[model.JobTypeAlert] = r.handleAlertJob
	if r.secretRefreshSvc != nil {
		r.handlers[model.JobTypeSecretRefresh] = r.handleSecretRefreshJob
	} else {
		r.logger.WarnContext(context.Background(), "SecretRefreshService not configured; secret refresh jobs will be ignored")
	}
	return r, nil
}

// Run starts worker goroutines and processes jobs until the context is cancelled.
func (r *Runner) Run(ctx context.Context) error {
	r.logger.InfoContext(ctx, "starting job runner", "type", r.jobType, "workers", r.workers, "lease", r.lease)

	// Derive a cancellable context that we can signal on first fatal error
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Subscribe for notifications for the job type we process
	unsub, ch := r.jobs.Subscribe(r.jobType)
	defer unsub()

	var wg sync.WaitGroup
	errCh := make(chan error, 1)

	for range r.workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := r.workerLoop(ctx, ch); err != nil {
				// first error wins, cancels all workers
				select {
				case errCh <- err:
					cancel()
				default:
				}
			}
		}()
	}

	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return ctx.Err()
	}
}

func (r *Runner) workerLoop(ctx context.Context, notify <-chan struct{}) error {
	for ctx.Err() == nil {
		job, err := r.jobs.ReserveNext(ctx, r.jobType, r.lease)
		switch {
		case err == nil:
			if job != nil {
				r.processJob(ctx, job)
			}
		case errors.Is(err, model.ErrNoJobsAvailable):
			if !r.waitForNotify(ctx, notify) {
				return nil
			}
		default:
			return fmt.Errorf("reserve next: %w", err)
		}
	}
	return ctx.Err()
}

func (r *Runner) waitForNotify(ctx context.Context, notify <-chan struct{}) bool {
	select {
	case <-ctx.Done():
		return false
	case <-notify:
		return true
	}
}

func (r *Runner) processJob(ctx context.Context, job *model.Job) {
	start := time.Now()
	emit := func(transition, result string, err error) {
		metrics.EmitJobLifecycle(r.metrics, metrics.JobMetric{
			JobType:    string(job.Type),
			Transition: transition,
			Result:     result,
			Duration:   time.Since(start),
			Err:        err,
		})
	}

	h, ok := r.handlers[job.Type]
	if !ok {
		err := fmt.Errorf("no handler for job type %s", job.Type)
		_ = r.fail(ctx, job.ID, err.Error(), service.JobFailureDetails{
			ErrorClass: obserrors.Classify(err),
			Metadata: map[string]string{
				"component": r.componentLabel(),
			},
		})
		emit("failed", metrics.ResultError, err)
		return
	}
	if err := h(ctx, job); err != nil {
		if _, ferr := r.jobs.FailWithDetails(ctx, job.ID, err.Error(), service.JobFailureDetails{
			ErrorClass: obserrors.Classify(err),
			Metadata: map[string]string{
				"component": r.componentLabel(),
			},
		}); ferr != nil {
			r.logger.ErrorContext(ctx, "fail job error", "job_id", job.ID, "error", ferr, "original_error", err)
		}
		emit("failed", metrics.ResultError, err)
		return
	}
	if completed, err := r.jobs.Complete(ctx, job.ID); err != nil {
		r.logger.ErrorContext(ctx, "complete job error", "job_id", job.ID, "error", err)
		emit("completed", metrics.ResultError, err)
	} else {
		result := metrics.ResultNoop
		if completed {
			result = metrics.ResultSuccess
		}
		emit("completed", result, nil)
	}
}

func (r *Runner) fail(ctx context.Context, id, msg string, details service.JobFailureDetails) bool {
	ok, err := r.jobs.FailWithDetails(ctx, id, msg, details)
	if err != nil {
		r.logger.ErrorContext(ctx, "fail job error", "job_id", id, "error", err)
	}
	return ok
}

func (r *Runner) componentLabel() string {
	switch r.jobType {
	case model.JobTypeBrowser:
		return "browser_runner"
	case model.JobTypeRules:
		return "rules_runner"
	case model.JobTypeSecretRefresh:
		return "secret_refresh_runner"
	case model.JobTypeAlert:
		return "alert_runner"
	default:
		return "job_runner"
	}
}

// handleAlertJob processes an alert job by preparing and sending an HTTP request,
// persisting the attempt details for later inspection.
func (r *Runner) handleAlertJob(ctx context.Context, job *model.Job) error {
	if r.sinks == nil {
		err := errors.New("alert sink repository not configured")
		result := newAlertJobResult(job, time.Now())
		result.JobStatus = model.JobStatusFailed
		result.ErrorMessage = err.Error()
		r.persistAlertJobResult(ctx, job, result)
		return err
	}

	start := time.Now()
	result := newAlertJobResult(job, start)
	var runErr error
	finalizer := finalizeAlertJobResultInput{
		job:       job,
		result:    result,
		startedAt: start,
	}
	defer func() {
		finalizer.err = runErr
		r.finalizeAlertJobResult(ctx, finalizer)
	}()

	sinkID, payload, err := decodeAlertJobPayload(job.Payload, result)
	if err != nil {
		runErr = err
		return runErr
	}

	sink, err := r.sinks.GetByID(ctx, sinkID)
	if err != nil {
		runErr = fmt.Errorf("load sink: %w", err)
		return runErr
	}

	result.SinkName = sink.Name

	// Prepare HTTP request (resolves secrets, builds URL/body/headers)
	preq, err := r.alerts.ProcessSinkConfiguration(ctx, *sink, payload)
	if err != nil {
		runErr = fmt.Errorf("prepare http request: %w", err)
		return runErr
	}
	redactor := service.NewSecretRedactor(preq.Secrets)
	applyPreparedRequest(&result.Request, preq, redactor)

	response, reqErr := r.sendAlertRequest(ctx, preq)
	if response != nil {
		result.Response = response
	}
	if reqErr != nil {
		runErr = reqErr
		return runErr
	}

	result.JobStatus = model.JobStatusCompleted
	return nil
}

func newAlertJobResult(job *model.Job, start time.Time) *service.AlertDeliveryJobResult {
	return &service.AlertDeliveryJobResult{
		JobID:         job.ID,
		AttemptNumber: job.RetryCount + 1,
		RetryCount:    job.RetryCount,
		MaxRetries:    job.MaxRetries,
		AttemptedAt:   start,
	}
}

type finalizeAlertJobResultInput struct {
	job       *model.Job
	result    *service.AlertDeliveryJobResult
	startedAt time.Time
	err       error
}

func (r *Runner) finalizeAlertJobResult(ctx context.Context, input finalizeAlertJobResultInput) {
	if input.job == nil || input.result == nil {
		return
	}
	if input.err != nil && input.result.ErrorMessage == "" {
		input.result.ErrorMessage = input.err.Error()
	}
	if input.result.JobStatus == "" {
		if input.err != nil {
			input.result.JobStatus = model.JobStatusFailed
		} else {
			input.result.JobStatus = model.JobStatusCompleted
		}
	}
	completed := time.Now()
	input.result.CompletedAt = &completed
	if !input.startedAt.IsZero() {
		input.result.DurationMs = completed.Sub(input.startedAt).Milliseconds()
	}
	r.persistAlertJobResult(ctx, input.job, input.result)
}

func decodeAlertJobPayload(raw []byte, result *service.AlertDeliveryJobResult) (string, json.RawMessage, error) {
	var payload struct {
		SinkID  string          `json:"sink_id"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return "", nil, fmt.Errorf("decode payload: %w", err)
	}
	result.SinkID = payload.SinkID
	result.Payload = payload.Payload
	if payload.SinkID == "" {
		return "", nil, errors.New("missing sink_id in job payload")
	}

	// Extract alert ID if present
	var alertPayload struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(payload.Payload, &alertPayload); err == nil {
		result.AlertID = alertPayload.ID
	}

	return payload.SinkID, payload.Payload, nil
}

func applyPreparedRequest(
	result *service.AlertDeliveryRequestSummary,
	preq *service.PreparedHTTPRequest,
	redactor service.SecretRedactor,
) {
	result.Method = preq.Method
	result.URL = redactor.RedactString(preq.URL)
	result.Headers = redactor.RedactHeaders(preq.Headers)
	if len(preq.Body) > 0 {
		body := redactor.RedactString(string(preq.Body))
		if len(body) > maxRequestBodyBytes {
			body = body[:maxRequestBodyBytes]
			result.BodyTruncated = true
		}
		result.Body = body
	}
	if result.OkStatus == 0 {
		result.OkStatus = preq.OkStatus
	}
}

func (r *Runner) sendAlertRequest(
	ctx context.Context,
	preq *service.PreparedHTTPRequest,
) (*service.AlertDeliveryResponse, error) {
	req, err := http.NewRequestWithContext(ctx, preq.Method, preq.URL, bytesReader(preq.Body))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	for k, v := range preq.Headers {
		req.Header.Set(k, v)
	}
	resp, err := r.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}

	body, truncated, readErr := readResponseBody(resp.Body)
	if readErr != nil {
		closeErr := resp.Body.Close()
		if closeErr != nil {
			return nil, errors.Join(
				fmt.Errorf("read response body: %w", readErr),
				fmt.Errorf("close response body: %w", closeErr),
			)
		}
		return nil, fmt.Errorf("read response body: %w", readErr)
	}
	if closeErr := resp.Body.Close(); closeErr != nil {
		return nil, fmt.Errorf("close response body: %w", closeErr)
	}

	response := &service.AlertDeliveryResponse{
		StatusCode:    resp.StatusCode,
		Headers:       flattenResponseHeaders(resp.Header),
		Body:          body,
		BodyTruncated: truncated,
	}

	if resp.StatusCode != preq.OkStatus {
		return response, fmt.Errorf("unexpected status: got %d, want %d", resp.StatusCode, preq.OkStatus)
	}

	return response, nil
}

// bytesReader returns an io.Reader for b, or nil if b is empty.
func bytesReader(b []byte) io.Reader {
	if len(b) == 0 {
		return nil
	}
	return bytes.NewReader(b)
}

func flattenResponseHeaders(h http.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for k, values := range h {
		out[k] = strings.Join(values, ", ")
	}
	return out
}

func readResponseBody(body io.Reader) (string, bool, error) {
	if body == nil {
		return "", false, nil
	}
	limited := io.LimitReader(body, maxResponseBodyBytes+1)
	data, readErr := io.ReadAll(limited)
	truncated := len(data) > maxResponseBodyBytes
	if truncated {
		data = data[:maxResponseBodyBytes]
		if _, drainErr := io.Copy(io.Discard, body); drainErr != nil && readErr == nil {
			readErr = drainErr
		}
	}
	return string(data), truncated, readErr
}

func (r *Runner) persistAlertJobResult(ctx context.Context, job *model.Job, result *service.AlertDeliveryJobResult) {
	if r.jobResults == nil || job == nil || result == nil {
		return
	}
	payload, err := json.Marshal(result)
	if err != nil {
		if r.logger != nil {
			r.logger.ErrorContext(ctx, "marshal alert job result", "job_id", job.ID, "error", err)
		}
		return
	}
	if upsertErr := r.jobResults.Upsert(ctx, core.UpsertJobResultParams{
		JobID:   job.ID,
		JobType: job.Type,
		Result:  payload,
	}); upsertErr != nil && r.logger != nil {
		r.logger.ErrorContext(ctx, "persist alert job result", "job_id", job.ID, "error", upsertErr)
	}
}

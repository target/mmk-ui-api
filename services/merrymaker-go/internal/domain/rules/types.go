package rules

import (
	"context"
	"errors"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

var (
	// ErrDuplicateEnqueue indicates a rules job enqueue request was a duplicate and should be skipped.
	ErrDuplicateEnqueue = errors.New("duplicate rules job request")
	// ErrResultsNotFound indicates no cached rules results were found for a job.
	ErrResultsNotFound = errors.New("rules results not found")
	// ErrEvaluationFailed indicates rule evaluation encountered errors that should surface to callers.
	ErrEvaluationFailed = errors.New("rule evaluation failed")
)

// EnqueueJobRequest represents a request to enqueue a rules processing job.
type EnqueueJobRequest struct {
	EventIDs []string `json:"event_ids"`
	SiteID   string   `json:"site_id"`
	Scope    string   `json:"scope"`
	Priority int      `json:"priority,omitempty"`
	IsTest   bool     `json:"is_test,omitempty"`
}

// Validate validates the enqueue rules job request.
func (r *EnqueueJobRequest) Validate() error {
	if len(r.EventIDs) == 0 {
		return errors.New("event_ids is required")
	}
	if r.SiteID == "" {
		return errors.New("site_id is required")
	}
	if r.Scope == "" {
		return errors.New("scope is required")
	}
	if r.Priority < 0 || r.Priority > 100 {
		return errors.New("priority must be between 0 and 100")
	}
	return nil
}

// JobPayload represents the payload for a rules processing job.
type JobPayload struct {
	EventIDs []string `json:"event_ids"`
	SiteID   string   `json:"site_id"`
	Scope    string   `json:"scope"`
}

// ProcessingResults contains the results of rules processing.
type ProcessingResults struct {
	AlertsCreated     int                  `json:"alerts_created"`
	DomainsProcessed  int                  `json:"domains_processed"`
	EventsSkipped     int                  `json:"events_skipped"`
	ProcessingTime    time.Duration        `json:"processing_time"`
	UnknownDomains    int                  `json:"unknown_domains"`
	IOCHostMatches    int                  `json:"ioc_host_matches"`
	ErrorsEncountered int                  `json:"errors_encountered"`
	IsDryRun          bool                 `json:"is_dry_run"`
	AlertMode         model.SiteAlertMode  `json:"alert_mode"`
	WouldAlertUnknown []string             `json:"would_alert_unknown,omitempty"`
	WouldAlertIOC     []string             `json:"would_alert_ioc,omitempty"`
	UnknownDomain     UnknownDomainMetrics `json:"unknown_domain"`
	IOC               IOCMetrics           `json:"ioc"`
}

const MetricsSampleLimit = 10

// MetricsBucket captures counts and sample domains for rules metrics.
type MetricsBucket struct {
	Count   int      `json:"count"`
	Samples []string `json:"samples,omitempty"`
}

// Record tracks a domain sample within the bucket, up to MetricsSampleLimit unique entries.
func (b *MetricsBucket) Record(domain string) {
	if b == nil {
		return
	}
	b.Count++
	appendSample(&b.Samples, domain)
}

// Merge combines another bucket into this bucket.
func (b *MetricsBucket) Merge(other MetricsBucket) {
	if b == nil {
		return
	}
	b.Count += other.Count
	for _, sample := range other.Samples {
		appendSample(&b.Samples, sample)
	}
}

// UnknownDomainMetrics tracks outcomes specific to unknown domain evaluations.
type UnknownDomainMetrics struct {
	Alerted             MetricsBucket `json:"alerted"`
	AlertedDryRun       MetricsBucket `json:"alerted_dry_run"`
	AlertedMuted        MetricsBucket `json:"alerted_muted"`
	SuppressedAllowlist MetricsBucket `json:"suppressed_allowlist"`
	SuppressedSeen      MetricsBucket `json:"suppressed_seen"`
	SuppressedDedupe    MetricsBucket `json:"suppressed_dedupe"`
	NormalizationFailed MetricsBucket `json:"normalization_failed"`
	Errors              MetricsBucket `json:"errors"`
}

// IOCMetrics tracks outcomes specific to IOC evaluations.
type IOCMetrics struct {
	Matches       MetricsBucket `json:"matches"`
	MatchesDryRun MetricsBucket `json:"matches_dry_run"`
	Alerts        MetricsBucket `json:"alerts"`
	AlertsMuted   MetricsBucket `json:"alerts_muted"`
}

// PipelineParams captures the context required to execute the rules pipeline.
type PipelineParams struct {
	Events    []*model.Event
	Payload   *JobPayload
	DryRun    bool
	AlertMode model.SiteAlertMode
	JobID     string
}

// Pipeline executes the domain rules evaluation workflow.
type Pipeline interface {
	Run(ctx context.Context, params PipelineParams) (*ProcessingResults, error)
}

// AlertResolutionParams captures the context required to resolve a site's alert mode.
type AlertResolutionParams struct {
	JobID  string
	SiteID string
}

// AlertResolver resolves the effective alert mode for a rules job.
type AlertResolver interface {
	Resolve(ctx context.Context, params AlertResolutionParams) model.SiteAlertMode
}

// EventFetchParams captures the context required to retrieve events for processing.
type EventFetchParams struct {
	JobID    string
	EventIDs []string
}

// EventFetcher retrieves events needed for rules processing.
type EventFetcher interface {
	Fetch(ctx context.Context, params EventFetchParams) ([]*model.Event, error)
}

// ResultStore persists and retrieves processing results.
type ResultStore interface {
	Cache(ctx context.Context, jobID string, results *ProcessingResults) error
	Persist(ctx context.Context, job *model.Job, results *ProcessingResults) error
	Get(ctx context.Context, jobID string) (*ProcessingResults, error)
}

// JobCoordinator encapsulates rules job request orchestration concerns.
type JobCoordinator interface {
	BuildPayload(req *EnqueueJobRequest) ([]byte, error)
	ShouldProcess(ctx context.Context, req *EnqueueJobRequest) (bool, error)
	ParsePayload(job *model.Job) (*JobPayload, error)
	LimitEventIDs(ids []string, jobID string) []string
}

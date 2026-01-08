package core

import (
	"context"
	"database/sql"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// This file contains repository interface definitions (ports in hexagonal architecture).
// These interfaces define the contracts between the service layer and data layer.
// Service implementations should depend on these interfaces, not concrete implementations.

// JobRepository defines the interface for job data operations.
type JobRepository interface {
	Create(ctx context.Context, req *model.CreateJobRequest) (*model.Job, error)
	GetByID(ctx context.Context, id string) (*model.Job, error)
	ReserveNext(ctx context.Context, jobType model.JobType, leaseSeconds int) (*model.Job, error)
	WaitForNotification(ctx context.Context, jobType model.JobType) error
	Heartbeat(ctx context.Context, jobID string, leaseSeconds int) (bool, error)
	Complete(ctx context.Context, id string) (bool, error)
	Fail(ctx context.Context, id, errMsg string) (bool, error)
	Stats(ctx context.Context, jobType model.JobType) (*model.JobStats, error)
	List(ctx context.Context, opts *model.JobListOptions) ([]*model.JobWithEventCount, error)
	Delete(ctx context.Context, id string) error
	DeleteByPayloadField(ctx context.Context, params DeleteByPayloadFieldParams) (int, error)
}

// JobRepositoryTx defines optional transactional job creation support.
type JobRepositoryTx interface {
	CreateInTx(ctx context.Context, tx *sql.Tx, req *model.CreateJobRequest) (*model.Job, error)
}

// DeleteByPayloadFieldParams groups parameters for DeleteByPayloadField to keep param count ≤3.
type DeleteByPayloadFieldParams struct {
	JobType    model.JobType
	FieldName  string
	FieldValue string
}

// UpsertJobResultParams groups parameters for JobResultRepository.Upsert.
type UpsertJobResultParams struct {
	JobID   string
	JobType model.JobType
	Result  []byte
}

// JobResultRepository defines the interface for persisted job result data.
type JobResultRepository interface {
	Upsert(ctx context.Context, params UpsertJobResultParams) error
	GetByJobID(ctx context.Context, jobID string) (*model.JobResult, error)
	ListByAlertID(ctx context.Context, alertID string) ([]*model.JobResult, error)
}

// EventRepository defines the interface for event data operations.
type EventRepository interface {
	BulkInsert(ctx context.Context, req model.BulkEventRequest, process bool) (int, error)
	BulkInsertWithProcessingFlags(
		ctx context.Context,
		req model.BulkEventRequest,
		shouldProcessMap map[int]bool,
	) (int, error)
	// ListByJob returns events for a job with optional filters (EventType, Category, SearchQuery, SortBy, SortDir).
	ListByJob(ctx context.Context, opts model.EventListByJobOptions) (*model.EventListPage, error)
	// CountByJob returns the total count of events for a job with optional filters (EventType, Category, SearchQuery).
	CountByJob(ctx context.Context, opts model.EventListByJobOptions) (int, error)
	// GetByIDs retrieves events by their IDs.
	GetByIDs(ctx context.Context, eventIDs []string) ([]*model.Event, error)
	// MarkProcessedByIDs sets processed=true for the given event IDs and returns the number of rows updated.
	MarkProcessedByIDs(ctx context.Context, eventIDs []string) (int, error)
}

// SourceRepository defines the interface for source data operations.
type SourceRepository interface {
	Create(ctx context.Context, req *model.CreateSourceRequest) (*model.Source, error)
	GetByID(ctx context.Context, id string) (*model.Source, error)
	GetByName(ctx context.Context, name string) (*model.Source, error)
	List(ctx context.Context, limit, offset int) ([]*model.Source, error)
	Update(ctx context.Context, id string, req model.UpdateSourceRequest) (*model.Source, error)
	Delete(ctx context.Context, id string) (bool, error)
}

// SecretRepository defines the interface for secret data operations.
type SecretRepository interface {
	Create(ctx context.Context, req model.CreateSecretRequest) (*model.Secret, error)
	GetByID(ctx context.Context, id string) (*model.Secret, error)
	GetByName(ctx context.Context, name string) (*model.Secret, error)
	List(ctx context.Context, limit, offset int) ([]*model.Secret, error)
	Update(ctx context.Context, id string, req model.UpdateSecretRequest) (*model.Secret, error)
	Delete(ctx context.Context, id string) (bool, error)

	// FindDueForRefresh finds secrets that need to be refreshed based on their refresh_interval.
	// Returns secrets where refresh_enabled=true AND (last_refreshed_at IS NULL OR last_refreshed_at + refresh_interval <= now).
	FindDueForRefresh(ctx context.Context, limit int) ([]*model.Secret, error)

	// UpdateRefreshStatus updates the refresh status fields after a refresh attempt.
	UpdateRefreshStatus(ctx context.Context, params UpdateSecretRefreshStatusParams) error
}

// UpdateSecretRefreshStatusParams groups parameters for updating secret refresh status (≤3 params rule).
type UpdateSecretRefreshStatusParams struct {
	SecretID    string
	Status      string // "success", "failed", or "pending"
	ErrorMsg    *string
	RefreshedAt time.Time
}

// HTTPAlertSinkRepository defines the interface for HTTP alert sink data operations.
type HTTPAlertSinkRepository interface {
	Create(ctx context.Context, req *model.CreateHTTPAlertSinkRequest) (*model.HTTPAlertSink, error)
	GetByID(ctx context.Context, id string) (*model.HTTPAlertSink, error)
	GetByName(ctx context.Context, name string) (*model.HTTPAlertSink, error)
	List(ctx context.Context, limit, offset int) ([]*model.HTTPAlertSink, error)
	Update(
		ctx context.Context,
		id string,
		req *model.UpdateHTTPAlertSinkRequest,
	) (*model.HTTPAlertSink, error)
	Delete(ctx context.Context, id string) (bool, error)
}

// SiteRepository defines the interface for site data operations.
type SiteRepository interface {
	Create(ctx context.Context, req *model.CreateSiteRequest) (*model.Site, error)
	GetByID(ctx context.Context, id string) (*model.Site, error)
	GetByName(ctx context.Context, name string) (*model.Site, error)
	List(ctx context.Context, limit, offset int) ([]*model.Site, error)
	Update(ctx context.Context, id string, req model.UpdateSiteRequest) (*model.Site, error)
	Delete(ctx context.Context, id string) (bool, error)
}

// SiteRepositoryListWithOptions is an optional extension for repositories that support filtered listing via options.
type SiteRepositoryListWithOptions interface {
	ListWithOptions(ctx context.Context, opts model.SitesListOptions) ([]*model.Site, error)
}

// AlertRepository defines the interface for alert data operations.
type AlertRepository interface {
	Create(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error)
	GetByID(ctx context.Context, id string) (*model.Alert, error)
	List(ctx context.Context, opts *model.AlertListOptions) ([]*model.Alert, error)
	ListWithSiteNames(
		ctx context.Context,
		opts *model.AlertListOptions,
	) ([]*model.AlertWithSiteName, error)
	ListWithSiteNamesAndCount(
		ctx context.Context,
		opts *model.AlertListOptions,
	) (*model.AlertListResult, error)
	Count(ctx context.Context, opts *model.AlertListOptions) (int, error)
	Delete(ctx context.Context, id string) (bool, error)
	Stats(ctx context.Context, siteID *string) (*model.AlertStats, error)
	Resolve(ctx context.Context, params ResolveAlertParams) (*model.Alert, error)
	UpdateDeliveryStatus(ctx context.Context, params UpdateAlertDeliveryStatusParams) (*model.Alert, error)
}

// ResolveAlertParams contains parameters for resolving an alert.
type ResolveAlertParams struct {
	ID         string
	ResolvedBy string
}

// UpdateAlertDeliveryStatusParams contains parameters for updating an alert's delivery status.
type UpdateAlertDeliveryStatusParams struct {
	ID     string
	Status model.AlertDeliveryStatus
}

// RuleRepository defines the interface for rule data operations.
type RuleRepository interface {
	Create(ctx context.Context, req model.CreateRuleRequest) (*model.Rule, error)
	GetByID(ctx context.Context, id string) (*model.Rule, error)
	List(ctx context.Context, opts model.RuleListOptions) ([]*model.Rule, error)
	Update(ctx context.Context, id string, req model.UpdateRuleRequest) (*model.Rule, error)
	Delete(ctx context.Context, id string) (bool, error)
	GetBySite(ctx context.Context, siteID string, enabled *bool) ([]*model.Rule, error)
}

// DomainAllowlistRepository defines the interface for domain allowlist data operations.
type DomainAllowlistRepository interface {
	Create(
		ctx context.Context,
		req *model.CreateDomainAllowlistRequest,
	) (*model.DomainAllowlist, error)
	GetByID(ctx context.Context, id string) (*model.DomainAllowlist, error)
	Update(
		ctx context.Context,
		id string,
		req model.UpdateDomainAllowlistRequest,
	) (*model.DomainAllowlist, error)
	Delete(ctx context.Context, id string) error
	List(
		ctx context.Context,
		opts model.DomainAllowlistListOptions,
	) ([]*model.DomainAllowlist, error)
	GetForScope(
		ctx context.Context,
		req model.DomainAllowlistLookupRequest,
	) ([]*model.DomainAllowlist, error)
	Stats(ctx context.Context, siteID *string) (*model.DomainAllowlistStats, error)
}

// SeenDomainRepository defines the interface for seen domain data operations.
type SeenDomainRepository interface {
	Create(ctx context.Context, req model.CreateSeenDomainRequest) (*model.SeenDomain, error)
	GetByID(ctx context.Context, id string) (*model.SeenDomain, error)
	List(ctx context.Context, opts model.SeenDomainListOptions) ([]*model.SeenDomain, error)
	Update(
		ctx context.Context,
		id string,
		req model.UpdateSeenDomainRequest,
	) (*model.SeenDomain, error)
	Delete(ctx context.Context, id string) (bool, error)
	Lookup(ctx context.Context, req model.SeenDomainLookupRequest) (*model.SeenDomain, error)
	RecordSeen(ctx context.Context, req model.RecordDomainSeenRequest) (*model.SeenDomain, error)
}

// IOCStats holds statistics about IOCs.
type IOCStats struct {
	TotalCount   int `json:"total_count"`
	EnabledCount int `json:"enabled_count"`
	FQDNCount    int `json:"fqdn_count"`
	IPCount      int `json:"ip_count"`
}

// IOCRepository defines the interface for global IOC data operations.
type IOCRepository interface {
	Create(ctx context.Context, req model.CreateIOCRequest) (*model.IOC, error)
	GetByID(ctx context.Context, id string) (*model.IOC, error)
	List(ctx context.Context, opts model.IOCListOptions) ([]*model.IOC, error)
	Update(ctx context.Context, id string, req model.UpdateIOCRequest) (*model.IOC, error)
	Delete(ctx context.Context, id string) (bool, error)
	BulkCreate(ctx context.Context, req model.BulkCreateIOCsRequest) (int, error)
	LookupHost(ctx context.Context, req model.IOCLookupRequest) (*model.IOC, error)
	Stats(ctx context.Context) (*IOCStats, error)
}

// ProcessedFileRepository defines the interface for processed file data operations.
type ProcessedFileRepository interface {
	Create(ctx context.Context, req model.CreateProcessedFileRequest) (*model.ProcessedFile, error)
	GetByID(ctx context.Context, id string) (*model.ProcessedFile, error)
	List(ctx context.Context, opts model.ProcessedFileListOptions) ([]*model.ProcessedFile, error)
	Update(
		ctx context.Context,
		id string,
		req model.UpdateProcessedFileRequest,
	) (*model.ProcessedFile, error)
	Delete(ctx context.Context, id string) (bool, error)
	Lookup(ctx context.Context, req model.ProcessedFileLookupRequest) (*model.ProcessedFile, error)
	Stats(ctx context.Context, siteID *string) (*model.ProcessedFileStats, error)
}

// DeleteOldJobsParams groups parameters for DeleteOldJobs to keep param count ≤3.
type DeleteOldJobsParams struct {
	Status    model.JobStatus
	MaxAge    time.Duration
	BatchSize int
}

// DeleteOldJobResultsParams groups parameters for DeleteOldJobResults.
type DeleteOldJobResultsParams struct {
	JobType   model.JobType
	MaxAge    time.Duration
	BatchSize int
}

// ReaperRepository defines the interface for job cleanup operations.
type ReaperRepository interface {
	// FailStalePendingJobs marks pending jobs older than maxAge as failed.
	// Processes up to batchSize jobs per call to prevent long locks.
	// Returns the number of jobs marked as failed.
	FailStalePendingJobs(ctx context.Context, maxAge time.Duration, batchSize int) (int64, error)

	// DeleteOldJobs deletes jobs with the given status older than maxAge.
	// Processes up to batchSize jobs per call to prevent long locks.
	// Returns the number of jobs deleted.
	DeleteOldJobs(ctx context.Context, params DeleteOldJobsParams) (int64, error)

	// DeleteOldJobResults deletes persisted job_results rows for the given job type
	// that are older than maxAge. Processes up to batchSize rows per call.
	DeleteOldJobResults(ctx context.Context, params DeleteOldJobResultsParams) (int64, error)
}

package rulestest

import (
	"context"
	"errors"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	domainrules "github.com/target/mmk-ui-api/internal/domain/rules"
)

var errNotImplemented = errors.New("rulestest: method not implemented")

// AlertResolverStub is a reusable implementation of domainrules.AlertResolver for tests.
type AlertResolverStub struct {
	ResolveFn func(ctx context.Context, params domainrules.AlertResolutionParams) model.SiteAlertMode
	Calls     []domainrules.AlertResolutionParams
}

// Resolve records the invocation and delegates to ResolveFn when provided.
func (s *AlertResolverStub) Resolve(
	ctx context.Context,
	params domainrules.AlertResolutionParams,
) model.SiteAlertMode {
	if s == nil {
		return model.SiteAlertModeActive
	}
	s.Calls = append(s.Calls, params)
	if s.ResolveFn != nil {
		return s.ResolveFn(ctx, params)
	}
	return model.SiteAlertModeActive
}

var _ domainrules.AlertResolver = (*AlertResolverStub)(nil)

// EventFetcherStub is a reusable implementation of domainrules.EventFetcher for tests.
type EventFetcherStub struct {
	FetchFn func(ctx context.Context, params domainrules.EventFetchParams) ([]*model.Event, error)
	Calls   []domainrules.EventFetchParams
}

// Fetch records the invocation and delegates to FetchFn when provided.
func (s *EventFetcherStub) Fetch(
	ctx context.Context,
	params domainrules.EventFetchParams,
) ([]*model.Event, error) {
	if s == nil {
		return nil, nil
	}
	s.Calls = append(s.Calls, params)
	if s.FetchFn != nil {
		return s.FetchFn(ctx, params)
	}
	return nil, nil
}

var _ domainrules.EventFetcher = (*EventFetcherStub)(nil)

// PipelineStub captures calls to Pipeline.Run for assertions.
type PipelineStub struct {
	RunFn func(ctx context.Context, params domainrules.PipelineParams) (*domainrules.ProcessingResults, error)
	Calls []domainrules.PipelineParams
}

// Run records the invocation and delegates to RunFn when provided.
func (s *PipelineStub) Run(
	ctx context.Context,
	params domainrules.PipelineParams,
) (*domainrules.ProcessingResults, error) {
	if s == nil {
		return &domainrules.ProcessingResults{}, nil
	}
	s.Calls = append(s.Calls, params)
	if s.RunFn != nil {
		return s.RunFn(ctx, params)
	}
	return &domainrules.ProcessingResults{}, nil
}

var _ domainrules.Pipeline = (*PipelineStub)(nil)

// ResultStoreStub captures interactions with a ResultStore.
type ResultStoreStub struct {
	CacheFn   func(ctx context.Context, jobID string, results *domainrules.ProcessingResults) error
	PersistFn func(ctx context.Context, job *model.Job, results *domainrules.ProcessingResults) error
	GetFn     func(ctx context.Context, jobID string) (*domainrules.ProcessingResults, error)

	CacheCalls   []string
	PersistCalls []string
	GetCalls     []string
}

// Cache records the invocation and delegates to CacheFn when provided.
func (s *ResultStoreStub) Cache(
	ctx context.Context,
	jobID string,
	results *domainrules.ProcessingResults,
) error {
	if s == nil {
		return nil
	}
	s.CacheCalls = append(s.CacheCalls, jobID)
	if s.CacheFn != nil {
		return s.CacheFn(ctx, jobID, results)
	}
	return nil
}

// Persist records the invocation and delegates to PersistFn when provided.
func (s *ResultStoreStub) Persist(
	ctx context.Context,
	job *model.Job,
	results *domainrules.ProcessingResults,
) error {
	if s == nil {
		return nil
	}
	jobID := ""
	if job != nil {
		jobID = job.ID
	}
	s.PersistCalls = append(s.PersistCalls, jobID)
	if s.PersistFn != nil {
		return s.PersistFn(ctx, job, results)
	}
	return nil
}

// Get records the invocation and delegates to GetFn when provided.
func (s *ResultStoreStub) Get(ctx context.Context, jobID string) (*domainrules.ProcessingResults, error) {
	if s == nil {
		return nil, domainrules.ErrResultsNotFound
	}
	s.GetCalls = append(s.GetCalls, jobID)
	if s.GetFn != nil {
		return s.GetFn(ctx, jobID)
	}
	return nil, domainrules.ErrResultsNotFound
}

var _ domainrules.ResultStore = (*ResultStoreStub)(nil)

// JobCoordinatorStub captures orchestration calls for assertions.
type JobCoordinatorStub struct {
	BuildPayloadFn  func(req *domainrules.EnqueueJobRequest) ([]byte, error)
	ShouldProcessFn func(ctx context.Context, req *domainrules.EnqueueJobRequest) (bool, error)
	ParsePayloadFn  func(job *model.Job) (*domainrules.JobPayload, error)
	LimitEventIDsFn func(ids []string, jobID string) []string
}

// BuildPayload delegates to BuildPayloadFn when provided.
func (s *JobCoordinatorStub) BuildPayload(req *domainrules.EnqueueJobRequest) ([]byte, error) {
	if s == nil {
		return nil, nil
	}
	if s.BuildPayloadFn != nil {
		return s.BuildPayloadFn(req)
	}
	return nil, nil
}

// ShouldProcess delegates to ShouldProcessFn when provided.
func (s *JobCoordinatorStub) ShouldProcess(
	ctx context.Context,
	req *domainrules.EnqueueJobRequest,
) (bool, error) {
	if s == nil {
		return true, nil
	}
	if s.ShouldProcessFn != nil {
		return s.ShouldProcessFn(ctx, req)
	}
	return true, nil
}

// ParsePayload delegates to ParsePayloadFn when provided.
func (s *JobCoordinatorStub) ParsePayload(job *model.Job) (*domainrules.JobPayload, error) {
	if s == nil {
		return nil, errors.New("job coordinator stub: no payload")
	}
	if s.ParsePayloadFn != nil {
		return s.ParsePayloadFn(job)
	}
	return nil, errors.New("job coordinator stub: no payload")
}

// LimitEventIDs delegates to LimitEventIDsFn when provided.
func (s *JobCoordinatorStub) LimitEventIDs(ids []string, jobID string) []string {
	if s == nil {
		return ids
	}
	if s.LimitEventIDsFn != nil {
		return s.LimitEventIDsFn(ids, jobID)
	}
	return ids
}

var _ domainrules.JobCoordinator = (*JobCoordinatorStub)(nil)

// SiteRepositoryStub provides a test double for core.SiteRepository.
type SiteRepositoryStub struct {
	CreateFn    func(ctx context.Context, req *model.CreateSiteRequest) (*model.Site, error)
	GetByIDFn   func(ctx context.Context, id string) (*model.Site, error)
	GetByNameFn func(ctx context.Context, name string) (*model.Site, error)
	ListFn      func(ctx context.Context, limit, offset int) ([]*model.Site, error)
	UpdateFn    func(ctx context.Context, id string, req model.UpdateSiteRequest) (*model.Site, error)
	DeleteFn    func(ctx context.Context, id string) (bool, error)
}

func (s *SiteRepositoryStub) Create(
	ctx context.Context,
	req *model.CreateSiteRequest,
) (*model.Site, error) {
	if s != nil && s.CreateFn != nil {
		return s.CreateFn(ctx, req)
	}
	return nil, errNotImplemented
}

func (s *SiteRepositoryStub) GetByID(ctx context.Context, id string) (*model.Site, error) {
	if s != nil && s.GetByIDFn != nil {
		return s.GetByIDFn(ctx, id)
	}
	return nil, errNotImplemented
}

func (s *SiteRepositoryStub) GetByName(ctx context.Context, name string) (*model.Site, error) {
	if s != nil && s.GetByNameFn != nil {
		return s.GetByNameFn(ctx, name)
	}
	return nil, errNotImplemented
}

func (s *SiteRepositoryStub) List(
	ctx context.Context,
	limit, offset int,
) ([]*model.Site, error) {
	if s != nil && s.ListFn != nil {
		return s.ListFn(ctx, limit, offset)
	}
	return nil, errNotImplemented
}

func (s *SiteRepositoryStub) Update(
	ctx context.Context,
	id string,
	req model.UpdateSiteRequest,
) (*model.Site, error) {
	if s != nil && s.UpdateFn != nil {
		return s.UpdateFn(ctx, id, req)
	}
	return nil, errNotImplemented
}

func (s *SiteRepositoryStub) Delete(ctx context.Context, id string) (bool, error) {
	if s != nil && s.DeleteFn != nil {
		return s.DeleteFn(ctx, id)
	}
	return false, errNotImplemented
}

var _ core.SiteRepository = (*SiteRepositoryStub)(nil)

// EventRepositoryStub provides a test double for core.EventRepository.
type EventRepositoryStub struct {
	BulkInsertFn                    func(ctx context.Context, req model.BulkEventRequest, process bool) (int, error)
	BulkInsertWithProcessingFlagsFn func(
		ctx context.Context,
		req model.BulkEventRequest,
		shouldProcessMap map[int]bool,
	) (int, error)
	ListByJobFn          func(ctx context.Context, opts model.EventListByJobOptions) (*model.EventListPage, error)
	CountByJobFn         func(ctx context.Context, opts model.EventListByJobOptions) (int, error)
	GetByIDsFn           func(ctx context.Context, eventIDs []string) ([]*model.Event, error)
	MarkProcessedByIDsFn func(ctx context.Context, eventIDs []string) (int, error)
}

func (s *EventRepositoryStub) BulkInsert(
	ctx context.Context,
	req model.BulkEventRequest,
	process bool,
) (int, error) {
	if s != nil && s.BulkInsertFn != nil {
		return s.BulkInsertFn(ctx, req, process)
	}
	return 0, errNotImplemented
}

func (s *EventRepositoryStub) BulkInsertWithProcessingFlags(
	ctx context.Context,
	req model.BulkEventRequest,
	shouldProcessMap map[int]bool,
) (int, error) {
	if s != nil && s.BulkInsertWithProcessingFlagsFn != nil {
		return s.BulkInsertWithProcessingFlagsFn(ctx, req, shouldProcessMap)
	}
	return 0, errNotImplemented
}

func (s *EventRepositoryStub) ListByJob(
	ctx context.Context,
	opts model.EventListByJobOptions,
) (*model.EventListPage, error) {
	if s != nil && s.ListByJobFn != nil {
		return s.ListByJobFn(ctx, opts)
	}
	return nil, errNotImplemented
}

func (s *EventRepositoryStub) CountByJob(
	ctx context.Context,
	opts model.EventListByJobOptions,
) (int, error) {
	if s != nil && s.CountByJobFn != nil {
		return s.CountByJobFn(ctx, opts)
	}
	return 0, errNotImplemented
}

func (s *EventRepositoryStub) GetByIDs(
	ctx context.Context,
	eventIDs []string,
) ([]*model.Event, error) {
	if s != nil && s.GetByIDsFn != nil {
		return s.GetByIDsFn(ctx, eventIDs)
	}
	return nil, errNotImplemented
}

func (s *EventRepositoryStub) MarkProcessedByIDs(
	ctx context.Context,
	eventIDs []string,
) (int, error) {
	if s != nil && s.MarkProcessedByIDsFn != nil {
		return s.MarkProcessedByIDsFn(ctx, eventIDs)
	}
	return 0, errNotImplemented
}

var _ core.EventRepository = (*EventRepositoryStub)(nil)

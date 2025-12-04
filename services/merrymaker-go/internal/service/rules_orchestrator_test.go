package service

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	domainrules "github.com/target/mmk-ui-api/internal/domain/rules"
	rulestest "github.com/target/mmk-ui-api/internal/domain/rules/rulestest"
)

func TestProcessRulesJob_Success(t *testing.T) {
	t.Helper()

	eventRepo := &stubEventRepo{}
	cachedResults := make(map[string]*domainrules.ProcessingResults)
	persistedResults := make(map[string]*domainrules.ProcessingResults)

	pipeline := &rulestest.PipelineStub{
		RunFn: func(_ context.Context, params domainrules.PipelineParams) (*domainrules.ProcessingResults, error) {
			return &domainrules.ProcessingResults{AlertsCreated: 1}, nil
		},
	}

	results := &rulestest.ResultStoreStub{
		CacheFn: func(_ context.Context, jobID string, res *domainrules.ProcessingResults) error {
			cachedResults[jobID] = cloneResults(res)
			return nil
		},
		PersistFn: func(_ context.Context, job *model.Job, res *domainrules.ProcessingResults) error {
			if job == nil {
				return errors.New("job is nil")
			}
			persistedResults[job.ID] = cloneResults(res)
			return nil
		},
	}

	resolver := &rulestest.AlertResolverStub{
		ResolveFn: func(_ context.Context, params domainrules.AlertResolutionParams) model.SiteAlertMode {
			return model.SiteAlertModeMuted
		},
	}

	fetcher := &rulestest.EventFetcherStub{
		FetchFn: func(_ context.Context, params domainrules.EventFetchParams) ([]*model.Event, error) {
			return []*model.Event{{ID: "evt-1"}}, nil
		},
	}

	limitCount := 0
	coordinator := &rulestest.JobCoordinatorStub{
		ParsePayloadFn: func(job *model.Job) (*domainrules.JobPayload, error) {
			if job == nil {
				return nil, errors.New("job is nil")
			}
			return &domainrules.JobPayload{
				EventIDs: []string{"evt-1"},
				SiteID:   "site-1",
				Scope:    "default",
			}, nil
		},
		LimitEventIDsFn: func(ids []string, _ string) []string {
			limitCount++
			return ids
		},
	}

	service := &RulesOrchestrationService{
		events:        eventRepo,
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		coordinator:   coordinator,
		results:       results,
		pipeline:      pipeline,
		alertResolver: resolver,
		eventFetcher:  fetcher,
	}

	job := &model.Job{ID: "job-success"}

	err := service.ProcessRulesJob(context.Background(), job)
	require.NoError(t, err)

	require.Len(t, pipeline.Calls, 1)
	assert.Equal(t, model.SiteAlertModeMuted, pipeline.Calls[0].AlertMode)
	require.Len(t, fetcher.Calls, 1)
	assert.Equal(t, []string{"evt-1"}, fetcher.Calls[0].EventIDs)
	require.Len(t, resolver.Calls, 1)
	assert.Equal(t, "job-success", resolver.Calls[0].JobID)
	assert.Equal(t, [][]string{{"evt-1"}}, eventRepo.markCalls)
	assert.Equal(t, 1, limitCount)

	cached := cachedResults["job-success"]
	require.NotNil(t, cached)
	assert.Equal(t, 1, cached.AlertsCreated)

	persisted := persistedResults["job-success"]
	require.NotNil(t, persisted)
	assert.Equal(t, 1, persisted.AlertsCreated)
}

func TestProcessRulesJob_PipelineError(t *testing.T) {
	t.Helper()

	eventRepo := &stubEventRepo{}
	results := &rulestest.ResultStoreStub{}
	pipeline := &rulestest.PipelineStub{
		RunFn: func(context.Context, domainrules.PipelineParams) (*domainrules.ProcessingResults, error) {
			return nil, errors.New("pipeline failed")
		},
	}
	fetcher := &rulestest.EventFetcherStub{
		FetchFn: func(_ context.Context, params domainrules.EventFetchParams) ([]*model.Event, error) {
			return []*model.Event{{ID: "evt-1"}}, nil
		},
	}
	limitCount := 0
	coordinator := &rulestest.JobCoordinatorStub{
		ParsePayloadFn: func(_ *model.Job) (*domainrules.JobPayload, error) {
			return &domainrules.JobPayload{
				EventIDs: []string{"evt-1"},
			}, nil
		},
		LimitEventIDsFn: func(ids []string, _ string) []string {
			limitCount++
			return ids
		},
	}

	service := &RulesOrchestrationService{
		events:        eventRepo,
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		coordinator:   coordinator,
		results:       results,
		pipeline:      pipeline,
		alertResolver: &rulestest.AlertResolverStub{},
		eventFetcher:  fetcher,
	}

	err := service.ProcessRulesJob(context.Background(), &model.Job{ID: "job-error"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "pipeline failed")
	assert.Empty(t, eventRepo.markCalls)
	assert.Empty(t, results.CacheCalls)
	assert.Empty(t, results.PersistCalls)
	assert.Equal(t, 1, limitCount)
}

func TestProcessRulesJob_NoEventsFinalizesEmpty(t *testing.T) {
	t.Helper()

	eventRepo := &stubEventRepo{}
	cachedResults := make(map[string]*domainrules.ProcessingResults)
	persistedResults := make(map[string]*domainrules.ProcessingResults)
	results := &rulestest.ResultStoreStub{
		CacheFn: func(_ context.Context, jobID string, res *domainrules.ProcessingResults) error {
			cachedResults[jobID] = cloneResults(res)
			return nil
		},
		PersistFn: func(_ context.Context, job *model.Job, res *domainrules.ProcessingResults) error {
			if job == nil {
				return errors.New("job is nil")
			}
			persistedResults[job.ID] = cloneResults(res)
			return nil
		},
	}
	coordinator := &rulestest.JobCoordinatorStub{
		ParsePayloadFn: func(_ *model.Job) (*domainrules.JobPayload, error) {
			return &domainrules.JobPayload{
				EventIDs: []string{"missing"},
			}, nil
		},
		LimitEventIDsFn: func(ids []string, _ string) []string {
			return ids
		},
	}
	fetcher := &rulestest.EventFetcherStub{
		FetchFn: func(_ context.Context, params domainrules.EventFetchParams) ([]*model.Event, error) {
			return nil, nil
		},
	}

	pipeline := &rulestest.PipelineStub{}
	service := &RulesOrchestrationService{
		events:        eventRepo,
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		coordinator:   coordinator,
		results:       results,
		pipeline:      pipeline,
		alertResolver: &rulestest.AlertResolverStub{},
		eventFetcher:  fetcher,
	}

	job := &model.Job{ID: "job-empty", IsTest: true}
	err := service.ProcessRulesJob(context.Background(), job)
	require.NoError(t, err)

	assert.Empty(t, eventRepo.markCalls)
	require.NotEmpty(t, results.CacheCalls)
	require.NotEmpty(t, results.PersistCalls)
	assert.Equal(t, []string{"job-empty"}, results.CacheCalls)
	assert.Equal(t, []string{"job-empty"}, results.PersistCalls)
	require.Len(t, fetcher.Calls, 1)
	assert.Equal(t, []string{"missing"}, fetcher.Calls[0].EventIDs)
	assert.Empty(t, pipeline.Calls)

	cached := cachedResults["job-empty"]
	require.NotNil(t, cached)
	assert.True(t, cached.IsDryRun)
	assert.Zero(t, cached.DomainsProcessed)
	assert.Zero(t, cached.AlertsCreated)

	persisted := persistedResults["job-empty"]
	require.NotNil(t, persisted)
	assert.True(t, persisted.IsDryRun)
}

type stubEventRepo struct {
	markCalls [][]string
}

func (s *stubEventRepo) BulkInsert(context.Context, model.BulkEventRequest, bool) (int, error) {
	return 0, nil
}

func (s *stubEventRepo) BulkInsertWithProcessingFlags(
	context.Context,
	model.BulkEventRequest,
	map[int]bool,
) (int, error) {
	return 0, nil
}

func (s *stubEventRepo) ListByJob(
	context.Context,
	model.EventListByJobOptions,
) (*model.EventListPage, error) {
	return &model.EventListPage{Events: nil}, nil
}

func (s *stubEventRepo) CountByJob(context.Context, model.EventListByJobOptions) (int, error) {
	return 0, nil
}

func (s *stubEventRepo) GetByIDs(context.Context, []string) ([]*model.Event, error) {
	return nil, nil
}

func (s *stubEventRepo) MarkProcessedByIDs(_ context.Context, eventIDs []string) (int, error) {
	s.markCalls = append(s.markCalls, append([]string(nil), eventIDs...))
	return len(eventIDs), nil
}

func cloneResults(results *domainrules.ProcessingResults) *domainrules.ProcessingResults {
	if results == nil {
		return nil
	}
	resCopy := *results
	resCopy.WouldAlertIOC = append([]string(nil), results.WouldAlertIOC...)
	resCopy.WouldAlertUnknown = append([]string(nil), results.WouldAlertUnknown...)
	return &resCopy
}

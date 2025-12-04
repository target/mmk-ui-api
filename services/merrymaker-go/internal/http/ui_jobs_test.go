package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
)

var (
	errNotImplemented = errors.New("not implemented in mock")
	// ErrNotFound is a sentinel error for "not found" cases in mocks.
	errNotFound = errors.New("not found")
)

func TestCalcRetriesAllowed(t *testing.T) {
	cases := []struct {
		name       string
		maxRetries int
		want       int
	}{
		{name: "negative", maxRetries: -1, want: 0},
		{name: "zero", maxRetries: 0, want: 0},
		{name: "one", maxRetries: 1, want: 0},
		{name: "two", maxRetries: 2, want: 1},
		{name: "three", maxRetries: 3, want: 2},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, calcRetriesAllowed(tc.maxRetries))
		})
	}
}

// mockJobReadService implements JobReadService for testing.
type mockJobReadService struct {
	jobs map[string]*model.Job
}

func (m *mockJobReadService) ListRecentByType(
	ctx context.Context,
	jobType model.JobType,
	limit int,
) ([]*model.Job, error) {
	return nil, errNotImplemented
}

func (m *mockJobReadService) ListRecentByTypeWithSiteNames(
	ctx context.Context,
	jobType model.JobType,
	limit int,
) ([]*model.JobWithEventCount, error) {
	return nil, errNotImplemented
}

func (m *mockJobReadService) Create(ctx context.Context, req *model.CreateJobRequest) (*model.Job, error) {
	return nil, errNotImplemented
}

func (m *mockJobReadService) GetByID(ctx context.Context, id string) (*model.Job, error) {
	if job, exists := m.jobs[id]; exists {
		return job, nil
	}
	// Return sentinel error to represent "not found" (clearer semantics than nil, nil)
	return nil, errNotFound
}

func (m *mockJobReadService) Stats(ctx context.Context, jobType model.JobType) (*model.JobStats, error) {
	return nil, errNotImplemented
}

func (m *mockJobReadService) List(ctx context.Context, opts *model.JobListOptions) ([]*model.JobWithEventCount, error) {
	return nil, errNotImplemented
}

func (m *mockJobReadService) Delete(ctx context.Context, id string) error {
	return errNotImplemented
}

// mockSitesService implements SitesService for testing.
type mockSitesService struct {
	sites map[string]*model.Site
}

func (m *mockSitesService) List(ctx context.Context, limit, offset int) ([]*model.Site, error) {
	return nil, errNotImplemented
}

func (m *mockSitesService) GetByID(ctx context.Context, id string) (*model.Site, error) {
	if site, exists := m.sites[id]; exists {
		return site, nil
	}
	// Return sentinel error to represent "not found" (clearer semantics than nil, nil)
	return nil, errNotFound
}

func (m *mockSitesService) Create(ctx context.Context, req *model.CreateSiteRequest) (*model.Site, error) {
	return nil, errNotImplemented
}

func (m *mockSitesService) Update(ctx context.Context, id string, req model.UpdateSiteRequest) (*model.Site, error) {
	return nil, errNotImplemented
}

func (m *mockSitesService) Delete(ctx context.Context, id string) (bool, error) {
	return false, errNotImplemented
}

// mockSourcesService implements SourcesService for testing.
type mockSourcesService struct {
	sources map[string]*model.Source
}

func (m *mockSourcesService) List(ctx context.Context, limit, offset int) ([]*model.Source, error) {
	return nil, errNotImplemented
}

func (m *mockSourcesService) GetByID(ctx context.Context, id string) (*model.Source, error) {
	if source, exists := m.sources[id]; exists {
		return source, nil
	}
	// Return sentinel error to represent "not found" (clearer semantics than nil, nil)
	return nil, errNotFound
}

func (m *mockSourcesService) Create(ctx context.Context, req *model.CreateSourceRequest) (*model.Source, error) {
	return nil, errNotImplemented
}

func (m *mockSourcesService) Delete(ctx context.Context, id string) (bool, error) {
	return false, errNotImplemented
}

func (m *mockSourcesService) ResolveScript(ctx context.Context, source *model.Source) (string, error) {
	if source == nil {
		return "", errNotFound
	}
	return source.Value, nil
}

func (m *mockSourcesService) CountJobsBySource(ctx context.Context, sourceID string, includeTests bool) (int, error) {
	return 0, errNotImplemented
}

func (m *mockSourcesService) CountBrowserJobsBySource(
	ctx context.Context,
	sourceID string,
	includeTests bool,
) (int, error) {
	return 0, errNotImplemented
}

type mockJobResultRepo struct {
	results map[string]*model.JobResult
	err     error
}

func (m *mockJobResultRepo) Upsert(ctx context.Context, params core.UpsertJobResultParams) error {
	return errNotImplemented
}

func (m *mockJobResultRepo) GetByJobID(ctx context.Context, jobID string) (*model.JobResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	if job, ok := m.results[jobID]; ok {
		return job, nil
	}
	return nil, data.ErrJobResultsNotFound
}

func (m *mockJobResultRepo) ListByAlertID(ctx context.Context, alertID string) ([]*model.JobResult, error) {
	return nil, errNotImplemented
}

type stubEventsService struct {
	listResp   *model.EventListPage
	count      int
	listErr    error
	countErr   error
	lastOpts   model.EventListByJobOptions
	eventsByID map[string]*model.Event
}

func (s *stubEventsService) ListByJob(
	ctx context.Context,
	opts model.EventListByJobOptions,
) (*model.EventListPage, error) {
	s.lastOpts = opts
	if s.listErr != nil {
		return nil, s.listErr
	}
	if s.listResp != nil {
		return s.listResp, nil
	}
	return &model.EventListPage{}, nil
}

func (s *stubEventsService) CountByJob(ctx context.Context, opts model.EventListByJobOptions) (int, error) {
	s.lastOpts = opts
	if s.countErr != nil {
		return 0, s.countErr
	}
	return s.count, nil
}

func (s *stubEventsService) GetByIDs(ctx context.Context, ids []string) ([]*model.Event, error) {
	if len(ids) == 0 {
		return []*model.Event{}, nil
	}
	out := make([]*model.Event, 0, len(ids))
	for _, id := range ids {
		if ev, ok := s.eventsByID[id]; ok {
			out = append(out, ev)
		}
	}
	return out, nil
}

func TestUIHandlers_JobView_ShowsBasicJobInfo(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	// Create a mock job
	now := time.Now()
	completedAt := now.Add(5 * time.Minute)
	mockJob := &model.Job{
		ID:          "test-job-123",
		Type:        model.JobTypeBrowser,
		Status:      "completed",
		Priority:    50,
		IsTest:      true,
		CreatedAt:   now,
		CompletedAt: &completedAt,
		LastError:   nil,
	}

	mockJobService := &mockJobReadService{
		jobs: map[string]*model.Job{
			"test-job-123": mockJob,
		},
	}

	h := &UIHandlers{
		T:    tr,
		Jobs: mockJobService,
	}

	// Test the JobView handler
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/jobs/test-job-123", nil)
	r.SetPathValue("id", "test-job-123")

	h.JobView(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Check that basic job information is displayed
	assert.Contains(t, body, "test-job-123")
	assert.Contains(t, body, "browser")
	assert.Contains(t, body, "Completed")  // Status badge shows "Completed" (capitalized)
	assert.Contains(t, body, "50")         // priority
	assert.Contains(t, body, "Test Run")   // Test run field in expanded details
	assert.Contains(t, body, "badge-info") // Test badge
}

func TestUIHandlers_JobView_ShowsFailedJobWithError(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	// Create a mock failed job
	now := time.Now()
	errorMsg := "Connection timeout"
	mockJob := &model.Job{
		ID:        "failed-job-456",
		Type:      model.JobTypeRules,
		Status:    "failed",
		Priority:  10,
		IsTest:    false,
		CreatedAt: now,
		LastError: &errorMsg,
	}

	mockJobService := &mockJobReadService{
		jobs: map[string]*model.Job{
			"failed-job-456": mockJob,
		},
	}

	h := &UIHandlers{
		T:    tr,
		Jobs: mockJobService,
	}

	// Test the JobView handler
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/jobs/failed-job-456", nil)
	r.SetPathValue("id", "failed-job-456")

	h.JobView(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Check that error information is displayed
	assert.Contains(t, body, "failed-job-456")
	assert.Contains(t, body, "rules")
	assert.Contains(t, body, "failed")
	assert.Contains(t, body, "Connection timeout")
	assert.NotContains(t, body, "<strong>Test Run:</strong> Yes") // should not show for non-test jobs
}

func TestUIHandlers_JobView_HandlesNonExistentJob(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	mockJobService := &mockJobReadService{
		jobs: map[string]*model.Job{},
	}

	h := &UIHandlers{
		T:    tr,
		Jobs: mockJobService,
	}

	// Test the JobView handler with non-existent job
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/jobs/nonexistent", nil)
	r.SetPathValue("id", "nonexistent")

	h.JobView(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Check that appropriate message is displayed
	assert.Contains(t, body, "Job not found or unable to load job information")
}

func TestUIHandlers_JobView_ShowsSiteAndSourceContext(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	// Create mock data
	now := time.Now()
	siteID := "site-123"
	sourceID := "source-456"
	scope := "production"

	mockJob := &model.Job{
		ID:        "job-with-context",
		Type:      model.JobTypeBrowser,
		Status:    "completed",
		Priority:  50,
		IsTest:    false,
		SiteID:    &siteID,
		SourceID:  &sourceID,
		CreatedAt: now,
	}

	mockSite := &model.Site{
		ID:              siteID,
		Name:            "Example Site",
		Enabled:         true,
		Scope:           &scope,
		RunEveryMinutes: 15,
		SourceID:        sourceID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	mockSource := &model.Source{
		ID:        sourceID,
		Name:      "Example Source",
		Value:     "async function run() { await page.goto('https://example.com'); }",
		Test:      false,
		CreatedAt: now,
	}

	mockJobService := &mockJobReadService{
		jobs: map[string]*model.Job{
			"job-with-context": mockJob,
		},
	}

	mockSiteService := &mockSitesService{
		sites: map[string]*model.Site{
			siteID: mockSite,
		},
	}

	mockSourceService := &mockSourcesService{
		sources: map[string]*model.Source{
			sourceID: mockSource,
		},
	}

	h := &UIHandlers{
		T:         tr,
		Jobs:      mockJobService,
		SiteSvc:   mockSiteService,
		SourceSvc: mockSourceService,
	}

	// Test the JobView handler
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/jobs/job-with-context", nil)
	r.SetPathValue("id", "job-with-context")

	h.JobView(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	// Check that Context section is displayed (in expanded details)
	assert.Contains(t, body, "Context")

	// Check Site information
	assert.Contains(t, body, "Example Site")
	assert.Contains(t, body, "production")
	assert.Contains(t, body, "15 minutes")
	assert.Contains(t, body, "/sites/"+siteID)

	// Check Source information
	assert.Contains(t, body, "Example Source")
	assert.Contains(t, body, "Script Preview")
	assert.Contains(t, body, "/sources?highlight="+sourceID)
}

func TestUIHandlers_JobView_ShowsAlertDeliveryPanel(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	now := time.Now().UTC()
	job := &model.Job{
		ID:        "alert-job-1",
		Type:      model.JobTypeAlert,
		Status:    model.JobStatusCompleted,
		CreatedAt: now,
	}

	payload := service.AlertDeliveryJobResult{
		JobID:         job.ID,
		SinkID:        "sink-123",
		SinkName:      "PagerDuty",
		JobStatus:     model.JobStatusCompleted,
		AttemptNumber: 1,
		AttemptedAt:   now,
		DurationMs:    1200,
		Payload:       json.RawMessage(`{"alert":"test"}`),
		Request: service.AlertDeliveryRequestSummary{
			Method:   "POST",
			URL:      "https://alerts.example.com/hook",
			Headers:  map[string]string{"Content-Type": "application/json"},
			Body:     `{"foo":"bar"}`,
			OkStatus: 200,
		},
		Response: &service.AlertDeliveryResponse{
			StatusCode: 202,
			Headers:    map[string]string{"X-Request-ID": "abc123"},
			Body:       "ok",
		},
	}
	resultBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	jobResults := &mockJobResultRepo{
		results: map[string]*model.JobResult{
			job.ID: {
				JobID:     &job.ID,
				JobType:   model.JobTypeAlert,
				Result:    resultBytes,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	h := &UIHandlers{
		T:          tr,
		Jobs:       &mockJobReadService{jobs: map[string]*model.Job{job.ID: job}},
		JobResults: jobResults,
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/jobs/"+job.ID, nil)
	r.SetPathValue("id", job.ID)

	h.JobView(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	assert.Contains(t, body, "Alert Delivery Result")
	assert.Contains(t, body, "Status: completed")
	assert.Contains(t, body, "Attempt 1")
	assert.Contains(t, body, "https://alerts.example.com/hook")
	assert.Contains(t, body, "Content-Type")
	assert.Contains(t, body, "&#34;alert&#34;: &#34;test&#34;")
	assert.Contains(t, body, "X-Request-ID")
}

func TestUIHandlers_JobView_HidesAttemptedWhenZero(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	job := &model.Job{
		ID:        "alert-job-no-attempt",
		Type:      model.JobTypeAlert,
		Status:    model.JobStatusCompleted,
		CreatedAt: time.Now().UTC(),
	}

	payload := service.AlertDeliveryJobResult{
		JobID:         job.ID,
		SinkID:        "sink-zero",
		JobStatus:     model.JobStatusCompleted,
		AttemptNumber: 1,
		AttemptedAt:   time.Time{},
		Request: service.AlertDeliveryRequestSummary{
			Method: "POST",
			URL:    "https://alerts.example.com/hook",
		},
	}
	resultBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	jobResults := &mockJobResultRepo{
		results: map[string]*model.JobResult{
			job.ID: {
				JobID:     &job.ID,
				JobType:   model.JobTypeAlert,
				Result:    resultBytes,
				CreatedAt: job.CreatedAt,
				UpdatedAt: job.CreatedAt,
			},
		},
	}

	h := &UIHandlers{
		T:          tr,
		Jobs:       &mockJobReadService{jobs: map[string]*model.Job{job.ID: job}},
		JobResults: jobResults,
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/jobs/"+job.ID, nil)
	r.SetPathValue("id", job.ID)

	h.JobView(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	assert.NotContains(t, body, "<dt>Attempted</dt>")
}

func TestUIHandlers_JobView_ShowsRequestBodyTruncationMessage(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	now := time.Now().UTC()
	job := &model.Job{
		ID:        "alert-job-truncated-request",
		Type:      model.JobTypeAlert,
		Status:    model.JobStatusCompleted,
		CreatedAt: now,
	}

	payload := service.AlertDeliveryJobResult{
		JobID:         job.ID,
		SinkID:        "sink-trunc",
		JobStatus:     model.JobStatusCompleted,
		AttemptNumber: 2,
		AttemptedAt:   now,
		Request: service.AlertDeliveryRequestSummary{
			Method:        "POST",
			URL:           "https://alerts.example.com/hook",
			Body:          "{\"example\":\"truncated\"}",
			BodyTruncated: true,
		},
	}
	resultBytes, err := json.Marshal(payload)
	require.NoError(t, err)

	jobResults := &mockJobResultRepo{
		results: map[string]*model.JobResult{
			job.ID: {
				JobID:     &job.ID,
				JobType:   model.JobTypeAlert,
				Result:    resultBytes,
				CreatedAt: now,
				UpdatedAt: now,
			},
		},
	}

	h := &UIHandlers{
		T:          tr,
		Jobs:       &mockJobReadService{jobs: map[string]*model.Job{job.ID: job}},
		JobResults: jobResults,
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/jobs/"+job.ID, nil)
	r.SetPathValue("id", job.ID)

	h.JobView(w, r)

	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()

	assert.Contains(t, body, "Body truncated to first 4KB.")
}

func TestBuildRulesSummaryCards_WithMutedAlerts(t *testing.T) {
	res := &service.RulesProcessingResults{
		AlertsCreated: 3,
		AlertMode:     model.SiteAlertModeMuted,
	}
	res.UnknownDomain.AlertedMuted.Count = 2
	res.IOC.AlertsMuted.Count = 1

	cards := buildRulesSummaryCards(res, model.SiteAlertModeMuted)

	require.Len(t, cards, 4)
	assert.Equal(t, "Alerts Delivered", cards[0].Title)
	assert.Equal(t, 0, cards[0].Value)
	assert.Equal(t, 3, cards[1].Value)
	assert.Equal(t, "Site alert mode: Muted", cards[1].Context)
}

func TestBuildRulesResultsView_UsesProvidedAlertMode(t *testing.T) {
	res := &service.RulesProcessingResults{}

	view := buildRulesResultsView(res, model.SiteAlertModeMuted)

	require.NotNil(t, view)
	assert.Equal(t, model.SiteAlertModeMuted, view.AlertMode)
	assert.Equal(t, "muted", view.AlertModeStr)
	assert.True(t, view.IsMutedMode)
	require.Len(t, view.Summary, 4)
	assert.Equal(t, "Site alert mode: Muted", view.Summary[1].Context)
}

func TestBuildRulesResultsView_NormalizesLegacyAlertMode(t *testing.T) {
	res := &service.RulesProcessingResults{
		AlertsCreated: 2,
		AlertMode:     model.SiteAlertMode("Muted"),
	}
	res.UnknownDomain.AlertedMuted.Count = 2

	view := buildRulesResultsView(res, "")

	require.NotNil(t, view)
	assert.Equal(t, model.SiteAlertModeMuted, res.AlertMode)
	assert.Equal(t, model.SiteAlertModeMuted, view.AlertMode)
	assert.Equal(t, "muted", view.AlertModeStr)
	assert.True(t, view.IsMutedMode)
	assert.Equal(t, 2, view.MutedAlerts)
	require.Len(t, view.Summary, 4)
	assert.Equal(t, "Site alert mode: Muted", view.Summary[1].Context)
}

func TestExtractSiteAlertMode_NormalizesValues(t *testing.T) {
	mode := extractSiteAlertMode(&siteView{
		Site: &model.Site{AlertMode: model.SiteAlertMode("Muted")},
	})

	assert.Equal(t, model.SiteAlertModeMuted, mode)
}

func TestCalcRulesAlertCounts_FallbackWhenMutedAndNoMetrics(t *testing.T) {
	res := &service.RulesProcessingResults{
		AlertsCreated: 5,
	}

	delivered, muted := calcRulesAlertCounts(res, model.SiteAlertModeMuted)

	assert.Equal(t, 0, delivered)
	assert.Equal(t, 5, muted)
}

func TestMapToHeaderList_SortsCaseInsensitive(t *testing.T) {
	headers := map[string]string{
		"X-Trace-ID":   "123",
		"accept":       "*/*",
		"Content-Type": "application/json",
	}

	list := mapToHeaderList(headers)

	require.Len(t, list, 3)
	assert.Equal(t, headerKV{Key: "accept", Value: "*/*"}, list[0])
	assert.Equal(t, headerKV{Key: "Content-Type", Value: "application/json"}, list[1])
	assert.Equal(t, headerKV{Key: "X-Trace-ID", Value: "123"}, list[2])
}

func TestJobEvents_CursorLinksIncludeFilters(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	jobID := "job-test"
	start := time.Unix(1700000000, 0)
	ev1 := &model.Event{ID: "evt-1", EventType: "console.log", CreatedAt: start, SourceJobID: strPtr(jobID)}
	ev2 := &model.Event{
		ID:          "evt-2",
		EventType:   "console.log",
		CreatedAt:   start.Add(time.Second),
		SourceJobID: strPtr(jobID),
	}

	nextCursor, err := data.EncodeEventCursorFromEvent(ev1, "timestamp", "asc")
	require.NoError(t, err)

	svc := &stubEventsService{
		listResp: &model.EventListPage{Events: []*model.Event{ev1, ev2}},
	}

	h := &UIHandlers{
		T:        tr,
		EventSvc: svc,
	}

	req := httptest.NewRequest(http.MethodGet, "/jobs/"+jobID+"/events?page_size=1&q=hello&category=console", nil)
	req.SetPathValue("id", jobID)
	w := httptest.NewRecorder()

	h.JobEvents(w, req)

	require.Equal(t, http.StatusOK, w.Result().StatusCode)
	body := w.Body.String()
	require.Contains(t, body, "cursor_after="+url.QueryEscape(nextCursor))
	require.Contains(t, body, "q=hello")
	require.Contains(t, body, "category=console")
	require.Contains(t, body, "index_offset=1")
}

func TestJobEvents_PrevLinkCarriesCursorAndIndex(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	jobID := "job-test"
	start := time.Unix(1700000100, 0)
	ev1 := &model.Event{ID: "evt-10", EventType: "network.request", CreatedAt: start, SourceJobID: strPtr(jobID)}
	ev2 := &model.Event{
		ID:          "evt-11",
		EventType:   "network.request",
		CreatedAt:   start.Add(time.Second),
		SourceJobID: strPtr(jobID),
	}
	prevCursor := "prev-token"
	nextCursor := "next-token"

	svc := &stubEventsService{
		listResp: &model.EventListPage{
			Events:     []*model.Event{ev1, ev2},
			NextCursor: &nextCursor,
			PrevCursor: &prevCursor,
		},
	}

	h := &UIHandlers{
		T:        tr,
		EventSvc: svc,
	}

	req := httptest.NewRequest(
		http.MethodGet,
		"/jobs/"+jobID+"/events?cursor_after=fwd-token&page_size=2&index_offset=4&sort=timestamp:desc&category=network",
		nil,
	)
	req.SetPathValue("id", jobID)
	w := httptest.NewRecorder()

	h.JobEvents(w, req)

	require.Equal(t, http.StatusOK, w.Result().StatusCode)
	body := w.Body.String()
	require.Contains(t, body, "cursor_before="+url.QueryEscape(prevCursor))
	require.Contains(t, body, "cursor_after="+url.QueryEscape(nextCursor))
	require.Contains(t, body, "index_offset=6")
	require.Contains(t, body, "index_offset=2")
	require.Contains(t, body, "category=network")
}

func strPtr(v string) *string {
	return &v
}

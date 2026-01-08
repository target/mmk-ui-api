package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	alertsvm "github.com/target/mmk-ui-api/internal/http/ui/alerts"
	"github.com/target/mmk-ui-api/internal/service"
)

var errAlertNotFound = errors.New("alert not found")

// mockAlertsService is a mock implementation of AlertsService for testing.
type mockAlertsService struct {
	alerts []*model.Alert
	err    error
}

func (m *mockAlertsService) List(_ context.Context, opts *model.AlertListOptions) ([]*model.Alert, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.alerts, nil
}

func (m *mockAlertsService) ListWithSiteNames(
	_ context.Context,
	opts *model.AlertListOptions,
) ([]*model.AlertWithSiteName, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Convert alerts to AlertWithSiteName for testing
	alertsWithSiteNames := make([]*model.AlertWithSiteName, len(m.alerts))
	for i, alert := range m.alerts {
		alertsWithSiteNames[i] = &model.AlertWithSiteName{
			Alert:    *alert,
			SiteName: "Test Site", // Mock site name
		}
	}
	return alertsWithSiteNames, nil
}

func (m *mockAlertsService) ListWithSiteNamesAndCount(
	_ context.Context,
	opts *model.AlertListOptions,
) (*model.AlertListResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	// Convert alerts to AlertWithSiteName for testing
	alertsWithSiteNames := make([]*model.AlertWithSiteName, len(m.alerts))
	for i, alert := range m.alerts {
		alertsWithSiteNames[i] = &model.AlertWithSiteName{
			Alert:    *alert,
			SiteName: "Test Site", // Mock site name
		}
	}
	return &model.AlertListResult{
		Alerts: alertsWithSiteNames,
		Total:  len(m.alerts),
	}, nil
}

func (m *mockAlertsService) Count(_ context.Context, opts *model.AlertListOptions) (int, error) {
	if m.err != nil {
		return 0, m.err
	}
	return len(m.alerts), nil
}

func (m *mockAlertsService) GetByID(_ context.Context, id string) (*model.Alert, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, alert := range m.alerts {
		if alert.ID == id {
			return alert, nil
		}
	}
	return nil, errAlertNotFound
}

func (m *mockAlertsService) Delete(_ context.Context, _ string) (bool, error) {
	return true, m.err
}

func (m *mockAlertsService) Resolve(_ context.Context, params core.ResolveAlertParams) (*model.Alert, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, alert := range m.alerts {
		if alert.ID == params.ID {
			now := time.Now()
			alert.ResolvedAt = &now
			resolvedBy := params.ResolvedBy
			alert.ResolvedBy = &resolvedBy
			return alert, nil
		}
	}
	return nil, errAlertNotFound
}

// mockSitesServiceForAlerts is a mock implementation of SitesService for alerts testing.
type mockSitesServiceForAlerts struct {
	sites []*model.Site
	err   error
}

func (m *mockSitesServiceForAlerts) List(_ context.Context, _, _ int) ([]*model.Site, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.sites, nil
}

func (m *mockSitesServiceForAlerts) GetByID(_ context.Context, id string) (*model.Site, error) {
	if m.err != nil {
		return nil, m.err
	}
	for _, site := range m.sites {
		if site.ID == id {
			return site, nil
		}
	}
	return nil, errAlertNotFound
}

func (m *mockSitesServiceForAlerts) Create(_ context.Context, _ *model.CreateSiteRequest) (*model.Site, error) {
	return nil, m.err
}

func (m *mockSitesServiceForAlerts) Update(
	_ context.Context,
	_ string,
	_ model.UpdateSiteRequest,
) (*model.Site, error) {
	return nil, m.err
}

func (m *mockSitesServiceForAlerts) Delete(_ context.Context, _ string) (bool, error) {
	return true, m.err
}

func TestParseAlertsFilter(t *testing.T) {
	tests := []struct {
		name     string
		query    url.Values
		expected alertsFilter
	}{
		{
			name:  "empty query",
			query: url.Values{},
			expected: alertsFilter{
				Sort: "fired_at",
				Dir:  "desc",
			},
		},
		{
			name: "all filters",
			query: url.Values{
				"site_id":    []string{"site-123"},
				"severity":   []string{"critical"},
				"rule_type":  []string{"unknown_domain"},
				"unresolved": []string{"true"},
				"sort":       []string{"severity"},
				"dir":        []string{"asc"},
			},
			expected: alertsFilter{
				SiteID:     "site-123",
				Severity:   "critical",
				RuleType:   "unknown_domain",
				Unresolved: true,
				Sort:       "severity",
				Dir:        "asc",
			},
		},
		{
			name: "partial filters",
			query: url.Values{
				"severity":   []string{"high"},
				"unresolved": []string{"true"},
			},
			expected: alertsFilter{
				Severity:   "high",
				Unresolved: true,
				Sort:       "fired_at",
				Dir:        "desc",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseAlertsFilter(tt.query)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAlertRowFriendlyFiredAt(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name     string
		time     time.Time
		expected string
	}{
		{
			name:     "just now",
			time:     now.Add(-30 * time.Second),
			expected: "just now",
		},
		{
			name:     "1 minute ago",
			time:     now.Add(-1 * time.Minute),
			expected: "1 minute ago",
		},
		{
			name:     "5 minutes ago",
			time:     now.Add(-5 * time.Minute),
			expected: "5 minutes ago",
		},
		{
			name:     "1 hour ago",
			time:     now.Add(-1 * time.Hour),
			expected: "1 hour ago",
		},
		{
			name:     "3 hours ago",
			time:     now.Add(-3 * time.Hour),
			expected: "3 hours ago",
		},
		{
			name:     "1 day ago",
			time:     now.Add(-24 * time.Hour),
			expected: "1 day ago",
		},
		{
			name:     "3 days ago",
			time:     now.Add(-72 * time.Hour),
			expected: "3 days ago",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			row := alertsvm.AlertRow{FiredAt: tt.time}
			result := row.FriendlyFiredAt()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestToAlertRowFromAlertWithSiteName_NormalizesSiteAlertMode(t *testing.T) {
	h := &UIHandlers{}
	alert := &model.Alert{
		ID:             "alert-1",
		SiteID:         "site-1",
		RuleType:       "unknown_domain",
		Severity:       "high",
		Title:          "Suspicious Domain",
		Description:    "example.com was observed",
		DeliveryStatus: model.AlertDeliveryStatusMuted,
		FiredAt:        time.Now(),
	}

	row := h.toAlertRowFromAlertWithSiteName(&model.AlertWithSiteName{
		Alert:         *alert,
		SiteName:      "Legacy Site",
		SiteAlertMode: model.SiteAlertMode("Muted"),
	})

	assert.Equal(t, model.SiteAlertModeMuted, row.SiteAlertMode)
	assert.Equal(t, "Muted", row.SiteAlertModeDisplay())
	assert.True(t, row.WasMutedOnFire())
}

func TestBuildAlertRowFromAlert_NormalizesSiteAlertMode(t *testing.T) {
	site := &model.Site{
		ID:        "site-2",
		Name:      "Muted Site",
		AlertMode: model.SiteAlertMode("Muted"),
	}
	alert := &model.Alert{
		ID:             "alert-2",
		SiteID:         site.ID,
		RuleType:       "unknown_domain",
		Severity:       "high",
		Title:          "Muted Alert",
		Description:    "Domain muted.example.com",
		DeliveryStatus: model.AlertDeliveryStatusMuted,
		FiredAt:        time.Now(),
	}

	h := &UIHandlers{
		SiteSvc: &mockSitesServiceForAlerts{
			sites: []*model.Site{site},
		},
	}

	row := h.buildAlertRowFromAlert(context.Background(), alert)

	assert.Equal(t, site.Name, row.SiteName)
	assert.Equal(t, model.SiteAlertModeMuted, row.SiteAlertMode)
	assert.Equal(t, "Muted", row.SiteAlertModeDisplay())
	assert.True(t, row.WasMutedOnFire())
}

func TestDeriveAlertDeliveryStatus(t *testing.T) {
	tests := []struct {
		name       string
		initial    model.AlertDeliveryStatus
		deliveries []alertsvm.DeliveryRow
		expected   model.AlertDeliveryStatus
	}{
		{
			name:    "keeps muted when initial muted",
			initial: model.AlertDeliveryStatusMuted,
			deliveries: []alertsvm.DeliveryRow{
				{Status: string(model.JobStatusCompleted)},
			},
			expected: model.AlertDeliveryStatusMuted,
		},
		{
			name:    "dispatched when any completed",
			initial: model.AlertDeliveryStatusPending,
			deliveries: []alertsvm.DeliveryRow{
				{Status: string(model.JobStatusCompleted)},
				{Status: string(model.JobStatusFailed)},
			},
			expected: model.AlertDeliveryStatusDispatched,
		},
		{
			name:    "pending when running",
			initial: model.AlertDeliveryStatusPending,
			deliveries: []alertsvm.DeliveryRow{
				{Status: string(model.JobStatusRunning)},
				{Status: string(model.JobStatusPending)},
			},
			expected: model.AlertDeliveryStatusPending,
		},
		{
			name:    "failed when only failures",
			initial: model.AlertDeliveryStatusPending,
			deliveries: []alertsvm.DeliveryRow{
				{Status: string(model.JobStatusFailed)},
			},
			expected: model.AlertDeliveryStatusFailed,
		},
		{
			name:       "falls back to initial when no deliveries",
			initial:    model.AlertDeliveryStatusPending,
			deliveries: nil,
			expected:   model.AlertDeliveryStatusPending,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := deriveAlertDeliveryStatus(tt.initial, tt.deliveries)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatJSONForDisplay(t *testing.T) {
	tests := []struct {
		name     string
		input    json.RawMessage
		expected string
	}{
		{
			name:     "empty",
			input:    json.RawMessage("{}"),
			expected: "",
		},
		{
			name:     "null",
			input:    json.RawMessage("null"),
			expected: "",
		},
		{
			name:     "valid json",
			input:    json.RawMessage(`{"key":"value","num":123}`),
			expected: "{\n  \"key\": \"value\",\n  \"num\": 123\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatJSONForDisplay(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractJobIDFromContext(t *testing.T) {
	tests := []struct {
		name     string
		input    json.RawMessage
		expected string
	}{
		{
			name:     "empty",
			input:    json.RawMessage("{}"),
			expected: "",
		},
		{
			name:     "with job_id",
			input:    json.RawMessage(`{"job_id":"job-123","other":"data"}`),
			expected: "job-123",
		},
		{
			name:     "without job_id",
			input:    json.RawMessage(`{"domain":"example.com"}`),
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractJobIDFromContext(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestAlertViewHandler(t *testing.T) {
	// Setup test data
	firedAt := time.Now().Add(-1 * time.Hour)
	alert := &model.Alert{
		ID:           "alert-123",
		SiteID:       "site-456",
		RuleType:     "unknown_domain",
		Severity:     "high",
		Title:        "Test Alert",
		Description:  "Test alert description",
		EventContext: json.RawMessage(`{"domain":"example.com","job_id":"job-789"}`),
		Metadata:     json.RawMessage(`{"key":"value"}`),
		FiredAt:      firedAt,
	}

	site := &model.Site{
		ID:   "site-456",
		Name: "Test Site",
	}

	// Create mock services
	alertsSvc := &mockAlertsService{alerts: []*model.Alert{alert}}
	sitesSvc := &mockSitesServiceForAlerts{sites: []*model.Site{site}}

	// Create test renderer
	renderer, err := NewTemplateRenderer(TemplateRendererConfig{
		TemplateFS: os.DirFS("../../frontend/templates"),
	})
	require.NoError(t, err)

	// Create handlers
	handlers := &UIHandlers{
		T:         renderer,
		AlertsSvc: alertsSvc,
		SiteSvc:   sitesSvc,
	}

	// Create test request
	req := httptest.NewRequest(http.MethodGet, "/alerts/alert-123", nil)
	req.SetPathValue("id", "alert-123")
	w := httptest.NewRecorder()

	// Call handler
	handlers.AlertView(w, req)

	// Verify response
	assert.Equal(t, http.StatusOK, w.Code)
	body := w.Body.String()
	assert.Contains(t, body, "Test Alert")
	assert.Contains(t, body, "Test Site")
	assert.Contains(t, body, "unknown_domain")
}

func TestAlertResolve_RowTargetSuccess(t *testing.T) {
	firedAt := time.Now().Add(-3 * time.Hour)
	alert := &model.Alert{
		ID:        "alert-123",
		SiteID:    "site-1",
		Title:     "Example Alert",
		RuleType:  string(model.AlertRuleTypeUnknownDomain),
		Severity:  string(model.AlertSeverityHigh),
		FiredAt:   firedAt,
		CreatedAt: firedAt,
	}

	alertsSvc := &mockAlertsService{alerts: []*model.Alert{alert}}
	sitesSvc := &mockSitesServiceForAlerts{sites: []*model.Site{
		{ID: "site-1", Name: "Example Site"},
	}}

	renderer, err := NewTemplateRenderer(TemplateRendererConfig{
		TemplateFS: os.DirFS("../../frontend/templates"),
	})
	require.NoError(t, err)

	handlers := &UIHandlers{
		T:         renderer,
		AlertsSvc: alertsSvc,
		SiteSvc:   sitesSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/alerts/alert-123/resolve", nil)
	req.Header.Set("Hx-Request", "true")
	req.Header.Set("Hx-Target", "#alert-row-alert-123")
	req.SetPathValue("id", "alert-123")

	w := httptest.NewRecorder()
	handlers.AlertResolve(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))

	trigger := w.Header().Get("Hx-Trigger")
	require.NotEmpty(t, trigger)
	require.Contains(t, trigger, "showToast")
	require.Contains(t, trigger, "Alert marked as resolved.")
	require.Contains(t, trigger, "\"type\":\"success\"")

	body := w.Body.String()
	assert.Contains(t, body, `id="alert-row-alert-123"`)
	assert.Contains(t, body, "row-resolved")
	assert.Contains(t, body, "Example Site")
	assert.NotContains(t, body, "/alerts/alert-123/resolve")
}

func TestAlertResolve_RowTargetError(t *testing.T) {
	alertsSvc := &mockAlertsService{err: errors.New("boom")}

	renderer, err := NewTemplateRenderer(TemplateRendererConfig{
		TemplateFS: os.DirFS("../../frontend/templates"),
	})
	require.NoError(t, err)

	handlers := &UIHandlers{
		T:         renderer,
		AlertsSvc: alertsSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/alerts/alert-123/resolve", nil)
	req.Header.Set("Hx-Request", "true")
	req.Header.Set("Hx-Target", "#alert-row-alert-123")
	req.SetPathValue("id", "alert-123")

	w := httptest.NewRecorder()
	handlers.AlertResolve(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	trigger := w.Header().Get("Hx-Trigger")
	require.NotEmpty(t, trigger)
	require.Contains(t, trigger, "Failed to resolve alert.")
	require.Contains(t, trigger, "\"type\":\"error\"")
	assert.Empty(t, w.Body.String())
}

func TestAlertResolve_HTMXRedirect(t *testing.T) {
	firedAt := time.Now().Add(-2 * time.Hour)
	alert := &model.Alert{
		ID:        "alert-456",
		SiteID:    "site-9",
		Title:     "Redirect Alert",
		RuleType:  string(model.AlertRuleTypeUnknownDomain),
		Severity:  string(model.AlertSeverityMedium),
		FiredAt:   firedAt,
		CreatedAt: firedAt,
	}

	alertsSvc := &mockAlertsService{alerts: []*model.Alert{alert}}

	handlers := &UIHandlers{
		AlertsSvc: alertsSvc,
	}

	req := httptest.NewRequest(http.MethodPost, "/alerts/alert-456/resolve", nil)
	req.Header.Set("Hx-Request", "true")
	req.Header.Set("Hx-Target", "main-content")
	req.SetPathValue("id", "alert-456")

	w := httptest.NewRecorder()
	handlers.AlertResolve(w, req)

	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "/alerts/alert-456", w.Header().Get("Hx-Redirect"))

	trigger := w.Header().Get("Hx-Trigger")
	require.NotEmpty(t, trigger)
	require.Contains(t, trigger, "\"type\":\"success\"")
	require.Contains(t, trigger, "Alert marked as resolved.")

	assert.Empty(t, w.Body.String())
}

func TestFetchAlertDeliveryJobs_UsesPersistedResults(t *testing.T) {
	ctx := context.Background()
	attemptedAt := time.Now().Add(-time.Hour)
	completedAt := attemptedAt.Add(2 * time.Minute)
	payload := service.AlertDeliveryJobResult{
		JobID:         "job-1",
		AlertID:       "alert-123",
		SinkID:        "sink-1",
		SinkName:      "Webhook",
		JobStatus:     model.JobStatusCompleted,
		AttemptNumber: 2,
		RetryCount:    1,
		AttemptedAt:   attemptedAt,
		CompletedAt:   &completedAt,
	}
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)

	jobID := "job-1"
	jobResult := &model.JobResult{
		JobID:     &jobID,
		JobType:   model.JobTypeAlert,
		Result:    encoded,
		CreatedAt: attemptedAt,
	}

	h := &UIHandlers{
		JobResults: &stubJobResultsRepo{results: map[string][]*model.JobResult{
			"alert-123": {jobResult},
		}},
	}

	rows := h.fetchAlertDeliveryJobs(ctx, "alert-123")
	require.Len(t, rows, 1)

	row := rows[0]
	assert.Equal(t, "job-1", row.JobID)
	assert.Equal(t, "sink-1", row.SinkID)
	assert.Equal(t, "Webhook", row.SinkName)
	assert.Equal(t, string(model.JobStatusCompleted), row.Status)
	assert.Equal(t, 2, row.AttemptNumber)
	require.NotNil(t, row.CompletedAt)
	assert.WithinDuration(t, completedAt, *row.CompletedAt, time.Millisecond)
	assert.WithinDuration(t, attemptedAt, row.AttemptedAt, time.Millisecond)
}

func TestFetchAlertDeliveryJobs_PendingJobsFallback(t *testing.T) {
	ctx := context.Background()
	attemptedAt := time.Now()
	payload := struct {
		SinkID  string `json:"sink_id"`
		Payload struct {
			ID string `json:"id"`
		} `json:"payload"`
	}{SinkID: "sink-2"}
	payload.Payload.ID = "alert-pending"
	encoded, err := json.Marshal(payload)
	require.NoError(t, err)

	job := model.Job{
		ID:          "job-pending",
		Type:        model.JobTypeAlert,
		Status:      model.JobStatusPending,
		Payload:     encoded,
		ScheduledAt: attemptedAt,
		RetryCount:  0,
		MaxRetries:  3,
	}

	h := &UIHandlers{
		Jobs: &stubJobReadService{listResults: []*model.JobWithEventCount{{Job: job}}},
		Sinks: &stubAlertSinkService{sinks: map[string]*model.HTTPAlertSink{
			"sink-2": {ID: "sink-2", Name: "Pending Webhook"},
		}},
	}

	rows := h.fetchAlertDeliveryJobs(ctx, "alert-pending")
	require.Len(t, rows, 1)
	row := rows[0]
	assert.Equal(t, "job-pending", row.JobID)
	assert.Equal(t, string(model.JobStatusPending), row.Status)
	assert.Equal(t, "Pending Webhook", row.SinkName)
	assert.Equal(t, 1, row.AttemptNumber)
}

type stubJobResultsRepo struct {
	results map[string][]*model.JobResult
	err     error
}

var errStubNotImplemented = errors.New("stub: not implemented")

func (s *stubJobResultsRepo) Upsert(context.Context, core.UpsertJobResultParams) error { return nil }

func (s *stubJobResultsRepo) GetByJobID(context.Context, string) (*model.JobResult, error) {
	return nil, data.ErrJobResultsNotFound
}

func (s *stubJobResultsRepo) ListByAlertID(_ context.Context, alertID string) ([]*model.JobResult, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.results[alertID], nil
}

type stubJobReadService struct {
	listResults []*model.JobWithEventCount
}

func (s *stubJobReadService) List(_ context.Context, _ *model.JobListOptions) ([]*model.JobWithEventCount, error) {
	return s.listResults, nil
}

func (s *stubJobReadService) ListRecentByType(context.Context, model.JobType, int) ([]*model.Job, error) {
	return nil, errStubNotImplemented
}

func (s *stubJobReadService) ListRecentByTypeWithSiteNames(
	context.Context,
	model.JobType,
	int,
) ([]*model.JobWithEventCount, error) {
	return nil, errStubNotImplemented
}

func (s *stubJobReadService) Create(context.Context, *model.CreateJobRequest) (*model.Job, error) {
	return nil, errStubNotImplemented
}

func (s *stubJobReadService) GetByID(context.Context, string) (*model.Job, error) {
	return nil, errStubNotImplemented
}

func (s *stubJobReadService) Stats(context.Context, model.JobType) (*model.JobStats, error) {
	return nil, errStubNotImplemented
}

func (s *stubJobReadService) Delete(context.Context, string) error { return nil }

type stubAlertSinkService struct {
	sinks map[string]*model.HTTPAlertSink
}

func (s *stubAlertSinkService) List(context.Context, int, int) ([]*model.HTTPAlertSink, error) {
	return nil, errStubNotImplemented
}

func (s *stubAlertSinkService) GetByID(_ context.Context, id string) (*model.HTTPAlertSink, error) {
	if sink, ok := s.sinks[id]; ok {
		return sink, nil
	}
	return nil, errors.New("not found")
}

func (s *stubAlertSinkService) Create(
	context.Context,
	*model.CreateHTTPAlertSinkRequest,
) (*model.HTTPAlertSink, error) {
	return nil, errStubNotImplemented
}

func (s *stubAlertSinkService) Update(
	context.Context,
	string,
	*model.UpdateHTTPAlertSinkRequest,
) (*model.HTTPAlertSink, error) {
	return nil, errStubNotImplemented
}

func (s *stubAlertSinkService) Delete(context.Context, string) (bool, error) {
	return false, nil
}

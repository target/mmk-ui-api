package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAlertDispatchWorkflow_SingleSink tests the complete alert dispatch workflow
// with a single HTTP alert sink. This test verifies:
// 1. Alert creation triggers automatic dispatch
// 2. Alert job is created with correct payload structure
// 3. Job payload contains sink ID and alert data
// 4. Job is configured with correct retry settings from sink.
func TestAlertDispatchWorkflow_SingleSink(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()

		// Set up repositories
		alertRepo := data.NewAlertRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		httpAlertSinkRepo := data.NewHTTPAlertSinkRepo(db)
		sourceRepo := data.NewSourceRepo(db)
		siteRepo := data.NewSiteRepo(db)

		// Create test source (required for site)
		source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
			Name:  "test-source-workflow",
			Value: "console.log('test');",
			Test:  true,
		})
		require.NoError(t, err)

		// Create test site
		enabled := true
		site, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
			Name:            "test-site-workflow",
			Enabled:         &enabled,
			RunEveryMinutes: 60,
			SourceID:        source.ID,
		})
		require.NoError(t, err)

		// Create test secret for HTTP alert sink
		secret, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "test-api-key-workflow",
			Value: "secret-api-key-123",
		})
		require.NoError(t, err)

		// Create HTTP alert sink with secrets and custom configuration
		okStatus := 200
		retry := 5
		sink, err := httpAlertSinkRepo.Create(ctx, &model.CreateHTTPAlertSinkRequest{
			Name:        "test-webhook-workflow",
			Method:      "POST",
			URI:         "https://example.com/alerts",
			Secrets:     []string{secret.Name},
			Headers:     testutil.StringPtr(`{"Authorization": "Bearer __test-api-key-workflow__"}`),
			QueryParams: testutil.StringPtr("api_key=__test-api-key-workflow__"),
			OkStatus:    &okStatus,
			Retry:       &retry,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, sink.ID)
		_, err = siteRepo.Update(ctx, site.ID, model.UpdateSiteRequest{
			HTTPAlertSinkID: &sink.ID,
		})
		require.NoError(t, err)

		// Set up alert sink service
		alertSinkSvc := NewAlertSinkService(AlertSinkServiceOptions{
			JobRepo:    jobRepo,
			SecretRepo: secretRepo,
			Evaluator:  nil,
		})

		// Set up alert dispatch service
		alertDispatchSvc := NewAlertDispatchService(AlertDispatchServiceOptions{
			Sites:     siteRepo,
			Sinks:     httpAlertSinkRepo,
			AlertSink: alertSinkSvc,
			Logger:    slog.Default(),
		})

		// Set up alert service with dispatcher
		alertService := MustNewAlertService(AlertServiceOptions{
			Repo:       alertRepo,
			Sites:      siteRepo,
			Dispatcher: alertDispatchSvc,
			Logger:     slog.Default(),
		})

		// Create an alert (should trigger automatic dispatch)
		alert, err := alertService.Create(ctx, &model.CreateAlertRequest{
			SiteID:      site.ID,
			RuleType:    string(model.AlertRuleTypeUnknownDomain),
			Severity:    string(model.AlertSeverityHigh),
			Title:       "Unknown domain detected",
			Description: "Test alert for workflow integration",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, alert.ID)
		assert.Equal(t, site.ID, alert.SiteID)
		assert.Equal(t, string(model.AlertRuleTypeUnknownDomain), alert.RuleType)
		assert.Equal(t, string(model.AlertSeverityHigh), alert.Severity)

		// Wait for async dispatch to complete using polling
		require.Eventually(t, func() bool {
			jobs, err := jobRepo.ListRecentByType(ctx, model.JobTypeAlert, 10)
			return err == nil && len(jobs) == 1
		}, 3*time.Second, 50*time.Millisecond, "Expected one alert job to be created")

		// Verify the job details
		jobs, err := jobRepo.ListRecentByType(ctx, model.JobTypeAlert, 10)
		require.NoError(t, err)
		require.Len(t, jobs, 1)

		job := jobs[0]
		assert.Equal(t, model.JobTypeAlert, job.Type)
		assert.Equal(t, model.JobStatusPending, job.Status)
		assert.Equal(t, retry, job.MaxRetries, "Job max retries should match sink retry setting")
		assert.Equal(t, 0, job.RetryCount, "Initial retry count should be 0")

		// Verify job payload structure
		var jobPayload struct {
			SinkID  string          `json:"sink_id"`
			Payload json.RawMessage `json:"payload"`
		}
		err = json.Unmarshal(job.Payload, &jobPayload)
		require.NoError(t, err)
		assert.Equal(t, sink.ID, jobPayload.SinkID, "Job payload should contain sink ID")
		assert.NotEmpty(t, jobPayload.Payload, "Job payload should contain alert data")

		// Verify alert payload contains the complete alert
		var alertPayload model.Alert
		err = json.Unmarshal(jobPayload.Payload, &alertPayload)
		require.NoError(t, err)
		assert.Equal(t, alert.ID, alertPayload.ID)
		assert.Equal(t, alert.SiteID, alertPayload.SiteID)
		assert.Equal(t, alert.RuleType, alertPayload.RuleType)
		assert.Equal(t, alert.Severity, alertPayload.Severity)
		assert.Equal(t, alert.Title, alertPayload.Title)
		assert.Equal(t, alert.Description, alertPayload.Description)
	})
}

// TestAlertDispatchWorkflow_MultipleSinks tests alert dispatch to multiple sinks.
// Verifies that a single alert creates separate jobs for each configured sink.
func TestAlertDispatchWorkflow_MultipleSinks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()

		// Set up repositories
		alertRepo := data.NewAlertRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		httpAlertSinkRepo := data.NewHTTPAlertSinkRepo(db)
		sourceRepo := data.NewSourceRepo(db)
		siteRepo := data.NewSiteRepo(db)

		// Create test source and site
		source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
			Name:  "test-source-multi-workflow",
			Value: "console.log('test');",
			Test:  true,
		})
		require.NoError(t, err)

		enabled := true
		site, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
			Name:            "test-site-multi-workflow",
			Enabled:         &enabled,
			RunEveryMinutes: 60,
			SourceID:        source.ID,
		})
		require.NoError(t, err)

		// Create three HTTP alert sinks with different configurations
		okStatus1 := 200
		okStatus2 := 201
		okStatus3 := 204
		retry1 := 3
		retry2 := 5
		retry3 := 2

		sink1, err := httpAlertSinkRepo.Create(ctx, &model.CreateHTTPAlertSinkRequest{
			Name:     "webhook-primary",
			Method:   "POST",
			URI:      "https://primary.example.com/alerts",
			OkStatus: &okStatus1,
			Retry:    &retry1,
		})
		require.NoError(t, err)

		sink2, err := httpAlertSinkRepo.Create(ctx, &model.CreateHTTPAlertSinkRequest{
			Name:     "webhook-secondary",
			Method:   "PUT",
			URI:      "https://secondary.example.com/notifications",
			OkStatus: &okStatus2,
			Retry:    &retry2,
		})
		require.NoError(t, err)

		sink3, err := httpAlertSinkRepo.Create(ctx, &model.CreateHTTPAlertSinkRequest{
			Name:     "webhook-tertiary",
			Method:   "POST",
			URI:      "https://tertiary.example.com/events",
			OkStatus: &okStatus3,
			Retry:    &retry3,
		})
		require.NoError(t, err)
		_, err = siteRepo.Update(ctx, site.ID, model.UpdateSiteRequest{
			HTTPAlertSinkID: &sink2.ID,
		})
		require.NoError(t, err)

		// Set up services
		alertSinkSvc := NewAlertSinkService(AlertSinkServiceOptions{
			JobRepo:    jobRepo,
			SecretRepo: secretRepo,
			Evaluator:  nil,
		})

		alertDispatchSvc := NewAlertDispatchService(AlertDispatchServiceOptions{
			Sites:     siteRepo,
			Sinks:     httpAlertSinkRepo,
			AlertSink: alertSinkSvc,
			Logger:    slog.Default(),
		})

		alertService := MustNewAlertService(AlertServiceOptions{
			Repo:       alertRepo,
			Sites:      siteRepo,
			Dispatcher: alertDispatchSvc,
			Logger:     slog.Default(),
		})

		// Create an alert
		alert, err := alertService.Create(ctx, &model.CreateAlertRequest{
			SiteID:      site.ID,
			RuleType:    string(model.AlertRuleTypeIOC),
			Severity:    string(model.AlertSeverityCritical),
			Title:       "IOC domain detected",
			Description: "Multiple sinks workflow test",
		})
		require.NoError(t, err)

		// Wait for async dispatch using polling
		require.Eventually(t, func() bool {
			jobs, err := jobRepo.ListRecentByType(ctx, model.JobTypeAlert, 10)
			return err == nil && len(jobs) == 1
		}, 3*time.Second, 50*time.Millisecond, "Expected one alert job for configured sink")

		// Verify the job details
		jobs, err := jobRepo.ListRecentByType(ctx, model.JobTypeAlert, 10)
		require.NoError(t, err)
		require.Len(t, jobs, 1)

		job := jobs[0]
		assert.Equal(t, model.JobTypeAlert, job.Type)
		assert.Equal(t, model.JobStatusPending, job.Status)

		var jobPayload struct {
			SinkID  string          `json:"sink_id"`
			Payload json.RawMessage `json:"payload"`
		}
		err = json.Unmarshal(job.Payload, &jobPayload)
		require.NoError(t, err)

		assert.Equal(t, sink2.ID, jobPayload.SinkID, "Job should target configured sink")
		assert.NotEqual(t, sink1.ID, jobPayload.SinkID, "Job should ignore unconfigured sink1")
		assert.NotEqual(t, sink3.ID, jobPayload.SinkID, "Job should ignore unconfigured sink3")
		assert.Equal(t, retry2, job.MaxRetries, "Job max retries should match configured sink")

		var alertPayload model.Alert
		err = json.Unmarshal(jobPayload.Payload, &alertPayload)
		require.NoError(t, err)
		assert.Equal(t, alert.ID, alertPayload.ID)
	})
}

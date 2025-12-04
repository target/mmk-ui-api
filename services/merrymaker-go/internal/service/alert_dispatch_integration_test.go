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

func TestAlertDispatchIntegration_EndToEnd(t *testing.T) {
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

		// Create test source first (required for site)
		sourceRepo := data.NewSourceRepo(db)
		source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
			Name:  "test-source",
			Value: "console.log('test');",
			Test:  true,
		})
		require.NoError(t, err)

		// Create test site
		siteRepo := data.NewSiteRepo(db)
		enabled := true
		site, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
			Name:            "test-site",
			Enabled:         &enabled,
			RunEveryMinutes: 60,
			SourceID:        source.ID,
		})
		require.NoError(t, err)

		// Create test secret
		secret, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "test-webhook-token",
			Value: "test-token-value",
		})
		require.NoError(t, err)

		// Create test HTTP alert sink
		sink, err := httpAlertSinkRepo.Create(ctx, &model.CreateHTTPAlertSinkRequest{
			Name:     "test-webhook",
			Method:   "POST",
			URI:      "https://webhook.site/test",
			Secrets:  []string{secret.Name},
			Headers:  stringPtr(`{"Content-Type": "application/json"}`),
			OkStatus: intPtr(200),
			Retry:    intPtr(3),
		})
		require.NoError(t, err)
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
			Severity:    string(model.AlertSeverityMedium),
			Title:       "Test alert",
			Description: "Test alert for dispatch integration",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, alert.ID)

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

		// Verify job payload contains sink ID and alert payload
		var jobPayload struct {
			SinkID  string          `json:"sink_id"`
			Payload json.RawMessage `json:"payload"`
		}
		err = json.Unmarshal(job.Payload, &jobPayload)
		require.NoError(t, err)
		assert.Equal(t, sink.ID, jobPayload.SinkID)

		// Verify alert payload contains the alert
		var alertPayload model.Alert
		err = json.Unmarshal(jobPayload.Payload, &alertPayload)
		require.NoError(t, err)
		assert.Equal(t, alert.ID, alertPayload.ID)
		assert.Equal(t, alert.Title, alertPayload.Title)
	})
}

func TestAlertDispatchIntegration_NoSinks(t *testing.T) {
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

		// Create test source first (required for site)
		sourceRepo := data.NewSourceRepo(db)
		source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
			Name:  "test-source-no-sinks",
			Value: "console.log('test');",
			Test:  true,
		})
		require.NoError(t, err)

		// Create test site
		siteRepo := data.NewSiteRepo(db)
		enabled := true
		site, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
			Name:            "test-site-no-sinks",
			Enabled:         &enabled,
			RunEveryMinutes: 60,
			SourceID:        source.ID,
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

		// Create an alert (should not fail even with no sinks)
		alert, err := alertService.Create(ctx, &model.CreateAlertRequest{
			SiteID:      site.ID,
			RuleType:    string(model.AlertRuleTypeUnknownDomain),
			Severity:    string(model.AlertSeverityMedium),
			Title:       "Test alert with no sinks",
			Description: "Test alert when no sinks are configured",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, alert.ID)

		// Give async dispatch a chance to run (should be no-op)
		// Use a short Eventually to ensure no jobs are created
		time.Sleep(100 * time.Millisecond)

		// Verify that no alert jobs were created
		jobs, err := jobRepo.ListRecentByType(ctx, model.JobTypeAlert, 10)
		require.NoError(t, err)
		assert.Empty(t, jobs, "Expected no alert jobs when no sinks are configured")
	})
}

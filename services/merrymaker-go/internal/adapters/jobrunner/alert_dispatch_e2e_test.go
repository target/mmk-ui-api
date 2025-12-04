package jobrunner

import (
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAlertDispatch_EndToEnd tests the complete alert dispatch flow:
// 1. Create HTTP alert sink with mock webhook server
// 2. Create alert (triggers dispatch)
// 3. Verify job is created
// 4. Run job runner to process the alert job
// 5. Verify HTTP request was sent to mock server with correct payload.
func TestAlertDispatch_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()

		// Set up mock webhook server to receive alert
		var receivedRequests []receivedRequest
		var mu sync.Mutex
		mockWebhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			defer mu.Unlock()

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Logf("Failed to read request body: %v", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}

			receivedRequests = append(receivedRequests, receivedRequest{
				Method:  r.Method,
				URL:     r.URL.String(),
				Headers: r.Header.Clone(),
				Body:    body,
			})

			w.WriteHeader(http.StatusOK)
		}))
		defer mockWebhook.Close()

		// Set up repositories
		alertRepo := data.NewAlertRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		httpAlertSinkRepo := data.NewHTTPAlertSinkRepo(db)

		// Create test source (required for site)
		sourceRepo := data.NewSourceRepo(db)
		source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
			Name:  "test-source-e2e",
			Value: "console.log('test');",
			Test:  true,
		})
		require.NoError(t, err)

		// Create test site
		siteRepo := data.NewSiteRepo(db)
		enabled := true
		site, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
			Name:            "test-site-e2e",
			Enabled:         &enabled,
			RunEveryMinutes: 60,
			SourceID:        source.ID,
		})
		require.NoError(t, err)

		// Create test secret
		secret, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "test-webhook-token-e2e",
			Value: "test-secret-value-123",
		})
		require.NoError(t, err)

		// Create HTTP alert sink pointing to mock server
		okStatus := 200
		retry := 3
		sink, err := httpAlertSinkRepo.Create(ctx, &model.CreateHTTPAlertSinkRequest{
			Name:        "test-webhook-e2e",
			Method:      "POST",
			URI:         mockWebhook.URL + "/webhook",
			Secrets:     []string{secret.Name},
			QueryParams: testutil.StringPtr("token=__test-webhook-token-e2e__"),
			Headers: testutil.StringPtr(
				`{"Content-Type": "application/json", "X-API-Key": "__test-webhook-token-e2e__"}`,
			),
			OkStatus: &okStatus,
			Retry:    &retry,
		})
		require.NoError(t, err)
		_, err = siteRepo.Update(ctx, site.ID, model.UpdateSiteRequest{
			HTTPAlertSinkID: &sink.ID,
		})
		require.NoError(t, err)

		// Set up alert sink service
		alertSinkSvc := service.NewAlertSinkService(service.AlertSinkServiceOptions{
			JobRepo:    jobRepo,
			SecretRepo: secretRepo,
			Evaluator:  nil,
		})

		// Set up alert dispatch service
		alertDispatchSvc := service.NewAlertDispatchService(service.AlertDispatchServiceOptions{
			Sites:     siteRepo,
			Sinks:     httpAlertSinkRepo,
			AlertSink: alertSinkSvc,
			Logger:    slog.Default(),
		})

		// Set up alert service with dispatcher
		alertService := service.MustNewAlertService(service.AlertServiceOptions{
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
			Title:       "Test alert for E2E dispatch",
			Description: "This alert should be dispatched to the mock webhook",
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
		assert.Equal(t, retry, job.MaxRetries)

		// Verify job payload
		var jobPayload struct {
			SinkID  string          `json:"sink_id"`
			Payload json.RawMessage `json:"payload"`
		}
		err = json.Unmarshal(job.Payload, &jobPayload)
		require.NoError(t, err)
		assert.Equal(t, sink.ID, jobPayload.SinkID)

		// Verify alert payload
		var alertPayload model.Alert
		err = json.Unmarshal(jobPayload.Payload, &alertPayload)
		require.NoError(t, err)
		assert.Equal(t, alert.ID, alertPayload.ID)
		assert.Equal(t, alert.Title, alertPayload.Title)

		// Now run the job runner to process the alert job
		httpClient := newHTTPClientNoProxy(t)
		runner, err := NewRunner(RunnerOptions{
			DB:            db,
			Logger:        slog.Default(),
			HTTPClient:    httpClient,
			Lease:         30 * time.Second,
			Concurrency:   1,
			JobType:       model.JobTypeAlert,
			JobsRepo:      jobRepo,
			SecretsRepo:   secretRepo,
			AlertSinkRepo: httpAlertSinkRepo,
		})
		require.NoError(t, err)

		reservedJob := runSingleJob(ctx, t, runner)
		require.Equal(t, job.ID, reservedJob.ID)

		// Verify the job was completed
		completedJob, err := jobRepo.GetByID(ctx, job.ID)
		require.NoError(t, err)
		assert.Equalf(
			t,
			model.JobStatusCompleted,
			completedJob.Status,
			"Job should be completed. last error: %v",
			completedJob.LastError,
		)

		// Verify the mock webhook received the request
		mu.Lock()
		defer mu.Unlock()

		require.Lenf(
			t,
			receivedRequests,
			1,
			"Expected one HTTP request to mock webhook (job last error: %v)",
			completedJob.LastError,
		)

		req := receivedRequests[0]
		assert.Equal(t, "POST", req.Method)
		assert.Contains(t, req.URL, "/webhook")
		assert.Contains(t, req.URL, "token=test-secret-value-123", "Query param should have secret resolved")

		// Verify headers
		assert.Equal(t, "application/json", req.Headers.Get("Content-Type"))
		assert.Equal(t, "test-secret-value-123", req.Headers.Get("X-Api-Key"), "Header should have secret resolved")

		// Verify body contains the alert
		var receivedAlert model.Alert
		err = json.Unmarshal(req.Body, &receivedAlert)
		require.NoError(t, err)
		assert.Equal(t, alert.ID, receivedAlert.ID)
		assert.Equal(t, alert.Title, receivedAlert.Title)
		assert.Equal(t, alert.Description, receivedAlert.Description)
		assert.Equal(t, alert.RuleType, receivedAlert.RuleType)
		assert.Equal(t, alert.Severity, receivedAlert.Severity)
		assert.Equal(t, alert.SiteID, receivedAlert.SiteID)

		// Verify job results were persisted with secrets redacted
		jobResultRepo := data.NewJobResultRepo(db)
		row, err := jobResultRepo.GetByJobID(ctx, job.ID)
		require.NoError(t, err)
		require.NotNil(t, row)

		var stored service.AlertDeliveryJobResult
		require.NoError(t, json.Unmarshal(row.Result, &stored))

		assert.Equal(t, job.ID, stored.JobID)
		assert.Equal(t, sink.ID, stored.SinkID)
		assert.Equal(t, sink.Name, stored.SinkName)
		assert.Equal(t, model.JobStatusCompleted, stored.JobStatus)
		assert.Equal(t, retry, stored.MaxRetries)
		assert.Equal(t, job.RetryCount, stored.RetryCount)
		assert.Equal(t, 1, stored.AttemptNumber)
		require.NotNil(t, stored.Response)
		assert.Equal(t, http.StatusOK, stored.Response.StatusCode)
		assert.False(t, stored.Response.BodyTruncated)
		assert.Contains(t, stored.Request.URL, "__test-webhook-token-e2e__")
		assert.NotContains(t, stored.Request.URL, secret.Value)
		if stored.Request.Headers != nil {
			assert.Equal(t, "__test-webhook-token-e2e__", stored.Request.Headers["X-API-Key"])
			assert.NotContains(t, stored.Request.Headers["X-API-Key"], secret.Value)
		}
	})
}

// receivedRequest captures details of an HTTP request received by the mock server.
type receivedRequest struct {
	Method  string
	URL     string
	Headers http.Header
	Body    []byte
}

// TestAlertDispatch_MultipleSinks tests that an alert is dispatched to multiple sinks.
func TestAlertDispatch_MultipleSinks(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()

		// Set up two mock webhook servers
		var receivedRequests1, receivedRequests2 []receivedRequest
		var mu1, mu2 sync.Mutex

		mockWebhook1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu1.Lock()
			defer mu1.Unlock()
			body, _ := io.ReadAll(r.Body)
			receivedRequests1 = append(receivedRequests1, receivedRequest{
				Method:  r.Method,
				URL:     r.URL.String(),
				Headers: r.Header.Clone(),
				Body:    body,
			})
			w.WriteHeader(http.StatusOK)
		}))
		defer mockWebhook1.Close()

		mockWebhook2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu2.Lock()
			defer mu2.Unlock()
			body, _ := io.ReadAll(r.Body)
			receivedRequests2 = append(receivedRequests2, receivedRequest{
				Method:  r.Method,
				URL:     r.URL.String(),
				Headers: r.Header.Clone(),
				Body:    body,
			})
			w.WriteHeader(http.StatusAccepted)
		}))
		defer mockWebhook2.Close()

		// Set up repositories
		alertRepo := data.NewAlertRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		httpAlertSinkRepo := data.NewHTTPAlertSinkRepo(db)
		sourceRepo := data.NewSourceRepo(db)
		siteRepo := data.NewSiteRepo(db)

		// Create test source and site
		source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
			Name:  "test-source-multi",
			Value: "console.log('test');",
			Test:  true,
		})
		require.NoError(t, err)

		enabled := true
		site, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
			Name:            "test-site-multi",
			Enabled:         &enabled,
			RunEveryMinutes: 60,
			SourceID:        source.ID,
		})
		require.NoError(t, err)

		// Create two HTTP alert sinks
		okStatus1 := 200
		okStatus2 := 202
		retry := 2

		sink1, err := httpAlertSinkRepo.Create(ctx, &model.CreateHTTPAlertSinkRequest{
			Name:     "webhook-1",
			Method:   "POST",
			URI:      mockWebhook1.URL + "/alerts",
			OkStatus: &okStatus1,
			Retry:    &retry,
		})
		require.NoError(t, err)

		sink2, err := httpAlertSinkRepo.Create(ctx, &model.CreateHTTPAlertSinkRequest{
			Name:     "webhook-2",
			Method:   "POST",
			URI:      mockWebhook2.URL + "/notifications",
			OkStatus: &okStatus2,
			Retry:    &retry,
		})
		require.NoError(t, err)
		_, err = siteRepo.Update(ctx, site.ID, model.UpdateSiteRequest{
			HTTPAlertSinkID: &sink1.ID,
		})
		require.NoError(t, err)

		// Set up services
		alertSinkSvc := service.NewAlertSinkService(service.AlertSinkServiceOptions{
			JobRepo:    jobRepo,
			SecretRepo: secretRepo,
			Evaluator:  nil,
		})

		alertDispatchSvc := service.NewAlertDispatchService(service.AlertDispatchServiceOptions{
			Sites:     siteRepo,
			Sinks:     httpAlertSinkRepo,
			AlertSink: alertSinkSvc,
			Logger:    slog.Default(),
		})

		alertService := service.MustNewAlertService(service.AlertServiceOptions{
			Repo:       alertRepo,
			Sites:      siteRepo,
			Dispatcher: alertDispatchSvc,
			Logger:     slog.Default(),
		})

		// Create an alert
		alert, err := alertService.Create(ctx, &model.CreateAlertRequest{
			SiteID:      site.ID,
			RuleType:    string(model.AlertRuleTypeUnknownDomain),
			Severity:    string(model.AlertSeverityCritical),
			Title:       "Multiple sinks test",
			Description: "This alert should go to both sinks",
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

		// Run job runner
		httpClient := newHTTPClientNoProxy(t)
		runner, err := NewRunner(RunnerOptions{
			DB:            db,
			Logger:        slog.Default(),
			HTTPClient:    httpClient,
			Lease:         30 * time.Second,
			Concurrency:   2,
			JobType:       model.JobTypeAlert,
			JobsRepo:      jobRepo,
			SecretsRepo:   secretRepo,
			AlertSinkRepo: httpAlertSinkRepo,
		})
		require.NoError(t, err)

		reservedJob := runSingleJob(ctx, t, runner)
		require.Equal(t, jobs[0].ID, reservedJob.ID)

		// Verify only configured webhook received request
		mu1.Lock()
		require.Len(t, receivedRequests1, 1, "Webhook 1 should receive one request")
		req1 := receivedRequests1[0]
		mu1.Unlock()

		mu2.Lock()
		require.Empty(t, receivedRequests2, "Webhook 2 should not receive requests")
		mu2.Unlock()

		// Verify request details
		assert.Equal(t, "POST", req1.Method)
		assert.Contains(t, req1.URL, "/alerts")
		var alert1 model.Alert
		require.NoError(t, json.Unmarshal(req1.Body, &alert1))
		assert.Equal(t, alert.ID, alert1.ID)

		// Verify sink IDs in job payloads
		sinkIDs := make(map[string]bool)
		for _, job := range jobs {
			var jobPayload struct {
				SinkID string `json:"sink_id"`
			}
			require.NoError(t, json.Unmarshal(job.Payload, &jobPayload))
			sinkIDs[jobPayload.SinkID] = true
		}
		assert.True(t, sinkIDs[sink1.ID], "Should have job for configured sink")
		assert.False(t, sinkIDs[sink2.ID], "Should not have job for unconfigured sink")
	})
}

// TestAlertDispatch_FailedRequest tests that failed HTTP requests are marked for retry.
func TestAlertDispatch_FailedRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()

		// Set up mock webhook that always fails with wrong status code
		var requestCount int
		var mu sync.Mutex

		mockWebhook := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mu.Lock()
			requestCount++
			mu.Unlock()
			// Return wrong status code (expecting 200, but returning 500)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer mockWebhook.Close()

		// Set up repositories
		alertRepo := data.NewAlertRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		httpAlertSinkRepo := data.NewHTTPAlertSinkRepo(db)
		sourceRepo := data.NewSourceRepo(db)
		siteRepo := data.NewSiteRepo(db)

		// Create test source and site
		source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
			Name:  "test-source-retry",
			Value: "console.log('test');",
			Test:  true,
		})
		require.NoError(t, err)

		enabled := true
		site, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
			Name:            "test-site-retry",
			Enabled:         &enabled,
			RunEveryMinutes: 60,
			SourceID:        source.ID,
		})
		require.NoError(t, err)

		// Create HTTP alert sink with retry
		okStatus := 200
		retry := 3
		sink, err := httpAlertSinkRepo.Create(ctx, &model.CreateHTTPAlertSinkRequest{
			Name:     "webhook-retry",
			Method:   "POST",
			URI:      mockWebhook.URL + "/webhook",
			OkStatus: &okStatus,
			Retry:    &retry,
		})
		require.NoError(t, err)
		assert.NotEmpty(t, sink.ID)
		_, err = siteRepo.Update(ctx, site.ID, model.UpdateSiteRequest{
			HTTPAlertSinkID: &sink.ID,
		})
		require.NoError(t, err)

		// Set up services
		alertSinkSvc := service.NewAlertSinkService(service.AlertSinkServiceOptions{
			JobRepo:    jobRepo,
			SecretRepo: secretRepo,
			Evaluator:  nil,
		})

		alertDispatchSvc := service.NewAlertDispatchService(service.AlertDispatchServiceOptions{
			Sites:     siteRepo,
			Sinks:     httpAlertSinkRepo,
			AlertSink: alertSinkSvc,
			Logger:    slog.Default(),
		})

		alertService := service.MustNewAlertService(service.AlertServiceOptions{
			Repo:       alertRepo,
			Dispatcher: alertDispatchSvc,
			Logger:     slog.Default(),
		})

		// Create an alert
		alert, err := alertService.Create(ctx, &model.CreateAlertRequest{
			SiteID:      site.ID,
			RuleType:    string(model.AlertRuleTypeUnknownDomain),
			Severity:    string(model.AlertSeverityMedium),
			Title:       "Retry test alert",
			Description: "This alert should be retried on failure",
		})
		require.NoError(t, err)
		assert.NotEmpty(t, alert.ID)

		// Wait for async dispatch using polling
		require.Eventually(t, func() bool {
			jobs, err := jobRepo.ListRecentByType(ctx, model.JobTypeAlert, 10)
			return err == nil && len(jobs) == 1
		}, 3*time.Second, 50*time.Millisecond, "Expected one alert job to be created")

		// Verify the job details
		jobs, err := jobRepo.ListRecentByType(ctx, model.JobTypeAlert, 10)
		require.NoError(t, err)
		require.Len(t, jobs, 1)

		job := jobs[0]
		assert.Equal(t, retry, job.MaxRetries)
		assert.Equal(t, 0, job.RetryCount, "Initial retry count should be 0")

		// Run job runner - it will fail
		httpClient := newHTTPClientNoProxy(t)
		runner, err := NewRunner(RunnerOptions{
			DB:            db,
			Logger:        slog.Default(),
			HTTPClient:    httpClient,
			Lease:         30 * time.Second,
			Concurrency:   1,
			JobType:       model.JobTypeAlert,
			JobsRepo:      jobRepo,
			SecretsRepo:   secretRepo,
			AlertSinkRepo: httpAlertSinkRepo,
		})
		require.NoError(t, err)

		reservedJob := runSingleJob(ctx, t, runner)
		require.Equal(t, job.ID, reservedJob.ID)

		// Verify webhook was called
		mu.Lock()
		count := requestCount
		mu.Unlock()
		assert.Equal(t, 1, count, "Webhook should have been called once")

		// Check job status after first attempt
		jobAfterAttempt, err := jobRepo.GetByID(ctx, job.ID)
		require.NoError(t, err)

		// Job should be pending (requeued for retry) since retry_count (1) < max_retries (3)
		assert.Equal(t, model.JobStatusPending, jobAfterAttempt.Status, "Job should be requeued for retry")
		assert.Equal(t, 1, jobAfterAttempt.RetryCount, "Retry count should be incremented")
		assert.NotNil(t, jobAfterAttempt.LastError, "Last error should be set")
		assert.Contains(t, *jobAfterAttempt.LastError, "unexpected status", "Error should mention status code mismatch")
	})
}

func runSingleJob(ctx context.Context, t *testing.T, runner *Runner) *model.Job {
	t.Helper()

	job, err := runner.jobs.ReserveNext(ctx, runner.jobType, runner.lease)
	require.NoError(t, err)
	require.NotNil(t, job, "expected job to be available for type %s", runner.jobType)

	runner.processJob(ctx, job)
	return job
}

func newHTTPClientNoProxy(tb testing.TB) *http.Client {
	tb.Helper()

	dialer := &net.Dialer{
		Timeout:   15 * time.Second,
		KeepAlive: 15 * time.Second,
	}

	transport := &http.Transport{
		Proxy: func(*http.Request) (*url.URL, error) {
			//nolint:nilnil // returning nil URL and nil error disables proxy usage
			return nil, nil
		},
		DialContext:           dialer.DialContext,
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          32,
		MaxConnsPerHost:       0,
		MaxIdleConnsPerHost:   16,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
	}

	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	tb.Cleanup(func() {
		transport.CloseIdleConnections()
	})

	return client
}

package httpx

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func createSourceAndCache(
	ctx context.Context,
	t testutil.TestingTB,
	sourceRepo *data.SourceRepo,
	sourceCache *core.SourceCacheService,
) *model.Source {
	t.Helper()
	src, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
		Name:  fmt.Sprintf("test-src-%d", time.Now().UnixNano()),
		Value: "console.log('hello from puppeteer script');",
		Test:  true,
	})
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if err := sourceCache.CacheSourceContent(ctx, src.ID); err != nil {
		t.Fatalf("cache source content: %v", err)
	}
	return src
}

func createJobHTTP(t testutil.TestingTB, baseURL string, req *model.CreateJobRequest) model.Job {
	t.Helper()
	resp := DoJSON(t, JSONRequest{
		Method:  http.MethodPost,
		URL:     baseURL + "/api/jobs",
		Payload: req,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create job status: %d", resp.StatusCode)
	}
	var out model.Job
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode create job: %v", err)
	}
	return out
}

type reserveNextConfig struct {
	BaseURL  string
	LeaseSec int
	WaitSec  int
}

func reserveNextHTTP(t testutil.TestingTB, cfg reserveNextConfig) model.Job {
	t.Helper()
	url := fmt.Sprintf(
		"%s/api/jobs/browser/reserve_next?lease=%d&wait=%d",
		cfg.BaseURL,
		cfg.LeaseSec,
		cfg.WaitSec,
	)
	resp := DoJSON(t, JSONRequest{
		Method: http.MethodGet,
		URL:    url,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reserve_next status: %d", resp.StatusCode)
	}
	var out model.Job
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode reserved job: %v", err)
	}
	return out
}

func postEventsHTTP(t testutil.TestingTB, baseURL string, batch PuppeteerEventBatch) {
	t.Helper()
	resp := DoJSON(t, JSONRequest{
		Method:  http.MethodPost,
		URL:     baseURL + "/api/events/bulk",
		Payload: batch,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events bulk status: %d", resp.StatusCode)
	}
}

func completeJobHTTP(t testutil.TestingTB, baseURL, jobID string) {
	t.Helper()
	resp := DoJSON(t, JSONRequest{
		Method: http.MethodPost,
		URL:    baseURL + "/api/jobs/" + jobID + "/complete",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete status: %d", resp.StatusCode)
	}
}

// testClient provides a simple HTTP client for workflow testing.
type testClient struct {
	baseURL string
}

func newTestClient(baseURL string) *testClient {
	return &testClient{baseURL: baseURL}
}

func (c *testClient) getJobStatus(t testutil.TestingTB, jobID string) model.JobStatusResponse {
	t.Helper()
	url := fmt.Sprintf("%s/api/jobs/%s/status", c.baseURL, jobID)
	resp := DoJSON(t, JSONRequest{
		Method: http.MethodGet,
		URL:    url,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get job status: %d", resp.StatusCode)
	}
	var status model.JobStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode job status: %v", err)
	}
	return status
}

func (c *testClient) getJobEvents(t testutil.TestingTB, jobID string, limit, offset int) []*model.Event {
	t.Helper()
	url := fmt.Sprintf("%s/api/jobs/%s/events?limit=%d&offset=%d", c.baseURL, jobID, limit, offset)
	resp := DoJSON(t, JSONRequest{
		Method: http.MethodGet,
		URL:    url,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get job events: %d", resp.StatusCode)
	}
	var apiResp struct {
		Events []*model.Event `json:"events"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&apiResp); err != nil {
		t.Fatalf("decode job events: %v", err)
	}
	return apiResp.Events
}

func (c *testClient) postEvents(t testutil.TestingTB, batch PuppeteerEventBatch) {
	t.Helper()
	resp := DoJSON(t, JSONRequest{
		Method:  http.MethodPost,
		URL:     c.baseURL + "/api/events/bulk",
		Payload: batch,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("events bulk status: %d", resp.StatusCode)
	}
}

func (c *testClient) completeJob(t testutil.TestingTB, jobID string) {
	t.Helper()
	resp := DoJSON(t, JSONRequest{
		Method: http.MethodPost,
		URL:    c.baseURL + "/api/jobs/" + jobID + "/complete",
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("complete status: %d", resp.StatusCode)
	}
}

// redisTestAddr returns the Redis test addr and whether it's reachable.
func redisTestAddr(t testutil.TestingTB) (string, bool) {
	t.Helper()
	return testutil.GetTestRedisAddr(t)
}

func Test_Workflow_CreateTestSource_RunBrowserJob_PushEvents_Complete(t *testing.T) {
	// Require Postgres test DB
	testutil.SkipIfNoTestDB(t)

	// Require Redis test instance
	addr, ok := redisTestAddr(t)
	if !ok {
		t.Skip("redis test instance unavailable; run docker-compose profile 'test'")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Wire repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		eventRepo := &data.EventRepo{DB: db}
		sourceRepo := data.NewSourceRepo(db)

		// Wire services used by HTTP router
		jobSvc := service.MustNewJobService(service.JobServiceOptions{
			Repo:         jobRepo,
			DefaultLease: 30 * time.Second,
		})
		eventSvc := service.MustNewEventService(service.EventServiceOptions{
			Repo: eventRepo,
			Config: service.EventServiceConfig{
				MaxBatch:                 1000,
				ThreatScoreProcessCutoff: 0.7,
			},
		})

		mux := NewRouter(RouterServices{Jobs: jobSvc, Events: eventSvc, IsDev: true})
		ts := httptest.NewServer(mux)
		defer ts.Close()

		// Wire cache for sources (using real RedisTest)
		redisClient := data.NewRedisClient(data.RedisConfig{Addr: addr})
		defer func() { _ = redisClient.Close() }()
		cacheRepo := data.NewRedisCacheRepo(redisClient)
		sourceCache := core.NewSourceCacheService(core.SourceCacheServiceOptions{
			Cache:   cacheRepo,
			Sources: sourceRepo,
			Secrets: nil,
			Config:  core.DefaultSourceCacheConfig(),
		})

		ctx := context.Background()

		// 1) Create a Source (test=true) and cache its content via production repos/services
		src := createSourceAndCache(ctx, t, sourceRepo, sourceCache)

		// 3) Enqueue a browser job referencing the source using HTTP API
		created := createJobHTTP(t, ts.URL, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  json.RawMessage(`{"action":"run"}`),
			SourceID: &src.ID,
			IsTest:   true,
		})

		// 4) Fake runner reserves the job via HTTP
		reserved := reserveNextHTTP(t, reserveNextConfig{
			BaseURL:  ts.URL,
			LeaseSec: 15,
			WaitSec:  1,
		})
		if reserved.ID != created.ID {
			t.Fatalf("reserved job mismatch: got %s want %s", reserved.ID, created.ID)
		}

		// 5) Runner fetches source content from cache (no HTTP, uses production caching impl)
		cached, err := sourceCache.GetCachedSourceContent(ctx, src.ID)
		if err != nil {
			t.Fatalf("get cached source content: %v", err)
		}
		if len(cached) == 0 {
			t.Fatalf("expected cached content for source %s", src.ID)
		}

		// 6) Runner posts a mock puppeteer batch to events API with jobId linkage
		batch := PuppeteerEventBatch{
			BatchID:   "batch-1",
			SessionID: created.ID,
			Events: []PuppeteerEvent{
				{
					ID:     "e1",
					Method: "Network.requestWillBeSent",
					Params: PuppeteerEventParams{
						Timestamp: time.Now().UnixMilli(),
						Payload:   map[string]any{"url": "https://example.com"},
					},
					Metadata: PuppeteerEventMetadata{
						Category:        "network",
						Tags:            []string{"network"},
						ProcessingHints: map[string]any{"isHighPriority": true},
						SequenceNumber:  1,
					},
				},
			},
			BatchMetadata: PuppeteerBatchMetadata{
				CreatedAt:  time.Now().UnixMilli(),
				EventCount: 1,
				TotalSize:  10,
				JobID:      reserved.ID,
			},
		}
		postEventsHTTP(t, ts.URL, batch)

		// 7) Runner marks job complete via HTTP
		completeJobHTTP(t, ts.URL, reserved.ID)

		// 8) Verify job is completed via repository (production code)
		got, err := jobRepo.GetByID(ctx, reserved.ID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if got.Status != model.JobStatusCompleted {
			t.Fatalf("job not completed; status=%s", got.Status)
		}

		// 9) Verify events are linked to the job via repository
		page, err := eventRepo.ListByJob(ctx, model.EventListByJobOptions{JobID: reserved.ID, Limit: 10, Offset: 0})
		if err != nil {
			t.Fatalf("list events by job: %v", err)
		}
		if len(page.Events) == 0 {
			t.Fatalf("expected events linked to job %s", reserved.ID)
		}
	})
}

// workflowTestHarness groups dependencies for workflow testing.
type workflowTestHarness struct {
	client      *testClient
	jobRepo     *data.JobRepo
	eventRepo   *data.EventRepo
	sourceRepo  *data.SourceRepo
	sourceCache *core.SourceCacheService
}

//revive:disable-next-line:argument-limit
func newWorkflowTestHarness(
	baseURL string,
	jobRepo *data.JobRepo,
	eventRepo *data.EventRepo,
	sourceRepo *data.SourceRepo,
	sourceCache *core.SourceCacheService,
) *workflowTestHarness {
	return &workflowTestHarness{
		client:      newTestClient(baseURL),
		jobRepo:     jobRepo,
		eventRepo:   eventRepo,
		sourceRepo:  sourceRepo,
		sourceCache: sourceCache,
	}
}

// createTestEventBatches creates sample event batches for testing live event streaming.
// Uses stable timestamps to avoid flakiness from time-based ordering.
func createTestEventBatches(jobID string) []PuppeteerEventBatch {
	baseTime := int64(1640995200000) // 2022-01-01 00:00:00 UTC in milliseconds

	return []PuppeteerEventBatch{
		{
			BatchID:   "batch-1",
			SessionID: jobID,
			Events: []PuppeteerEvent{
				{
					ID:     "e1",
					Method: "Page.loadEventFired",
					Params: PuppeteerEventParams{
						Timestamp: baseTime,
						Payload:   map[string]any{"timestamp": baseTime},
					},
					Metadata: PuppeteerEventMetadata{
						Category:       "page",
						Tags:           []string{"page", "load"},
						SequenceNumber: 1,
					},
				},
			},
			BatchMetadata: PuppeteerBatchMetadata{
				CreatedAt:  baseTime,
				EventCount: 1,
				TotalSize:  50,
				JobID:      jobID,
			},
		},
		{
			BatchID:   "batch-2",
			SessionID: jobID,
			Events: []PuppeteerEvent{
				{
					ID:     "e2",
					Method: "Network.requestWillBeSent",
					Params: PuppeteerEventParams{
						Timestamp: baseTime + 100,
						Payload:   map[string]any{"url": "https://example.com", "method": "GET"},
					},
					Metadata: PuppeteerEventMetadata{
						Category:       "network",
						Tags:           []string{"network", "request"},
						SequenceNumber: 2,
					},
				},
				{
					ID:     "e3",
					Method: "Network.responseReceived",
					Params: PuppeteerEventParams{
						Timestamp: baseTime + 200,
						Payload:   map[string]any{"url": "https://example.com", "status": 200},
					},
					Metadata: PuppeteerEventMetadata{
						Category:       "network",
						Tags:           []string{"network", "response"},
						SequenceNumber: 3,
					},
				},
			},
			BatchMetadata: PuppeteerBatchMetadata{
				CreatedAt:  baseTime + 100,
				EventCount: 2,
				TotalSize:  120,
				JobID:      jobID,
			},
		},
	}
}

// verifyEventMethods checks that all expected event methods are present in the results.
func verifyEventMethods(t testutil.TestingTB, events []*model.Event, expected []string) {
	t.Helper()
	expectedMethods := make(map[string]bool)
	for _, method := range expected {
		expectedMethods[method] = true
	}

	actualMethods := make(map[string]bool)
	for _, event := range events {
		actualMethods[event.EventType] = true
	}

	for method := range expectedMethods {
		if !actualMethods[method] {
			t.Fatalf("expected event method %s not found in results", method)
		}
	}
}

// simulateWorkflowSteps performs the core workflow steps: create source, enqueue job, reserve, cache check.
func (h *workflowTestHarness) simulateWorkflowSteps(ctx context.Context, t testutil.TestingTB) model.Job {
	t.Helper()
	// Step 1: Create a test source with auto-enqueue (mimics SourceService.Create with Test=true)
	testSource := createSourceAndCache(ctx, t, h.sourceRepo, h.sourceCache)

	// Step 2: Auto-enqueue a browser job for the test source (mimics service layer behavior)
	testJob := createJobHTTP(t, h.client.baseURL, &model.CreateJobRequest{
		Type:     model.JobTypeBrowser,
		Payload:  json.RawMessage(fmt.Sprintf(`{"action":"run","source_id":"%s"}`, testSource.ID)),
		SourceID: &testSource.ID,
		IsTest:   true,
	})

	// Step 3: Simulate runner reserving the job
	reserved := reserveNextHTTP(t, reserveNextConfig{
		BaseURL:  h.client.baseURL,
		LeaseSec: 30,
		WaitSec:  1,
	})
	if reserved.ID != testJob.ID {
		t.Fatalf("expected reserved job %s, got %s", testJob.ID, reserved.ID)
	}

	// Step 4: Verify source content is cached and accessible
	cachedContent, err := h.sourceCache.GetCachedSourceContent(ctx, testSource.ID)
	if err != nil {
		t.Fatalf("get cached source content: %v", err)
	}
	if string(cachedContent) != testSource.Value {
		t.Fatalf("cached content mismatch: got %q, want %q", string(cachedContent), testSource.Value)
	}

	return reserved
}

// validateRepositoryState performs final validation of job and event state in the repository.
func (h *workflowTestHarness) validateRepositoryState(ctx context.Context, t testutil.TestingTB, jobID string) {
	t.Helper()
	// Verify job is completed in repository
	finalJob, err := h.jobRepo.GetByID(ctx, jobID)
	if err != nil {
		t.Fatalf("get final job: %v", err)
	}
	if finalJob.Status != model.JobStatusCompleted {
		t.Fatalf("job not completed in repository: status=%s", finalJob.Status)
	}

	// Verify events are linked to the job in repository
	repoPage, err := h.eventRepo.ListByJob(ctx, model.EventListByJobOptions{
		JobID:  jobID,
		Limit:  10,
		Offset: 0,
	})
	if err != nil {
		t.Fatalf("list events by job: %v", err)
	}
	if len(repoPage.Events) != 3 {
		t.Fatalf("expected 3 events in repository, got %d", len(repoPage.Events))
	}

	// Verify all events are linked to the correct job
	for _, event := range repoPage.Events {
		if event.SourceJobID == nil || *event.SourceJobID != jobID {
			t.Fatalf("event %s not linked to job %s", event.ID, jobID)
		}
	}
}

// Test_Workflow_SourceTestRun_WithLiveEvents tests the complete Source test workflow
// that mimics the frontend user experience: create test source, auto-enqueue job,
// simulate runner work with live event streaming, and poll for status/events.
func Test_Workflow_SourceTestRun_WithLiveEvents(t *testing.T) {
	// Require Postgres test DB
	testutil.SkipIfNoTestDB(t)

	// Require Redis test instance
	addr, ok := redisTestAddr(t)
	if !ok {
		t.Skip("redis test instance unavailable; run docker-compose profile 'test'")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Wire repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		eventRepo := &data.EventRepo{DB: db}
		sourceRepo := data.NewSourceRepo(db)

		// Wire services
		jobSvc := service.MustNewJobService(service.JobServiceOptions{
			Repo:         jobRepo,
			DefaultLease: 30 * time.Second,
		})
		eventSvc := service.MustNewEventService(service.EventServiceOptions{
			Repo: eventRepo,
			Config: service.EventServiceConfig{
				MaxBatch:                 1000,
				ThreatScoreProcessCutoff: 0.7,
			},
		})

		// Wire cache for sources (using real Redis)
		redisClient := data.NewRedisClient(data.RedisConfig{Addr: addr})
		defer func() { _ = redisClient.Close() }()
		cacheRepo := data.NewRedisCacheRepo(redisClient)
		sourceCache := core.NewSourceCacheService(core.SourceCacheServiceOptions{
			Cache:   cacheRepo,
			Sources: sourceRepo,
			Secrets: nil,
			Config:  core.DefaultSourceCacheConfig(),
		})

		// Create HTTP server with all endpoints
		mux := NewRouter(RouterServices{Jobs: jobSvc, Events: eventSvc, IsDev: true})
		ts := httptest.NewServer(mux)
		defer ts.Close()

		ctx := context.Background()

		// Create test harness to group dependencies
		harness := newWorkflowTestHarness(ts.URL, jobRepo, eventRepo, sourceRepo, sourceCache)

		// Steps 1-4: Create source, enqueue job, reserve, and verify caching
		reserved := harness.simulateWorkflowSteps(ctx, t)

		// Step 5: Simulate runner posting multiple event batches (live streaming)
		eventBatches := createTestEventBatches(reserved.ID)
		for _, batch := range eventBatches {
			harness.client.postEvents(t, batch)
		}

		// Step 6: Simulate frontend polling for job status (mimics source_form.js)
		statusResp := harness.client.getJobStatus(t, reserved.ID)
		if statusResp.Status != model.JobStatusRunning {
			t.Fatalf("expected job status %s, got %s", model.JobStatusRunning, statusResp.Status)
		}

		// Step 7: Simulate frontend polling for events (mimics source_form.js fetchEvents)
		events := harness.client.getJobEvents(t, reserved.ID, 100, 0)
		if len(events) != 3 {
			t.Fatalf("expected 3 events, got %d", len(events))
		}

		// Verify event content (order may vary due to concurrent inserts)
		expectedMethods := []string{"Page.loadEventFired", "Network.requestWillBeSent", "Network.responseReceived"}
		verifyEventMethods(t, events, expectedMethods)

		// Step 8: Simulate pagination - get events with offset
		moreEvents := harness.client.getJobEvents(t, reserved.ID, 2, 1)
		if len(moreEvents) != 2 {
			t.Fatalf("expected 2 events with offset, got %d", len(moreEvents))
		}

		// Step 9: Runner completes the job
		harness.client.completeJob(t, reserved.ID)

		// Step 10: Final status check - should be completed
		finalStatus := harness.client.getJobStatus(t, reserved.ID)
		if finalStatus.Status != model.JobStatusCompleted {
			t.Fatalf("expected final status %s, got %s", model.JobStatusCompleted, finalStatus.Status)
		}

		// Step 11: Verify all events are still accessible after job completion
		allEvents := harness.client.getJobEvents(t, reserved.ID, 100, 0)
		if len(allEvents) != 3 {
			t.Fatalf("expected 3 events after completion, got %d", len(allEvents))
		}

		// Step 12: Verify job and events via repository (backend validation)
		harness.validateRepositoryState(ctx, t, reserved.ID)
	})
}

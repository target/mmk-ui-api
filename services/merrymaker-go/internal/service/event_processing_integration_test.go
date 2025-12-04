package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service/rules"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEventProcessingIntegration_EndToEnd(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		redisClient := testutil.SetupTestRedis(t)
		defer redisClient.Close()

		ctx := context.Background()

		// Create test site
		site := createTestSite(t, db, "integration-test-site")

		// Share the latest created source job ID across subtests
		var lastSourceJobID string

		// Set up repositories
		eventRepo := &data.EventRepo{DB: db}
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		alertRepo := data.NewAlertRepo(db)
		seenRepo := data.NewSeenDomainRepo(db)
		allowlistRepo := data.NewDomainAllowlistRepo(db)
		redisRepo := data.NewRedisCacheRepo(redisClient)

		// Set up services
		eventService := MustNewEventService(EventServiceOptions{
			Repo: eventRepo,
			Config: EventServiceConfig{
				MaxBatch:                 1000,
				ThreatScoreProcessCutoff: 0.7,
			},
		})
		jobService := MustNewJobService(JobServiceOptions{
			Repo:         jobRepo,
			DefaultLease: 30 * time.Second,
		})
		alertService := MustNewAlertService(AlertServiceOptions{
			Repo: alertRepo,
		})

		// Set up rules engine components
		caches := buildTestRulesCaches(redisRepo, seenRepo)
		allowlistChecker := rules.NewDomainAllowlistChecker(rules.DomainAllowlistCheckerOptions{
			Service: NewDomainAllowlistService(DomainAllowlistServiceOptions{
				Repo: allowlistRepo,
			}),
			CacheTTL:  5 * time.Minute,
			CacheSize: 1000,
		})

		unknownDomainEvaluator := &rules.UnknownDomainEvaluator{
			Caches:    caches,
			Alerter:   alertService,
			Allowlist: allowlistChecker,
			AlertTTL:  time.Hour, // 1 hour dedupe TTL
		}

		// Set up orchestrator
		orchestrator := NewRulesOrchestrationService(RulesOrchestrationOptions{
			Events:                 eventRepo,
			Jobs:                   jobRepo,
			Caches:                 caches,
			BatchSize:              100,
			UnknownDomainEvaluator: unknownDomainEvaluator,
		})

		// Set up event filter
		filter := NewEventFilterService()

		// Test 1: Event ingestion with filtering
		t.Run("event_ingestion_with_filtering", func(t *testing.T) {
			events := []model.RawEvent{
				{
					Type: "Network.requestWillBeSent",
					Data: json.RawMessage(`{"request":{"url":"https://example.com/path"}}`),
				},
				{
					Type: "Network.responseReceived",
					Data: json.RawMessage(`{"response":{"url":"https://malicious.com/path"}}`),
				},
				{
					Type: "Page.loadEventFired",
					Data: json.RawMessage(`{}`),
				},
			}

			// Create source job
			sourceJob, err := jobService.Create(ctx, &model.CreateJobRequest{
				Type:     model.JobTypeBrowser,
				Payload:  json.RawMessage(`{}`),
				SiteID:   &site.ID,
				Priority: 50,
			})
			require.NoError(t, err)

			lastSourceJobID = sourceJob.ID

			// Test event filtering
			shouldProcessMap := filter.ShouldProcessEvents(events)
			assert.True(t, shouldProcessMap[0])  // Network.requestWillBeSent should be processed
			assert.True(t, shouldProcessMap[1])  // Network.responseReceived should be processed
			assert.False(t, shouldProcessMap[2]) // Page.loadEventFired should not be processed

			// Insert events with processing flags
			bulkReq := model.BulkEventRequest{
				SessionID:   uuid.NewString(),
				SourceJobID: &sourceJob.ID,
				Events:      events,
			}

			count, err := eventService.BulkInsertWithProcessingFlags(ctx, bulkReq, shouldProcessMap)
			require.NoError(t, err)
			assert.Equal(t, 3, count)

			// Verify events were inserted correctly
			insertedPage, err := eventService.ListByJob(ctx, model.EventListByJobOptions{
				JobID: sourceJob.ID,
				Limit: 10,
			})
			require.NoError(t, err)
			insertedEvents := insertedPage.Events
			assert.Len(t, insertedEvents, 3)

			// Check processing flags
			processableCount := 0
			for _, event := range insertedEvents {
				if event.ShouldProcess {
					processableCount++
				}
			}
			assert.Equal(t, 2, processableCount) // Only network events should be processable
		})

		// Test 2: Rules job enqueue and processing
		t.Run("rules_job_processing", func(t *testing.T) {
			// Get processable events
			eventsPage, err := eventService.ListByJob(ctx, model.EventListByJobOptions{
				JobID: lastSourceJobID,
				Limit: 10,
			})
			require.NoError(t, err)
			events := eventsPage.Events

			var processableEventIDs []string
			for _, event := range events {
				if event.ShouldProcess && !event.Processed {
					processableEventIDs = append(processableEventIDs, event.ID)
				}
			}

			if len(processableEventIDs) == 0 {
				t.Skip("No processable events found")
			}

			// Enqueue rules job
			rulesJob, err := orchestrator.EnqueueRulesProcessingJob(ctx, EnqueueRulesJobRequest{
				EventIDs: processableEventIDs,
				SiteID:   site.ID,
				Scope:    "default",
				Priority: 50,
			})
			require.NoError(t, err)
			require.NotNil(t, rulesJob)

			// Process the rules job
			err = orchestrator.ProcessRulesJob(ctx, rulesJob)
			require.NoError(t, err)

			// Verify events were marked as processed
			processedPage, err := eventService.ListByJob(ctx, model.EventListByJobOptions{
				JobID: lastSourceJobID,
				Limit: 10,
			})
			require.NoError(t, err)
			processedEvents := processedPage.Events

			processedCount := 0
			for _, event := range processedEvents {
				if event.Processed {
					processedCount++
				}
			}
			assert.Positive(t, processedCount, "Some events should be marked as processed")
		})

		// Test 3: Alert generation
		t.Run("alert_generation", func(t *testing.T) {
			// Check if any alerts were created
			// Note: This is a basic check - in a real scenario we'd need to set up
			// specific conditions that would trigger alerts
			alerts, err := alertRepo.List(ctx, &model.AlertListOptions{
				SiteID: &site.ID,
				Limit:  10,
			})
			require.NoError(t, err)

			// We expect at least some processing to have occurred
			// The exact number of alerts depends on the test data and rules
			t.Logf("Generated %d alerts during processing", len(alerts))
		})
	})
}

func createTestSite(t *testing.T, db *sql.DB, name string) *model.Site {
	ctx := context.Background()

	// Create a minimal source first; Site requires SourceID
	sourceRepo := data.NewSourceRepo(db)
	srcName := fmt.Sprintf("%s-src-%d", name, time.Now().UnixNano())
	src, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
		Name:  srcName,
		Value: "console.log('test');",
	})
	require.NoError(t, err)

	siteRepo := data.NewSiteRepo(db)
	site, err := siteRepo.Create(ctx, &model.CreateSiteRequest{
		Name:            name,
		RunEveryMinutes: 60, // Run every hour
		SourceID:        src.ID,
	})
	require.NoError(t, err)
	return site
}

func buildTestRulesCaches(redisRepo core.CacheRepository, seenRepo core.SeenDomainRepository) rules.Caches {
	localCache := rules.NewLocalLRU(rules.DefaultLocalLRUConfig())
	ttl := rules.DefaultCacheTTL()

	return rules.Caches{
		Seen: rules.NewSeenDomainsCache(rules.SeenDomainsCacheDeps{
			Local: localCache,
			Redis: redisRepo,
			Repo:  seenRepo,
			TTL:   ttl,
		}),
		// Add other caches as needed for testing
	}
}

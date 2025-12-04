package rulesrunner

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRulesRunner_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()

		// Create repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		eventRepo := &data.EventRepo{DB: db}

		// Create test events with known IDs
		sessionID := uuid.New().String()
		eventID1 := uuid.New().String()
		eventID2 := uuid.New().String()

		// Insert test events directly using SQL to avoid FK constraints
		_, err := db.ExecContext(ctx, `
			INSERT INTO events (id, session_id, event_type, event_data, should_process, created_at)
			VALUES
				($1, $2, 'Network.requestWillBeSent', '{"request":{"url":"https://example.com/api"}}', true, NOW()),
				($3, $2, 'Network.responseReceived', '{"response":{"url":"https://suspicious.com/malware"}}', true, NOW())
		`, eventID1, sessionID, eventID2)
		require.NoError(t, err)

		// Verify events were inserted
		eventIDs := []string{eventID1, eventID2}
		insertedEvents, err := eventRepo.GetByIDs(ctx, eventIDs)
		require.NoError(t, err)
		require.Len(t, insertedEvents, 2)

		// Create rules orchestration service (without rule evaluators for this test)
		orchestrator := service.NewRulesOrchestrationService(service.RulesOrchestrationOptions{
			Events:    eventRepo,
			Jobs:      jobRepo,
			BatchSize: 100,
		})

		// Create a rules job payload manually to avoid site FK constraint
		payload := service.RulesJobPayload{
			EventIDs: eventIDs,
			SiteID:   "test-site",
			Scope:    "default",
		}
		payloadBytes, err := json.Marshal(payload)
		require.NoError(t, err)

		// Create the job directly without site ID
		jobReq := &model.CreateJobRequest{
			Type:       model.JobTypeRules,
			Payload:    payloadBytes,
			SiteID:     nil, // No site ID to avoid FK constraint
			Priority:   50,
			MaxRetries: 3,
		}

		job, err := jobRepo.Create(ctx, jobReq)
		require.NoError(t, err)
		require.NotNil(t, job)

		assert.Equal(t, model.JobTypeRules, job.Type)
		assert.Equal(t, 50, job.Priority)

		// Process the job
		err = orchestrator.ProcessRulesJob(ctx, job)
		require.NoError(t, err)

		t.Logf("Successfully processed rules job %s with %d events", job.ID, len(eventIDs))
	})
}

func TestRulesRunner_NewRunner(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Test creating a new rules runner
		runner, err := NewRunner(RunnerOptions{
			DB:          db,
			RedisClient: nil, // No Redis for this test
			Concurrency: 1,
			Lease:       30,
		})

		require.NoError(t, err)
		require.NotNil(t, runner)

		// Verify runner was created successfully
		// Note: fields are private, so we can only verify the runner is not nil
		assert.NotNil(t, runner)
	})
}

func TestRulesRunner_DependencyResolution(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Test dependency resolution
		opts := RunnerOptions{
			DB:          db,
			RedisClient: nil,
			Concurrency: 1,
			Lease:       30,
		}

		deps := resolveDependencies(opts)
		require.NotNil(t, deps)

		// Verify all required dependencies are resolved
		assert.NotNil(t, deps.jobsRepo)
		assert.NotNil(t, deps.eventsRepo)
		assert.NotNil(t, deps.alertRepo)
		assert.NotNil(t, deps.seenRepo)
		assert.NotNil(t, deps.alertSinkRepo)
		assert.NotNil(t, deps.siteRepo)
		assert.NotNil(t, deps.iocsRepo)
		// filesRepo is nil since ProcessedFileRepo doesn't exist yet
		assert.Nil(t, deps.filesRepo)
		// cacheRepo is nil since no Redis client provided
		assert.Nil(t, deps.cacheRepo)
	})
}

func TestRulesRunner_JobProcessing(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Create a runner
		runner, err := NewRunner(RunnerOptions{
			DB:          db,
			RedisClient: nil,
			Concurrency: 1,
			Lease:       30,
		})
		require.NoError(t, err)

		// Start the runner in a goroutine
		errCh := make(chan error, 1)
		go func() {
			if err := runner.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				errCh <- err
			}
		}()

		// Wait a short time to let the runner start
		time.Sleep(100 * time.Millisecond)

		// Cancel the context to stop the runner
		cancel()

		// Check for any errors
		select {
		case err := <-errCh:
			t.Fatalf("Runner failed: %v", err)
		case <-time.After(1 * time.Second):
			// Runner stopped cleanly
		}
	})
}

package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchedulerService_Integration_WithSourceCaching(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup Redis client for testing
		redisClient := testutil.SetupTestRedis(t)
		defer redisClient.Close()

		// Clean up Redis and database state
		redisClient.FlushDB(context.Background())

		// Clean up any existing jobs and scheduled tasks
		ctx := context.Background()
		_, _ = db.ExecContext(ctx, "DELETE FROM jobs")
		_, _ = db.ExecContext(ctx, "DELETE FROM scheduled_jobs")

		// Setup repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		scheduledRepo := data.NewScheduledJobsRepo(db)
		sourceRepo := data.NewSourceRepo(db)
		cacheRepo := data.NewRedisCacheRepo(redisClient)

		// Create a test source
		source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
			Name:  "test-source",
			Value: "console.log('Hello from cached source!');",
		})
		require.NoError(t, err)

		// Setup source cache service
		cacheConfig := core.DefaultSourceCacheConfig()
		cacheConfig.TTL = 5 * time.Minute // Short TTL for testing
		sourceCacheService := core.NewSourceCacheService(core.SourceCacheServiceOptions{
			Cache:   cacheRepo,
			Sources: sourceRepo,
			Secrets: nil,
			Config:  cacheConfig,
		})

		// Setup scheduler service with caching
		schedulerConfig := core.DefaultSchedulerConfig()
		schedulerConfig.BatchSize = 1
		schedulerConfig.DefaultJobType = model.JobTypeBrowser

		scheduler := NewSchedulerService(SchedulerServiceOptions{
			Repo:            scheduledRepo,
			Jobs:            jobRepo,
			JobIntrospector: jobRepo,
			Config:          &schedulerConfig,
			SourceCache:     sourceCacheService,
		})

		// Create a scheduled task with source_id in payload
		taskPayload := struct {
			SiteID   string `json:"site_id"`
			SourceID string `json:"source_id"`
		}{
			SiteID:   "site-123",
			SourceID: source.ID,
		}
		payloadBytes, err := json.Marshal(taskPayload)
		require.NoError(t, err)

		adminRepo := data.NewScheduledJobsAdminRepo(db)
		err = adminRepo.UpsertByTaskName(ctx, domain.UpsertTaskParams{
			TaskName: "test:site:123",
			Payload:  payloadBytes,
			Interval: time.Minute,
		})
		require.NoError(t, err)

		// Verify source is not cached initially
		cachedContent, err := sourceCacheService.GetCachedSourceContent(ctx, source.ID)
		require.NoError(t, err)
		assert.Nil(t, cachedContent)

		// Run scheduler tick - this should cache the source content
		now := time.Now()
		processed, err := scheduler.Tick(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)

		// Verify source content is now cached
		cachedContent, err = sourceCacheService.GetCachedSourceContent(ctx, source.ID)
		require.NoError(t, err)
		assert.NotNil(t, cachedContent)
		assert.Equal(t, []byte(source.Value), cachedContent)

		// Verify a browser job was created
		jobs, err := jobRepo.Stats(ctx, model.JobTypeBrowser)
		require.NoError(t, err)
		assert.Equal(t, 1, jobs.Pending)

		// Verify the job has the correct payload with cached script content
		job, err := jobRepo.ReserveNext(ctx, model.JobTypeBrowser, 30)
		require.NoError(t, err)

		// Parse the actual payload
		var actualPayload map[string]any
		err = json.Unmarshal(job.Payload, &actualPayload)
		require.NoError(t, err)

		// Verify the payload contains the expected fields plus the cached script
		expectedPayload := map[string]any{
			"site_id":   "site-123",
			"source_id": source.ID,
			"script":    "console.log('Hello from cached source!');",
		}
		assert.Equal(t, expectedPayload, actualPayload)

		// Verify scheduler metadata is present
		var metadata map[string]any
		err = json.Unmarshal(job.Metadata, &metadata)
		require.NoError(t, err)
		assert.Equal(t, "test:site:123", metadata["scheduler.task_name"])
		assert.Contains(t, metadata, "scheduler.fire_key")
	})
}

func TestSchedulerService_Integration_CachingOptional(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup repositories without cache
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		scheduledRepo := data.NewScheduledJobsRepo(db)

		// Setup scheduler service WITHOUT caching
		schedulerConfig := core.DefaultSchedulerConfig()
		schedulerConfig.BatchSize = 1
		schedulerConfig.DefaultJobType = model.JobTypeBrowser

		scheduler := NewSchedulerService(SchedulerServiceOptions{
			Repo:            scheduledRepo,
			Jobs:            jobRepo,
			JobIntrospector: jobRepo,
			Config:          &schedulerConfig,
			SourceCache:     nil, // No caching
		})

		// Create a scheduled task
		taskPayload := struct {
			SiteID   string `json:"site_id"`
			SourceID string `json:"source_id"`
		}{
			SiteID:   "site-456",
			SourceID: "source-456",
		}
		payloadBytes, err := json.Marshal(taskPayload)
		require.NoError(t, err)

		ctx := context.Background()
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		err = adminRepo.UpsertByTaskName(ctx, domain.UpsertTaskParams{
			TaskName: "test:site:456",
			Payload:  payloadBytes,
			Interval: time.Minute,
		})
		require.NoError(t, err)

		// Run scheduler tick - should work without caching
		now := time.Now()
		processed, err := scheduler.Tick(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)

		// Verify a browser job was created
		jobs, err := jobRepo.Stats(ctx, model.JobTypeBrowser)
		require.NoError(t, err)
		assert.Equal(t, 1, jobs.Pending)
	})
}

func TestSchedulerService_Integration_NonBrowserJobs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup Redis client for testing
		redisClient := testutil.SetupTestRedis(t)
		defer redisClient.Close()

		// Clean up Redis
		redisClient.FlushDB(context.Background())

		// Setup repositories
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		scheduledRepo := data.NewScheduledJobsRepo(db)
		sourceRepo := data.NewSourceRepo(db)
		cacheRepo := data.NewRedisCacheRepo(redisClient)

		// Setup source cache service
		sourceCacheService := core.NewSourceCacheService(core.SourceCacheServiceOptions{
			Cache:   cacheRepo,
			Sources: sourceRepo,
			Secrets: nil,
			Config:  core.DefaultSourceCacheConfig(),
		})

		// Setup scheduler service for RULES jobs (not browser)
		schedulerConfig := core.DefaultSchedulerConfig()
		schedulerConfig.BatchSize = 1
		schedulerConfig.DefaultJobType = model.JobTypeRules // Not browser

		scheduler := NewSchedulerService(SchedulerServiceOptions{
			Repo:            scheduledRepo,
			Jobs:            jobRepo,
			JobIntrospector: jobRepo,
			Config:          &schedulerConfig,
			SourceCache:     sourceCacheService,
		})

		// Create a scheduled task
		taskPayload := struct {
			RuleID string `json:"rule_id"`
		}{
			RuleID: "rule-123",
		}
		payloadBytes, err := json.Marshal(taskPayload)
		require.NoError(t, err)

		ctx := context.Background()
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		err = adminRepo.UpsertByTaskName(ctx, domain.UpsertTaskParams{
			TaskName: "test:rule:123",
			Payload:  payloadBytes,
			Interval: time.Minute,
		})
		require.NoError(t, err)

		// Run scheduler tick - should NOT attempt caching for non-browser jobs
		now := time.Now()
		processed, err := scheduler.Tick(ctx, now)
		require.NoError(t, err)
		assert.Equal(t, 1, processed)

		// Verify a rules job was created
		jobs, err := jobRepo.Stats(ctx, model.JobTypeRules)
		require.NoError(t, err)
		assert.Equal(t, 1, jobs.Pending)
	})
}

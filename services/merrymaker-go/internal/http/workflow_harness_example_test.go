//nolint:ireturn // Returning interfaces here is intentional for provider simplicity in example tests.

//go:build example

package httpx

import (
	"database/sql"
	"testing"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

// repositoryProvider implements workflowtest.RepositoryProvider to avoid import cycles.
type repositoryProvider struct {
	db *sql.DB
}

//lint:ignore ireturn Returning interfaces simplifies test harness and avoids import cycles.
func (p *repositoryProvider) JobRepository() core.JobRepository {
	return data.NewJobRepo(p.db, data.RepoConfig{})
}

//lint:ignore ireturn Returning interfaces simplifies test harness and avoids import cycles.
func (p *repositoryProvider) EventRepository() core.EventRepository {
	return &data.EventRepo{DB: p.db}
}

//lint:ignore ireturn Returning interfaces simplifies test harness and avoids import cycles.
func (p *repositoryProvider) SourceRepository() core.SourceRepository {
	return data.NewSourceRepo(p.db)
}

// cacheProvider implements testutil.CacheProvider for Redis tests.
type cacheProvider struct{}

//lint:ignore ireturn Returning interfaces simplifies test harness and avoids import cycles.
func (p *cacheProvider) CacheRepository(client *redis.Client) core.CacheRepository {
	return data.NewRedisCacheRepo(client)
}

// TestWorkflowHarnessUsageExample demonstrates how to use the workflow harness
// from outside the testutil package, avoiding import cycles.
func TestWorkflowHarnessUsageExample(t *testing.T) {
	// Create options with repository provider
	opts := testutil.DefaultWorkflowOptions()
	opts.RepositoryProvider = &repositoryProvider{}

	// Use the workflow harness
	testutil.WithWorkflowHarness(t, opts, func(harness *testutil.WorkflowTestHarness) {
		// Verify harness is properly initialized
		assert.NotNil(t, harness.JobRepo)
		assert.NotNil(t, harness.EventRepo)
		assert.NotNil(t, harness.SourceRepo)
		assert.NotNil(t, harness.JobSvc)
		assert.NotNil(t, harness.EventSvc)

		// Create HTTP client for API calls
		client := harness.NewHTTPClient()
		assert.NotNil(t, client)

		// Create workflow helpers
		helpers := harness.NewWorkflowHelpers()
		assert.NotNil(t, helpers)

		// Test event batch creation
		batch := testutil.CreateSimpleEventBatch("test-batch", "", "job-123")
		assert.Equal(t, "test-batch", batch.BatchID)
		assert.NotEmpty(t, batch.SessionID)
		assert.Equal(t, "job-123", batch.BatchMetadata.JobID)
	})
}

// TestWorkflowHarnessWithRedisExample demonstrates Redis usage.
func TestWorkflowHarnessWithRedisExample(t *testing.T) {
	// Create Redis options with both providers
	opts := testutil.RedisWorkflowOptions()
	opts.RepositoryProvider = &repositoryProvider{}
	opts.CacheProvider = &cacheProvider{}

	// This test will be skipped if Redis is not available
	testutil.WithWorkflowHarness(t, opts, func(harness *testutil.WorkflowTestHarness) {
		// Verify Redis components are available
		assert.NotNil(t, harness.RedisClient)
		assert.NotNil(t, harness.CacheRepo)
		assert.NotNil(t, harness.SourceCache)
	})
}

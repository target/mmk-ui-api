package rules

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func TestAlertOnceCacheRedis_ConcurrentAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup Redis client for testing with automatic address detection
	client := testutil.SetupTestRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Create cache with Redis backend
	redisRepo := data.NewRedisCacheRepo(client)
	scope := ScopeKey{SiteID: "site1", Scope: "test"}
	dedupeKey := "concurrent-alert"
	ttl := time.Hour

	// Clear any existing state
	client.FlushDB(ctx)

	const numGoroutines = 10
	var wg sync.WaitGroup
	results := make([]bool, numGoroutines)
	errors := make([]error, numGoroutines)
	timestamps := make([]time.Time, numGoroutines)

	// Use a barrier to ensure all goroutines start as close to simultaneously as possible
	startBarrier := make(chan struct{})

	// Launch multiple goroutines that try to set the same alert
	for i := range numGoroutines {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Wait for all goroutines to be ready
			<-startBarrier

			// Create a separate cache instance to simulate different service instances
			localCache := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
			alertCache := NewAlertOnceCache(localCache, redisRepo)

			req := AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: ttl}
			start := time.Now()
			seen, err := alertCache.Seen(ctx, req)
			timestamps[index] = start
			results[index] = seen
			errors[index] = err
		}(i)
	}

	// Release all goroutines at once
	close(startBarrier)
	wg.Wait()

	// Check that all operations succeeded
	for i, err := range errors {
		require.NoError(t, err, "goroutine %d should not have error", i)
	}

	// Count how many returned false (first time seeing) vs true (already seen)
	firstTimeCount := 0
	alreadySeenCount := 0
	var firstTimeIndices []int
	for i, seen := range results {
		if seen {
			alreadySeenCount++
		} else {
			firstTimeCount++
			firstTimeIndices = append(firstTimeIndices, i)
		}
	}

	// If the test fails, provide detailed debugging information
	if firstTimeCount != 1 {
		t.Logf("Race condition detected! Expected exactly 1 first-time, got %d", firstTimeCount)
		t.Logf("First-time goroutines: %v", firstTimeIndices)
		for i, result := range results {
			t.Logf("Goroutine %d: seen=%v, timestamp=%v", i, result, timestamps[i])
		}
	}

	// Exactly one should have succeeded in setting the key (returned false)
	// All others should have seen it already existed (returned true)
	assert.Equal(t, 1, firstTimeCount, "exactly one goroutine should be first to see the alert")
	assert.Equal(t, numGoroutines-1, alreadySeenCount, "all other goroutines should see alert as already seen")

	// Verify the key exists in Redis
	exists, err := redisRepo.Exists(ctx, "rules:alertonce:site:site1:scope:test:key:concurrent-alert")
	require.NoError(t, err)
	assert.True(t, exists, "key should exist in Redis")
}

func TestAlertOnceCacheRedis_StressTest(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup Redis client for testing with automatic address detection
	client := testutil.SetupTestRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Create cache with Redis backend
	redisRepo := data.NewRedisCacheRepo(client)
	scope := ScopeKey{SiteID: "site1", Scope: "test"}
	ttl := time.Hour
	const iterations = 20
	const numGoroutines = 10

	for iter := range iterations {
		runStressTestIteration(ctx, t, client, redisRepo, scope, ttl, iter, numGoroutines)
	}
}

// runStressTestIteration runs a single iteration of the stress test.
//
//revive:disable-next-line:argument-limit
func runStressTestIteration(
	ctx context.Context,
	t *testing.T,
	client *redis.Client,
	redisRepo *data.RedisCacheRepo,
	scope ScopeKey,
	ttl time.Duration,
	iter, numGoroutines int,
) {
	// Clear any existing state
	client.FlushDB(ctx)

	dedupeKey := fmt.Sprintf("stress-alert-%d", iter)
	var wg sync.WaitGroup
	results := make([]bool, numGoroutines)
	errors := make([]error, numGoroutines)
	redisResults := make([]bool, numGoroutines) // Track direct Redis results

	// Use a barrier to ensure all goroutines start simultaneously
	startBarrier := make(chan struct{})

	// Launch multiple goroutines that try to set the same alert
	for i := range numGoroutines {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()

			// Wait for all goroutines to be ready
			<-startBarrier

			// Create a separate cache instance to simulate different service instances
			localCache := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
			alertCache := NewAlertOnceCache(localCache, redisRepo)

			req := AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: ttl}
			seen, err := alertCache.Seen(ctx, req)
			results[index] = seen
			errors[index] = err

			// For debugging, also test the direct Redis operation to see if it's working
			redisKey := fmt.Sprintf("rules:alertonce:site:%s:scope:%s:key:%s", scope.SiteID, scope.Scope, dedupeKey)
			wasSet, redisErr := redisRepo.SetIfNotExists(ctx, redisKey+"_debug", []byte("1"), ttl)
			redisResults[index] = wasSet
			if redisErr != nil {
				// Don't fail the test for debug operation errors
				redisResults[index] = false
			}
		}(i)
	}

	// Release all goroutines at once
	close(startBarrier)
	wg.Wait()

	// Extra diagnostics: if failure is detected, log Redis addr/DB and counts
	redisSetCount := 0
	for _, wasSet := range redisResults {
		if wasSet {
			redisSetCount++
		}
	}
	firstTimeCount := 0
	for _, seen := range results {
		if !seen {
			firstTimeCount++
		}
	}
	if firstTimeCount != 1 {
		opts := client.Options()
		t.Logf("Diagnostics: Redis addr=%s DB=%d (iter=%d)", opts.Addr, opts.DB, iter)
		t.Logf("Diagnostics: redisSetCount=%d firstTimeCount=%d", redisSetCount, firstTimeCount)
	}

	validateStressTestResults(t, iter, numGoroutines, results, redisResults, errors)
}

// validateStressTestResults validates the results of a stress test iteration.
//
//revive:disable-next-line:argument-limit
func validateStressTestResults(t *testing.T, iter, numGoroutines int, results, redisResults []bool, errors []error) {
	// Check that all operations succeeded
	for i, err := range errors {
		require.NoError(t, err, "iteration %d, goroutine %d should not have error", iter, i)
	}

	// Count Redis results first
	redisSetCount := 0
	for _, wasSet := range redisResults {
		if wasSet {
			redisSetCount++
		}
	}

	// Count cache results
	firstTimeCount := 0
	alreadySeenCount := 0
	for _, seen := range results {
		if seen {
			alreadySeenCount++
		} else {
			firstTimeCount++
		}
	}

	// If we detect a race condition, provide detailed debugging
	if firstTimeCount != 1 {
		t.Logf("Race condition detected in iteration %d!", iter)
		t.Logf("Cache Seen results: %d first-time, %d already-seen", firstTimeCount, alreadySeenCount)
		t.Logf(
			"Debug Redis SetIfNotExists results: %d succeeded, %d failed",
			redisSetCount,
			numGoroutines-redisSetCount,
		)
		for i := range numGoroutines {
			t.Logf("Goroutine %d: Cache seen=%v, Debug Redis wasSet=%v", i, results[i], redisResults[i])
		}
		t.Fatalf("Expected exactly 1 cache first-time, got %d cache first-times", firstTimeCount)
	}
}

func TestAlertOnceCacheRedis_SequentialAccess(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup Redis client for testing with automatic address detection
	client := testutil.SetupTestRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Create cache with Redis backend
	redisRepo := data.NewRedisCacheRepo(client)
	local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
	cache := NewAlertOnceCache(local, redisRepo)

	scope := ScopeKey{SiteID: "site1", Scope: "test"}
	dedupeKey := "sequential-alert"
	ttl := time.Hour

	// Clear any existing state
	client.FlushDB(ctx)

	// First call should return false (first time)
	req := AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: ttl}
	seen, err := cache.Seen(ctx, req)
	require.NoError(t, err)
	assert.False(t, seen, "first call should return false")

	// Subsequent calls should return true (already seen)
	for i := range 5 {
		seen, err := cache.Seen(ctx, req)
		require.NoError(t, err)
		assert.True(t, seen, "subsequent call %d should return true", i+1)
	}
}

func TestAlertOnceCacheRedis_DifferentScopes(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test")
	}

	// Setup Redis client for testing with automatic address detection
	client := testutil.SetupTestRedis(t)
	defer client.Close()

	ctx := context.Background()

	// Create cache with Redis backend
	redisRepo := data.NewRedisCacheRepo(client)
	local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
	cache := NewAlertOnceCache(local, redisRepo)

	scope1 := ScopeKey{SiteID: "site1", Scope: "scope1"}
	scope2 := ScopeKey{SiteID: "site1", Scope: "scope2"}
	dedupeKey := "same-alert"
	ttl := time.Hour

	// Clear any existing state
	client.FlushDB(ctx)

	// First call for scope1 should return false
	req1 := AlertSeenRequest{Scope: scope1, DedupeKey: dedupeKey, TTL: ttl}
	seen, err := cache.Seen(ctx, req1)
	require.NoError(t, err)
	assert.False(t, seen, "first call for scope1 should return false")

	// First call for scope2 should also return false (different scope)
	req2 := AlertSeenRequest{Scope: scope2, DedupeKey: dedupeKey, TTL: ttl}
	seen, err = cache.Seen(ctx, req2)
	require.NoError(t, err)
	assert.False(t, seen, "first call for scope2 should return false")

	// Second calls should return true for both scopes
	seen, err = cache.Seen(ctx, req1)
	require.NoError(t, err)
	assert.True(t, seen, "second call for scope1 should return true")

	seen, err = cache.Seen(ctx, req2)
	require.NoError(t, err)
	assert.True(t, seen, "second call for scope2 should return true")
}

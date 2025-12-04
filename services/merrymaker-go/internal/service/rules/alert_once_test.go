package rules

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

func TestAlertOnceCacheRedis_Seen(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	scope := ScopeKey{SiteID: "site1", Scope: "test"}
	dedupeKey := "alert123"
	ttl := time.Hour

	t.Run("first time seeing alert - no redis", func(t *testing.T) {
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, nil)

		req := AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: ttl}
		seen, err := cache.Seen(ctx, req)
		require.NoError(t, err)
		assert.False(t, seen, "first time should return false")

		// Second call should return true (cached locally)
		seen, err = cache.Seen(ctx, req)
		require.NoError(t, err)
		assert.True(t, seen, "second time should return true")
	})

	t.Run("first time seeing alert - with redis", func(t *testing.T) {
		mockRedis := core.NewMockCacheRepository(ctrl)
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, mockRedis)

		// SetIfNotExists should succeed (key doesn't exist)
		mockRedis.EXPECT().SetIfNotExists(ctx, gomock.Any(), []byte("1"), ttl).Return(true, nil)

		req := AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: ttl}
		seen, err := cache.Seen(ctx, req)
		require.NoError(t, err)
		assert.False(t, seen, "first time should return false")
	})

	t.Run("already seen alert - with redis", func(t *testing.T) {
		mockRedis := core.NewMockCacheRepository(ctrl)
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, mockRedis)

		// SetIfNotExists should fail (key already exists)
		mockRedis.EXPECT().SetIfNotExists(ctx, gomock.Any(), []byte("1"), ttl).Return(false, nil)

		req := AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: ttl}
		seen, err := cache.Seen(ctx, req)
		require.NoError(t, err)
		assert.True(t, seen, "already seen should return true")
	})

	t.Run("local cache hit", func(t *testing.T) {
		mockRedis := core.NewMockCacheRepository(ctrl)
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, mockRedis)

		// First call - Redis interaction
		mockRedis.EXPECT().SetIfNotExists(ctx, gomock.Any(), []byte("1"), ttl).Return(true, nil)

		req := AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: ttl}
		seen, err := cache.Seen(ctx, req)
		require.NoError(t, err)
		assert.False(t, seen)

		// Second call - should hit local cache, no Redis interaction
		seen, err = cache.Seen(ctx, req)
		require.NoError(t, err)
		assert.True(t, seen, "local cache should return true")
	})

	t.Run("validation errors", func(t *testing.T) {
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, nil)

		// Invalid scope
		invalidScope := ScopeKey{SiteID: "", Scope: "test"}
		invalidReq := AlertSeenRequest{Scope: invalidScope, DedupeKey: dedupeKey, TTL: ttl}
		_, err := cache.Seen(ctx, invalidReq)
		require.Error(t, err)

		// Empty dedupe key
		emptyReq := AlertSeenRequest{Scope: scope, DedupeKey: "", TTL: ttl}
		_, err = cache.Seen(ctx, emptyReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dedupe key is required")

		// Whitespace-only dedupe key
		whitespaceReq := AlertSeenRequest{Scope: scope, DedupeKey: "   ", TTL: ttl}
		_, err = cache.Seen(ctx, whitespaceReq)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "dedupe key is required")
	})

	t.Run("key normalization", func(t *testing.T) {
		mockRedis := core.NewMockCacheRepository(ctrl)
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, mockRedis)

		// Expect normalized key (lowercase, trimmed)
		expectedKey := "rules:alertonce:site:site1:scope:test:key:alert123"
		mockRedis.EXPECT().SetIfNotExists(ctx, expectedKey, []byte("1"), ttl).Return(true, nil)

		// Use mixed case and whitespace
		req := AlertSeenRequest{Scope: scope, DedupeKey: "  ALERT123  ", TTL: ttl}
		seen, err := cache.Seen(ctx, req)
		require.NoError(t, err)
		assert.False(t, seen)
	})
}

func TestAlertOnceCacheRedis_Peek(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ctx := context.Background()
	scope := ScopeKey{SiteID: "site1", Scope: "test"}
	dedupeKey := "alert123"
	key := "rules:alertonce:site:site1:scope:test:key:alert123"

	t.Run("local cache hit bypasses redis", func(t *testing.T) {
		mockRedis := core.NewMockCacheRepository(ctrl)
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, mockRedis)

		local.Set(key, []byte("1"), time.Minute)

		seen, err := cache.Peek(
			ctx,
			AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: time.Minute},
		)
		require.NoError(t, err)
		assert.True(t, seen, "local cache should short-circuit peek")
	})

	t.Run("redis exists seeds local cache when ttl > 0", func(t *testing.T) {
		mockRedis := core.NewMockCacheRepository(ctrl)
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, mockRedis)

		ttl := time.Minute
		mockRedis.EXPECT().Exists(ctx, key).Return(true, nil)

		req := AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: ttl}
		seen, err := cache.Peek(ctx, req)
		require.NoError(t, err)
		assert.True(t, seen, "redis existence should report seen")
		assert.True(t, local.Exists(key), "local cache should be seeded when ttl > 0")
	})

	t.Run("redis exists without ttl leaves local empty", func(t *testing.T) {
		mockRedis := core.NewMockCacheRepository(ctrl)
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, mockRedis)

		mockRedis.EXPECT().Exists(ctx, key).Return(true, nil)

		req := AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: 0}
		seen, err := cache.Peek(ctx, req)
		require.NoError(t, err)
		assert.True(t, seen)
		assert.False(t, local.Exists(key), "ttl=0 should avoid local cache population")
	})

	t.Run("redis error", func(t *testing.T) {
		mockRedis := core.NewMockCacheRepository(ctrl)
		local := NewLocalLRU(LocalLRUConfig{Capacity: 100, Now: time.Now})
		cache := NewAlertOnceCache(local, mockRedis)

		mockRedis.EXPECT().Exists(ctx, key).Return(false, errors.New("boom"))

		_, err := cache.Peek(
			ctx,
			AlertSeenRequest{Scope: scope, DedupeKey: dedupeKey, TTL: time.Minute},
		)
		require.Error(t, err)
		assert.Contains(t, err.Error(), key)
	})
}

func TestAlertOnceCacheRedis_SeenConcurrentStripedLocks(t *testing.T) {
	ctx := context.Background()
	scope := ScopeKey{SiteID: "site1", Scope: "concurrency"}
	local := NewLocalLRU(LocalLRUConfig{Capacity: 4096, Now: time.Now})
	cache := NewAlertOnceCache(local, nil)

	const (
		keyCount       = 256
		callsPerKey    = 8
		expectedMisses = 1
	)

	startGate := make(chan struct{})
	var wg sync.WaitGroup
	var mu sync.Mutex
	falseCounts := make(map[string]int)
	errCh := make(chan error, keyCount*callsPerKey)

	for i := range keyCount {
		domain := fmt.Sprintf("domain-%d.example", i)
		for range callsPerKey {
			wg.Add(1)
			go func(key string) {
				defer wg.Done()
				<-startGate

				seen, err := cache.Seen(ctx, AlertSeenRequest{
					Scope:     scope,
					DedupeKey: key,
					TTL:       time.Minute,
				})
				if err != nil {
					errCh <- err
					return
				}
				if !seen {
					mu.Lock()
					falseCounts[key]++
					mu.Unlock()
				}
			}(domain)
		}
	}

	close(startGate)
	wg.Wait()
	close(errCh)

	for err := range errCh {
		require.NoError(t, err)
	}

	mu.Lock()
	defer mu.Unlock()

	require.Len(t, falseCounts, keyCount, "each key should record exactly one miss")
	for key, count := range falseCounts {
		assert.Equalf(t, expectedMisses, count, "expected exactly one miss for key %s", key)
	}
}

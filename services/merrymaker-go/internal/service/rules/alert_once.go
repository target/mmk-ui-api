package rules

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
	"sync"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
)

const alertOnceLockStripeCount = 256

// alertOnceLockStripes provides striped synchronization across all AlertOnceCacheRedis instances
// within the same process. This bounds memory usage while still preventing local race conditions.
// Cross-process coordination continues to rely on Redis SET NX semantics.
//
//nolint:gochecknoglobals // process-wide coordination is intentional and safe; see comments above
var alertOnceLockStripes [alertOnceLockStripeCount]sync.Mutex

// AlertOnceCacheRedis enforces alert-once-per-scope using Redis with a small local assist.
type AlertOnceCacheRedis struct {
	local *LocalLRU
	redis core.CacheRepository
}

func NewAlertOnceCache(local *LocalLRU, redis core.CacheRepository) *AlertOnceCacheRedis {
	return &AlertOnceCacheRedis{
		local: local,
		redis: redis,
	}
}

// small helpers to reduce cyclomatic complexity.
func (a *AlertOnceCacheRedis) localExists(key string) bool {
	return a.local != nil && a.local.Exists(key)
}

func (a *AlertOnceCacheRedis) localSet(key string, ttl time.Duration) {
	if a.local != nil {
		a.local.Set(key, []byte("1"), ttl)
	}
}

// getKeyMutex returns the striped mutex protecting the given key.
// Striped locking keeps memory bounded while ensuring only one goroutine per stripe
// attempts the Redis coordination path at a time.
func (a *AlertOnceCacheRedis) getKeyMutex(key string) *sync.Mutex {
	idx := alertOnceStripeIndex(key)
	return &alertOnceLockStripes[idx]
}

func alertOnceStripeIndex(key string) int {
	hash := fnv.New32a()
	_, _ = hash.Write([]byte(key))
	return int(hash.Sum32() % uint32(alertOnceLockStripeCount))
}

// redisSetRequest groups parameters for Redis operations to keep parameter count ≤3.
type redisSetRequest struct {
	key string
	ttl time.Duration
}

func (a *AlertOnceCacheRedis) redisSetIfNotExists(
	ctx context.Context,
	req redisSetRequest,
) (bool, error) {
	if a.redis == nil {
		return false, nil
	}
	return a.redis.SetIfNotExists(ctx, req.key, []byte("1"), req.ttl)
}

func (a *AlertOnceCacheRedis) Seen(ctx context.Context, req AlertSeenRequest) (bool, error) {
	if err := req.Scope.Validate(); err != nil {
		return false, err
	}
	k := strings.ToLower(strings.TrimSpace(req.DedupeKey))
	if k == "" {
		return false, errors.New("dedupe key is required")
	}
	key := "rules:alertonce:site:" + req.Scope.SiteID + ":scope:" + req.Scope.Scope + ":key:" + k

	// Synchronize access per key to prevent race conditions across instances in-process.
	// We acquire the lock before any local or Redis checks to ensure exactly one goroutine
	// can attempt the Redis SetIfNotExists call per key.
	keyMutex := a.getKeyMutex(key)
	keyMutex.Lock()
	defer keyMutex.Unlock()

	// Check local cache inside the lock (another goroutine might have just set it)
	if a.localExists(key) {
		return true, nil
	}

	// No Redis backend - just use local cache
	if a.redis == nil {
		a.localSet(key, req.TTL)
		return false, nil
	}

	// Use atomic SetIfNotExists to avoid race conditions between multiple processes
	redisReq := redisSetRequest{key: key, ttl: req.TTL}
	wasSet, err := a.redisSetIfNotExists(ctx, redisReq)
	if err != nil {
		return false, err
	}

	// Cache locally regardless of whether we set the key or it already existed
	a.localSet(key, req.TTL)

	// Return true if we've seen this alert before (key already existed)
	return !wasSet, nil
}

func (a *AlertOnceCacheRedis) Peek(ctx context.Context, req AlertSeenRequest) (bool, error) {
	if err := req.Scope.Validate(); err != nil {
		return false, err
	}
	k := strings.ToLower(strings.TrimSpace(req.DedupeKey))
	if k == "" {
		return false, errors.New("dedupe key is required")
	}
	key := "rules:alertonce:site:" + req.Scope.SiteID + ":scope:" + req.Scope.Scope + ":key:" + k

	if a.localExists(key) {
		return true, nil
	}
	if a.redis == nil {
		return false, nil
	}

	exists, err := a.redis.Exists(ctx, key)
	if err != nil {
		return false, fmt.Errorf("alertonce peek exists key=%q: %w", key, err)
	}
	if exists && req.TTL > 0 {
		a.localSet(key, req.TTL)
	}
	return exists, nil
}

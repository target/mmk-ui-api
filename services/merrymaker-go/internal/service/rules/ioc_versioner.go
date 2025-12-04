package rules

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
)

// IOCVersioner defines behaviour for managing IOC cache versions.
type IOCVersioner interface {
	Current(ctx context.Context) (string, error)
	Bump(ctx context.Context) (string, error)
}

const (
	defaultIOCCacheVersionKey     = "rules:ioc:version"
	defaultIOCCacheVersionRefresh = time.Second
)

// IOCCacheVersioner keeps a lightweight version value in Redis (or in-memory fallback)
// so cache clients can invalidate host-specific entries when IOCs change.
type IOCCacheVersioner struct {
	redis   core.CacheRepository
	key     string
	refresh time.Duration

	mu        sync.RWMutex
	last      string
	lastFetch time.Time
	clock     func() time.Time
}

// NewIOCCacheVersioner constructs a cache versioner. When redis is nil it still
// works using process-local state, which is sufficient for single-node tests.
func NewIOCCacheVersioner(redis core.CacheRepository, key string, refresh time.Duration) *IOCCacheVersioner {
	if key == "" {
		key = defaultIOCCacheVersionKey
	}
	if refresh <= 0 {
		refresh = defaultIOCCacheVersionRefresh
	}
	return &IOCCacheVersioner{
		redis:   redis,
		key:     key,
		refresh: refresh,
		clock:   time.Now,
	}
}

// Current returns the cached version, refreshing from Redis when the refresh
// interval has elapsed. Errors from Redis are returned but the last known value
// is still provided so callers can continue operating.
func (v *IOCCacheVersioner) Current(ctx context.Context) (string, error) {
	now := v.clock()

	v.mu.RLock()
	last := v.last
	since := now.Sub(v.lastFetch)
	v.mu.RUnlock()

	if last != "" && since <= v.refresh {
		return last, nil
	}

	v.mu.Lock()
	defer v.mu.Unlock()

	// Re-check after acquiring the write lock to avoid duplicate fetches.
	if v.last != "" && now.Sub(v.lastFetch) <= v.refresh {
		return v.last, nil
	}

	if v.redis == nil {
		if v.last == "" {
			v.last = "0"
		}
		v.lastFetch = now
		return v.last, nil
	}

	b, err := v.redis.Get(ctx, v.key)
	if err != nil {
		// Preserve previously known value for resilience.
		v.lastFetch = now
		if v.last == "" {
			v.last = "0"
		}
		return v.last, err
	}

	if len(b) == 0 {
		v.last = "0"
	} else {
		v.last = string(b)
	}
	v.lastFetch = now

	return v.last, nil
}

// Bump writes a new version value, using unix nanos for monotonicity. The new
// version is cached locally to ensure subsequent Current() calls observe it
// immediately.
func (v *IOCCacheVersioner) Bump(ctx context.Context) (string, error) {
	now := v.clock()
	newVersion := strconv.FormatInt(now.UnixNano(), 36)

	if v.redis != nil {
		if err := v.redis.Set(ctx, v.key, []byte(newVersion), 0); err != nil {
			// Even if Redis write fails we still update the local copy so that
			// the caller can continue operating.
			v.mu.Lock()
			v.last = newVersion
			v.lastFetch = now
			v.mu.Unlock()
			return newVersion, err
		}
	}

	v.mu.Lock()
	v.last = newVersion
	v.lastFetch = now
	v.mu.Unlock()

	return newVersion, nil
}

// SetClock allows tests to inject a deterministic clock.
func (v *IOCCacheVersioner) SetClock(fn func() time.Time) {
	if fn == nil {
		return
	}
	v.mu.Lock()
	v.clock = fn
	v.mu.Unlock()
}

var _ IOCVersioner = (*IOCCacheVersioner)(nil)

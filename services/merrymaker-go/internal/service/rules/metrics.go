package rules

// Minimal cache metrics hooks for rules caches.
// Keep interface compact and avoid external deps; callers can adapt to Prometheus or other systems.
// Functions must have ≤3 parameters; use a struct for event details.

type CacheName string

type CacheTier string

type CacheOp string

const (
	CacheSeen  CacheName = "seen"
	CacheIOC   CacheName = "ioc"
	CacheFiles CacheName = "files"
)

const (
	TierLocal CacheTier = "local"
	TierRedis CacheTier = "redis"
	TierRepo  CacheTier = "repo"
)

const (
	OpHit   CacheOp = "hit"
	OpMiss  CacheOp = "miss"
	OpWrite CacheOp = "write"
)

// CacheEvent is a compact event describing a cache metric occurrence.
// Add fields cautiously to keep it small and general.
// Name: which typed cache (seen/ioc/files)
// Tier: which tier (local/redis/repo)
// Op:   hit/miss/write
// Ok:   whether operation succeeded (for writes/lookups)
// Note: Not every combination is used by all caches; unused are fine.

type CacheEvent struct {
	Name CacheName
	Tier CacheTier
	Op   CacheOp
	Ok   bool
}

// CacheMetrics is an optional hook; implementations may aggregate counters.
// A Noop implementation is provided for convenience.

type CacheMetrics interface {
	RecordCacheEvent(e CacheEvent)
}

// NoopCacheMetrics is the default when no metrics are provided.

type NoopCacheMetrics struct{}

func (NoopCacheMetrics) RecordCacheEvent(_ CacheEvent) {}

package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

const defaultAllowlistFetchTimeout = 10 * time.Second

// DomainAllowlistService defines the interface for domain allowlist operations needed by the checker.
type DomainAllowlistService interface {
	GetForScope(ctx context.Context, req model.DomainAllowlistLookupRequest) ([]*model.DomainAllowlist, error)
}

// DomainAllowlistChecker implements AllowlistChecker interface with caching support.
// It checks domains against both global and scoped allowlist patterns with pattern matching.
type DomainAllowlistChecker struct {
	service      DomainAllowlistService
	matcher      *PatternMatcher
	cache        *allowlistCache
	fetchTimeout time.Duration
}

// DomainAllowlistCheckerOptions configures the domain allowlist checker.
type DomainAllowlistCheckerOptions struct {
	Service      DomainAllowlistService
	CacheTTL     time.Duration  // TTL for cached allowlist entries
	CacheSize    int            // Maximum number of cached scope entries
	FetchTimeout *time.Duration // Optional timeout for upstream lookups; nil defaults to 10s, zero disables
}

// NewDomainAllowlistChecker creates a new domain allowlist checker with caching.
func NewDomainAllowlistChecker(opts DomainAllowlistCheckerOptions) *DomainAllowlistChecker {
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 5 * time.Minute // Default cache TTL
	}
	if opts.CacheSize == 0 {
		opts.CacheSize = 1000 // Default cache size
	}

	timeout := defaultAllowlistFetchTimeout
	if opts.FetchTimeout != nil {
		timeout = *opts.FetchTimeout
	}

	return &DomainAllowlistChecker{
		service:      opts.Service,
		matcher:      NewPatternMatcher(),
		cache:        newAllowlistCache(opts.CacheTTL, opts.CacheSize),
		fetchTimeout: timeout,
	}
}

// Allowed checks if a domain is allowed for the given scope.
// It implements the AllowlistChecker interface.
func (c *DomainAllowlistChecker) Allowed(ctx context.Context, scope ScopeKey, domain string) bool {
	if c.service == nil {
		return false // No service configured, deny all
	}

	if ctx == nil {
		ctx = context.Background()
	}

	// Get allowlist patterns for this scope (with caching)
	patterns, err := c.getPatternsForScope(ctx, scope)
	if err != nil {
		// On error, be conservative and deny
		return false
	}

	// Check if domain matches any pattern
	return c.matcher.MatchAny(domain, patterns)
}

// getPatternsForScope retrieves allowlist patterns for a scope, using cache when possible.
func (c *DomainAllowlistChecker) getPatternsForScope(
	ctx context.Context,
	scope ScopeKey,
) ([]model.DomainAllowlist, error) {
	// Check cache first
	if patterns, found := c.cache.get(scope); found {
		return patterns, nil
	}

	// Cache miss, fetch from database with optional timeout derived from caller context
	fetchCtx := ctx
	var cancel context.CancelFunc
	if c.fetchTimeout > 0 {
		fetchCtx, cancel = context.WithTimeout(ctx, c.fetchTimeout)
	}
	if cancel != nil {
		defer cancel()
	}

	req := model.DomainAllowlistLookupRequest{
		Scope:  scope.Scope,
		Domain: "", // Not used for GetForScope
	}

	allowlists, err := c.service.GetForScope(fetchCtx, req)
	if err != nil {
		return nil, fmt.Errorf("fetch allowlist patterns: %w", err)
	}

	// Convert to slice of values for easier handling
	patterns := make([]model.DomainAllowlist, len(allowlists))
	for i, allowlist := range allowlists {
		patterns[i] = *allowlist
	}

	// Cache the result
	c.cache.set(scope, patterns)

	return patterns, nil
}

// InvalidateCache clears the cache for a specific scope or all scopes.
func (c *DomainAllowlistChecker) InvalidateCache(scope *ScopeKey) {
	if scope == nil {
		c.cache.clear()
	} else {
		c.cache.delete(*scope)
	}
}

// allowlistCache provides thread-safe caching for allowlist patterns.
type allowlistCache struct {
	mu          sync.RWMutex
	entries     map[string]cacheEntry
	ttl         time.Duration
	maxSize     int
	lastCleanup time.Time
}

type cacheEntry struct {
	patterns  []model.DomainAllowlist
	expiresAt time.Time
}

// newAllowlistCache creates a new allowlist cache.
func newAllowlistCache(ttl time.Duration, maxSize int) *allowlistCache {
	return &allowlistCache{
		entries:     make(map[string]cacheEntry),
		ttl:         ttl,
		maxSize:     maxSize,
		lastCleanup: time.Now(),
	}
}

// cacheKey generates a cache key for a scope.
func (c *allowlistCache) cacheKey(scope ScopeKey) string {
	return fmt.Sprintf("site:%s:scope:%s", scope.SiteID, scope.Scope)
}

// get retrieves patterns from cache if they exist and haven't expired.
func (c *allowlistCache) get(scope ScopeKey) ([]model.DomainAllowlist, bool) {
	key := c.cacheKey(scope)

	// First pass with read lock
	c.mu.RLock()
	entry, exists := c.entries[key]
	if !exists {
		c.mu.RUnlock()
		return nil, false
	}
	expired := time.Now().After(entry.expiresAt)
	c.mu.RUnlock()

	if expired {
		// Upgrade to write lock to purge expired entry to avoid memory bloat
		c.mu.Lock()
		if e, ok := c.entries[key]; ok && time.Now().After(e.expiresAt) {
			delete(c.entries, key)
		}
		c.mu.Unlock()
		return nil, false
	}

	return entry.patterns, true
}

// set stores patterns in cache with expiration.
func (c *allowlistCache) set(scope ScopeKey, patterns []model.DomainAllowlist) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Periodic cleanup to prevent unbounded growth
	if time.Since(c.lastCleanup) > c.ttl {
		c.cleanupExpiredLocked()
		c.lastCleanup = time.Now()
	}

	// If cache is full, remove oldest entries
	if len(c.entries) >= c.maxSize {
		c.evictOldestLocked()
	}

	key := c.cacheKey(scope)
	c.entries[key] = cacheEntry{
		patterns:  patterns,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// delete removes a specific scope from cache.
func (c *allowlistCache) delete(scope ScopeKey) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := c.cacheKey(scope)
	delete(c.entries, key)
}

// clear removes all entries from cache.
func (c *allowlistCache) clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries = make(map[string]cacheEntry)
}

// cleanupExpiredLocked removes expired entries. Must be called with write lock held.
func (c *allowlistCache) cleanupExpiredLocked() {
	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
		}
	}
}

// evictOldestLocked removes the oldest entry to make room. Must be called with write lock held.
func (c *allowlistCache) evictOldestLocked() {
	if len(c.entries) == 0 {
		return
	}

	// Find the entry with the earliest expiration time
	var oldestKey string
	var oldestTime time.Time
	first := true

	for key, entry := range c.entries {
		if first || entry.expiresAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.expiresAt
			first = false
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
	}
}

// Stats returns cache statistics for monitoring.
func (c *allowlistCache) Stats() AllowlistCacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	now := time.Now()
	expired := 0
	for _, entry := range c.entries {
		if now.After(entry.expiresAt) {
			expired++
		}
	}

	return AllowlistCacheStats{
		TotalEntries:   len(c.entries),
		ExpiredEntries: expired,
		ActiveEntries:  len(c.entries) - expired,
		MaxSize:        c.maxSize,
		TTL:            c.ttl,
	}
}

// AllowlistCacheStats provides cache statistics.
type AllowlistCacheStats struct {
	TotalEntries   int           `json:"total_entries"`
	ExpiredEntries int           `json:"expired_entries"`
	ActiveEntries  int           `json:"active_entries"`
	MaxSize        int           `json:"max_size"`
	TTL            time.Duration `json:"ttl"`
}

// MarshalJSON implements json.Marshaler for AllowlistCacheStats.
func (s AllowlistCacheStats) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		TotalEntries   int `json:"total_entries"`
		ExpiredEntries int `json:"expired_entries"`
		ActiveEntries  int `json:"active_entries"`
		MaxSize        int `json:"max_size"`
		TTLSeconds     int `json:"ttl_seconds"`
	}{
		TotalEntries:   s.TotalEntries,
		ExpiredEntries: s.ExpiredEntries,
		ActiveEntries:  s.ActiveEntries,
		MaxSize:        s.MaxSize,
		TTLSeconds:     int(s.TTL.Seconds()),
	})
}

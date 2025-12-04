// Package core provides the business logic and service layer for the merrymaker job system.
package core

import (
	"context"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// CacheRepository defines the interface for caching operations.
// This follows the hexagonal architecture pattern where the core defines interfaces
// and the data layer provides implementations.
type CacheRepository interface {
	// Set stores a value in the cache with the given key and TTL.
	// If TTL is 0, the key will not expire.
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error

	// Get retrieves a value from the cache by key.
	// Returns nil if the key doesn't exist or has expired.
	Get(ctx context.Context, key string) ([]byte, error)

	// Delete removes a key from the cache.
	// Returns true if the key was deleted, false if it didn't exist.
	Delete(ctx context.Context, key string) (bool, error)

	// Exists checks if a key exists in the cache.
	Exists(ctx context.Context, key string) (bool, error)

	// SetTTL updates the TTL for an existing key.
	// Returns true if the key exists and TTL was updated.
	SetTTL(ctx context.Context, key string, ttl time.Duration) (bool, error)

	// SetIfNotExists atomically sets a key only if it doesn't already exist.
	// Returns true if the key was set, false if it already existed.
	// This is useful for implementing distributed locks and deduplication.
	SetIfNotExists(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)

	// Health checks the health of the cache connection.
	Health(ctx context.Context) error
}

// SourceCacheService provides business logic for caching source content.
// This service orchestrates caching operations for browser job source content.
type SourceCacheService struct {
	cache   CacheRepository
	sources SourceRepository
	secrets SecretRepository
	ttl     time.Duration
}

// SourceCacheConfig holds configuration for source caching.
type SourceCacheConfig struct {
	TTL time.Duration `json:"ttl"`
}

// SourceCacheServiceOptions bundles dependencies for NewSourceCacheService.
type SourceCacheServiceOptions struct {
	Cache   CacheRepository
	Sources SourceRepository
	Secrets SecretRepository
	Config  SourceCacheConfig
}

// DefaultSourceCacheConfig returns a SourceCacheConfig with sensible defaults.
func DefaultSourceCacheConfig() SourceCacheConfig {
	return SourceCacheConfig{
		TTL: 30 * time.Minute, // Cache source content for 30 minutes
	}
}

// NewSourceCacheService creates a new SourceCacheService.
func NewSourceCacheService(opts SourceCacheServiceOptions) *SourceCacheService {
	return &SourceCacheService{
		cache:   opts.Cache,
		sources: opts.Sources,
		secrets: opts.Secrets,
		ttl:     opts.Config.TTL,
	}
}

// CacheSourceContent caches the source content for a given source ID.
// This is called when creating browser jobs to ensure the source content is available.
func (s *SourceCacheService) CacheSourceContent(ctx context.Context, sourceID string) error {
	if sourceID == "" {
		return nil // No source to cache
	}

	source, err := s.sources.GetByID(ctx, sourceID)
	if err != nil {
		return err
	}

	return s.CacheResolvedSourceContent(ctx, source)
}

// CacheResolvedSourceContent caches the provided source without fetching it from the repository again.
// Useful when the caller already has a freshly created or updated source instance on hand.
func (s *SourceCacheService) CacheResolvedSourceContent(ctx context.Context, source *model.Source) error {
	if source == nil || source.ID == "" {
		return nil
	}

	key := s.sourceContentKey(source.ID)
	cached, err := s.cache.Get(ctx, key)
	if err != nil {
		return err
	}

	value := source.Value
	resolved, err := ResolveSecretPlaceholders(ctx, s.secrets, source.Secrets, value)
	if err != nil {
		return err
	}
	value = resolved

	if len(cached) > 0 && string(cached) == value {
		return nil
	}

	// Cache the resolved source value (the Puppeteer script content)
	return s.cache.Set(ctx, key, []byte(value), s.ttl)
}

// GetCachedSourceContent retrieves cached source content by source ID.
// Returns nil if not cached.
func (s *SourceCacheService) GetCachedSourceContent(ctx context.Context, sourceID string) ([]byte, error) {
	if sourceID == "" {
		return nil, nil
	}

	key := s.sourceContentKey(sourceID)
	return s.cache.Get(ctx, key)
}

// InvalidateSourceContent removes cached source content for a source ID.
// This should be called when a source is updated.
func (s *SourceCacheService) InvalidateSourceContent(ctx context.Context, sourceID string) error {
	if sourceID == "" {
		return nil
	}

	key := s.sourceContentKey(sourceID)
	_, err := s.cache.Delete(ctx, key)
	return err
}

// sourceContentKey generates a cache key for source content.
func (s *SourceCacheService) sourceContentKey(sourceID string) string {
	return "source:content:" + sourceID
}

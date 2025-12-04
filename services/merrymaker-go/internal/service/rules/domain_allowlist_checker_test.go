package rules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDomainAllowlistService implements core.DomainAllowlistService for testing.
type mockDomainAllowlistService struct {
	allowlists map[string][]*model.DomainAllowlist // key: siteID:scope
}

type slowDomainAllowlistService struct {
	delay   time.Duration
	lastErr error
}

func newMockDomainAllowlistService() *mockDomainAllowlistService {
	return &mockDomainAllowlistService{
		allowlists: make(map[string][]*model.DomainAllowlist),
	}
}

func (m *mockDomainAllowlistService) addAllowlist(scope string, allowlist *model.DomainAllowlist) {
	m.allowlists[scope] = append(m.allowlists[scope], allowlist)
}

func (m *mockDomainAllowlistService) GetForScope(
	ctx context.Context,
	req model.DomainAllowlistLookupRequest,
) ([]*model.DomainAllowlist, error) {
	var result []*model.DomainAllowlist

	// Add entries for the requested scope
	if entries, exists := m.allowlists[req.Scope]; exists {
		result = append(result, entries...)
	}

	// Add global entries (they apply to all scopes)
	if globalEntries, exists := m.allowlists["global"]; exists {
		result = append(result, globalEntries...)
	}

	return result, nil
}

// Implement other methods as no-ops for testing.
func (m *mockDomainAllowlistService) Create(
	ctx context.Context,
	req *model.CreateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockDomainAllowlistService) GetByID(
	ctx context.Context,
	id string,
) (*model.DomainAllowlist, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockDomainAllowlistService) Update(
	ctx context.Context,
	id string,
	req model.UpdateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockDomainAllowlistService) Delete(ctx context.Context, id string) error {
	return nil
}

func (m *mockDomainAllowlistService) List(
	ctx context.Context,
	opts model.DomainAllowlistListOptions,
) ([]*model.DomainAllowlist, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockDomainAllowlistService) Stats(
	ctx context.Context,
	siteID *string,
) (*model.DomainAllowlistStats, error) {
	return nil, errors.New("not implemented in mock")
}

func (s *slowDomainAllowlistService) GetForScope(
	ctx context.Context,
	_ model.DomainAllowlistLookupRequest,
) ([]*model.DomainAllowlist, error) {
	select {
	case <-ctx.Done():
		s.lastErr = ctx.Err()
		return nil, s.lastErr
	case <-time.After(s.delay):
		s.lastErr = nil
		return []*model.DomainAllowlist{}, nil
	}
}

func TestDomainAllowlistChecker_Allowed(t *testing.T) {
	mockService := newMockDomainAllowlistService()

	// Add some test allowlist entries
	mockService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "allowed.com",
		PatternType: model.PatternTypeExact,
		Enabled:     true,
	})

	mockService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "*.example.com",
		PatternType: model.PatternTypeWildcard,
		Enabled:     true,
	})

	// Add global allowlist entry
	mockService.addAllowlist("global", &model.DomainAllowlist{
		Pattern:     "global.com",
		PatternType: model.PatternTypeExact,
		Enabled:     true,
	})

	// Add disabled entry
	mockService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "disabled.com",
		PatternType: model.PatternTypeExact,
		Enabled:     false,
	})

	checker := NewDomainAllowlistChecker(DomainAllowlistCheckerOptions{
		Service:   mockService,
		CacheTTL:  1 * time.Minute,
		CacheSize: 100,
	})

	scope := ScopeKey{SiteID: "site1", Scope: "default"}
	ctx := context.Background()

	tests := []struct {
		name     string
		domain   string
		expected bool
	}{
		{
			name:     "exact match allowed",
			domain:   "allowed.com",
			expected: true,
		},
		{
			name:     "wildcard match allowed",
			domain:   "sub.example.com",
			expected: true,
		},
		{
			name:     "global allowlist match",
			domain:   "global.com",
			expected: true,
		},
		{
			name:     "disabled entry not allowed",
			domain:   "disabled.com",
			expected: false,
		},
		{
			name:     "no match not allowed",
			domain:   "other.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := checker.Allowed(ctx, scope, tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDomainAllowlistChecker_Caching(t *testing.T) {
	mockService := newMockDomainAllowlistService()

	mockService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "cached.com",
		PatternType: model.PatternTypeExact,
		Enabled:     true,
	})

	checker := NewDomainAllowlistChecker(DomainAllowlistCheckerOptions{
		Service:   mockService,
		CacheTTL:  100 * time.Millisecond, // Short TTL for testing
		CacheSize: 100,
	})

	scope := ScopeKey{SiteID: "site1", Scope: "default"}
	ctx := context.Background()

	// First call should populate cache
	result1 := checker.Allowed(ctx, scope, "cached.com")
	assert.True(t, result1)

	// Second call should use cache (even if we modify the mock)
	result2 := checker.Allowed(ctx, scope, "cached.com")
	assert.True(t, result2)

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// This call should fetch fresh data
	result3 := checker.Allowed(ctx, scope, "cached.com")
	assert.True(t, result3)
}

func TestDomainAllowlistChecker_AllowedHonorsContextCancellation(t *testing.T) {
	slowService := &slowDomainAllowlistService{delay: 100 * time.Millisecond}
	timeout := 10 * time.Millisecond
	checker := NewDomainAllowlistChecker(DomainAllowlistCheckerOptions{
		Service:      slowService,
		CacheTTL:     time.Minute,
		CacheSize:    100,
		FetchTimeout: &timeout,
	})

	scope := ScopeKey{SiteID: "site1", Scope: "default"}

	start := time.Now()
	result := checker.Allowed(context.Background(), scope, "any.com")
	elapsed := time.Since(start)

	assert.False(t, result, "timeout should deny allowlist evaluation")
	assert.Less(t, elapsed, slowService.delay, "lookup should respect configured timeout")
	require.Error(t, slowService.lastErr)
	assert.ErrorIs(t, slowService.lastErr, context.DeadlineExceeded)
}

func TestDomainAllowlistChecker_InvalidateCache(t *testing.T) {
	mockService := newMockDomainAllowlistService()

	mockService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "test.com",
		PatternType: model.PatternTypeExact,
		Enabled:     true,
	})

	checker := NewDomainAllowlistChecker(DomainAllowlistCheckerOptions{
		Service:   mockService,
		CacheTTL:  1 * time.Hour, // Long TTL
		CacheSize: 100,
	})

	scope := ScopeKey{SiteID: "site1", Scope: "default"}
	ctx := context.Background()

	// Populate cache
	result1 := checker.Allowed(ctx, scope, "test.com")
	assert.True(t, result1)

	// Invalidate specific scope
	checker.InvalidateCache(&scope)

	// Should still work (will fetch fresh data)
	result2 := checker.Allowed(ctx, scope, "test.com")
	assert.True(t, result2)

	// Invalidate all cache
	checker.InvalidateCache(nil)

	// Should still work
	result3 := checker.Allowed(ctx, scope, "test.com")
	assert.True(t, result3)
}

func TestDomainAllowlistChecker_NoService(t *testing.T) {
	checker := NewDomainAllowlistChecker(DomainAllowlistCheckerOptions{
		Service:   nil, // No service
		CacheTTL:  1 * time.Minute,
		CacheSize: 100,
	})

	scope := ScopeKey{SiteID: "site1", Scope: "default"}

	// Should deny all when no service is configured
	result := checker.Allowed(context.Background(), scope, "any.com")
	assert.False(t, result)
}

func TestAllowlistCache_Stats(t *testing.T) {
	cache := newAllowlistCache(1*time.Minute, 100)

	// Initially empty
	stats := cache.Stats()
	assert.Equal(t, 0, stats.TotalEntries)
	assert.Equal(t, 0, stats.ActiveEntries)
	assert.Equal(t, 0, stats.ExpiredEntries)
	assert.Equal(t, 100, stats.MaxSize)
	assert.Equal(t, 1*time.Minute, stats.TTL)

	// Add some entries
	scope1 := ScopeKey{SiteID: "site1", Scope: "default"}
	scope2 := ScopeKey{SiteID: "site2", Scope: "default"}

	cache.set(scope1, []model.DomainAllowlist{})
	cache.set(scope2, []model.DomainAllowlist{})

	stats = cache.Stats()
	assert.Equal(t, 2, stats.TotalEntries)
	assert.Equal(t, 2, stats.ActiveEntries)
	assert.Equal(t, 0, stats.ExpiredEntries)
}

func TestAllowlistCache_Eviction(t *testing.T) {
	cache := newAllowlistCache(1*time.Hour, 2) // Small cache size

	scope1 := ScopeKey{SiteID: "site1", Scope: "default"}
	scope2 := ScopeKey{SiteID: "site2", Scope: "default"}
	scope3 := ScopeKey{SiteID: "site3", Scope: "default"}

	// Fill cache to capacity
	cache.set(scope1, []model.DomainAllowlist{})
	cache.set(scope2, []model.DomainAllowlist{})

	stats := cache.Stats()
	assert.Equal(t, 2, stats.TotalEntries)

	// Adding third entry should evict oldest
	cache.set(scope3, []model.DomainAllowlist{})

	stats = cache.Stats()
	assert.Equal(t, 2, stats.TotalEntries) // Still at max capacity

	// First entry should be evicted
	_, found := cache.get(scope1)
	assert.False(t, found)

	// Other entries should still be there
	_, found = cache.get(scope2)
	assert.True(t, found)
	_, found = cache.get(scope3)
	assert.True(t, found)
}

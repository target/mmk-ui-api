package rules

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockAlertRepo implements core.AlertRepository for testing.
type mockAlertRepo struct {
	alerts []*model.Alert
}

func (m *mockAlertRepo) Create(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error) {
	alert := &model.Alert{
		ID:             "test-alert-" + string(rune(len(m.alerts))),
		SiteID:         req.SiteID,
		RuleType:       req.RuleType,
		Severity:       req.Severity,
		Title:          req.Title,
		Description:    req.Description,
		FiredAt:        time.Now(),
		CreatedAt:      time.Now(),
		DeliveryStatus: model.AlertDeliveryStatusPending,
	}
	m.alerts = append(m.alerts, alert)
	return alert, nil
}

func (m *mockAlertRepo) GetByID(ctx context.Context, id string) (*model.Alert, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockAlertRepo) List(ctx context.Context, opts *model.AlertListOptions) ([]*model.Alert, error) {
	return m.alerts, nil
}

func (m *mockAlertRepo) ListWithSiteNames(
	ctx context.Context,
	opts *model.AlertListOptions,
) ([]*model.AlertWithSiteName, error) {
	// Convert alerts to AlertWithSiteName for testing
	alertsWithSiteNames := make([]*model.AlertWithSiteName, len(m.alerts))
	for i, alert := range m.alerts {
		alertsWithSiteNames[i] = &model.AlertWithSiteName{
			Alert:    *alert,
			SiteName: "Test Site", // Mock site name
		}
	}
	return alertsWithSiteNames, nil
}

func (m *mockAlertRepo) Delete(ctx context.Context, id string) (bool, error) {
	return false, errors.New("not implemented in mock")
}

func (m *mockAlertRepo) Stats(ctx context.Context, siteID *string) (*model.AlertStats, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockAlertRepo) Resolve(ctx context.Context, params core.ResolveAlertParams) (*model.Alert, error) {
	return nil, errors.New("not implemented in mock")
}

func (m *mockAlertRepo) UpdateDeliveryStatus(
	ctx context.Context,
	params core.UpdateAlertDeliveryStatusParams,
) (*model.Alert, error) {
	return nil, errors.New("not implemented in mock")
}

func TestUnknownDomainEvaluator_WithAllowlist_Integration(t *testing.T) {
	ctx := context.Background()

	// Create mock services
	mockAllowlistService := newMockDomainAllowlistService()
	mockAlertRepo := &mockAlertRepo{}

	// Set up allowlist entries
	mockAllowlistService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "allowed.com",
		PatternType: model.PatternTypeExact,
		Enabled:     true,
		Priority:    100,
	})

	mockAllowlistService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "*.trusted.com",
		PatternType: model.PatternTypeWildcard,
		Enabled:     true,
		Priority:    200,
	})

	// Global allowlist entry
	mockAllowlistService.addAllowlist("global", &model.DomainAllowlist{
		Pattern:     "global-allowed.com",
		PatternType: model.PatternTypeExact,
		Enabled:     true,
		Priority:    300,
	})

	// Create allowlist checker
	allowlistChecker := NewDomainAllowlistChecker(DomainAllowlistCheckerOptions{
		Service:   mockAllowlistService,
		CacheTTL:  5 * time.Minute,
		CacheSize: 1000,
	})

	// Create alert service (using core for rules package to avoid import cycle)
	alertService := &core.AlertService{
		Repo: mockAlertRepo,
	}

	// Create caches (minimal setup for testing)
	seenLocal := NewLocalLRU(DefaultLocalLRUConfig())
	caches := Caches{
		Seen: NewSeenDomainsCache(SeenDomainsCacheDeps{
			Local: seenLocal,
			TTL:   DefaultCacheTTL(),
		}),
	}

	// Create unknown domain evaluator with allowlist
	evaluator := &UnknownDomainEvaluator{
		Caches:    caches,
		Alerter:   alertService,
		Allowlist: allowlistChecker,
		AlertTTL:  24 * time.Hour,
	}

	scope := ScopeKey{SiteID: "site1", Scope: "default"}

	tests := []struct {
		name        string
		domain      string
		expectAlert bool
		description string
	}{
		{
			name:        "exact allowlist match - no alert",
			domain:      "allowed.com",
			expectAlert: false,
			description: "Domain is in exact allowlist, should not generate alert",
		},
		{
			name:        "wildcard allowlist match - no alert",
			domain:      "sub.trusted.com",
			expectAlert: false,
			description: "Domain matches wildcard allowlist, should not generate alert",
		},
		{
			name:        "global allowlist match - no alert",
			domain:      "global-allowed.com",
			expectAlert: false,
			description: "Domain is in global allowlist, should not generate alert",
		},
		{
			name:        "unknown domain - alert generated",
			domain:      "unknown.com",
			expectAlert: true,
			description: "Domain is not in allowlist, should generate alert",
		},
		{
			name:        "another unknown domain - alert generated",
			domain:      "suspicious.net",
			expectAlert: true,
			description: "Another unknown domain, should generate alert",
		},
	}

	initialAlertCount := len(mockAlertRepo.alerts)
	alertsExpected := 0

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := UnknownDomainRequest{
				Scope:  scope,
				Domain: tt.domain,
				SiteID: scope.SiteID,
			}

			alerted, err := evaluator.Evaluate(ctx, req)
			require.NoError(t, err, "Evaluate should not return error")

			assert.Equal(t, tt.expectAlert, alerted, tt.description)

			// Check that alert was actually created if expected
			if tt.expectAlert {
				alertsExpected++
				expectedAlertCount := initialAlertCount + alertsExpected
				assert.Len(t, mockAlertRepo.alerts, expectedAlertCount, "Alert should be created")

				// Check the latest alert
				if len(mockAlertRepo.alerts) > 0 {
					latestAlert := mockAlertRepo.alerts[len(mockAlertRepo.alerts)-1]
					assert.Equal(t, scope.SiteID, latestAlert.SiteID)
					assert.Equal(t, "unknown_domain", latestAlert.RuleType)
					assert.Equal(t, "Unknown domain observed", latestAlert.Title)
					assert.Contains(t, latestAlert.Description, tt.domain)
				}
			}
		})
	}
}

func TestUnknownDomainEvaluator_MultiplePatternTypes_Integration(t *testing.T) {
	ctx := context.Background()

	// Create mock services
	mockAllowlistService := newMockDomainAllowlistService()
	mockAlertRepo := &mockAlertRepo{}

	// Set up different pattern types
	mockAllowlistService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "exact.com",
		PatternType: model.PatternTypeExact,
		Enabled:     true,
		Priority:    100,
	})

	mockAllowlistService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "*.wildcard.com",
		PatternType: model.PatternTypeWildcard,
		Enabled:     true,
		Priority:    200,
	})

	mockAllowlistService.addAllowlist("default", &model.DomainAllowlist{
		Pattern:     "etld.com",
		PatternType: model.PatternTypeETLDPlusOne,
		Enabled:     true,
		Priority:    300,
	})

	// Create allowlist checker
	allowlistChecker := NewDomainAllowlistChecker(DomainAllowlistCheckerOptions{
		Service:   mockAllowlistService,
		CacheTTL:  5 * time.Minute,
		CacheSize: 1000,
	})

	// Create alert service
	alertService := &core.AlertService{
		Repo: mockAlertRepo,
	}

	// Create caches
	seenLocal := NewLocalLRU(DefaultLocalLRUConfig())
	caches := Caches{
		Seen: NewSeenDomainsCache(SeenDomainsCacheDeps{
			Local: seenLocal,
			TTL:   DefaultCacheTTL(),
		}),
	}

	// Create unknown domain evaluator
	evaluator := &UnknownDomainEvaluator{
		Caches:    caches,
		Alerter:   alertService,
		Allowlist: allowlistChecker,
		AlertTTL:  24 * time.Hour,
	}

	scope := ScopeKey{SiteID: "site1", Scope: "default"}

	tests := []struct {
		name        string
		domain      string
		expectAlert bool
	}{
		{
			name:        "exact pattern match",
			domain:      "exact.com",
			expectAlert: false,
		},
		{
			name:        "wildcard pattern match",
			domain:      "sub.wildcard.com",
			expectAlert: false,
		},
		{
			name:        "etld+1 pattern match",
			domain:      "sub.etld.com",
			expectAlert: false,
		},
		{
			name:        "no pattern match",
			domain:      "unknown.com",
			expectAlert: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := UnknownDomainRequest{
				Scope:  scope,
				Domain: tt.domain,
				SiteID: scope.SiteID,
			}

			alerted, err := evaluator.Evaluate(ctx, req)
			require.NoError(t, err)
			assert.Equal(t, tt.expectAlert, alerted)
		})
	}
}

package rules

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeAlertRepo struct{ created []*model.CreateAlertRequest }

var errStub = errors.New("stub")

func (f *fakeAlertRepo) Create(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error) {
	f.created = append(f.created, req)
	return &model.Alert{
		ID:             "a1",
		SiteID:         req.SiteID,
		RuleType:       req.RuleType,
		Severity:       req.Severity,
		Title:          req.Title,
		Description:    req.Description,
		DeliveryStatus: req.DeliveryStatus,
	}, nil
}

func (f *fakeAlertRepo) GetByID(ctx context.Context, id string) (*model.Alert, error) {
	return nil, errStub
}

func (f *fakeAlertRepo) List(ctx context.Context, opts *model.AlertListOptions) ([]*model.Alert, error) {
	return nil, nil
}

func (f *fakeAlertRepo) ListWithSiteNames(
	ctx context.Context,
	opts *model.AlertListOptions,
) ([]*model.AlertWithSiteName, error) {
	return nil, nil
}
func (f *fakeAlertRepo) Delete(ctx context.Context, id string) (bool, error) { return false, nil }
func (f *fakeAlertRepo) Stats(ctx context.Context, siteID *string) (*model.AlertStats, error) {
	return nil, errStub
}

func (f *fakeAlertRepo) Resolve(ctx context.Context, params core.ResolveAlertParams) (*model.Alert, error) {
	return nil, errStub
}

func (f *fakeAlertRepo) UpdateDeliveryStatus(
	ctx context.Context,
	params core.UpdateAlertDeliveryStatusParams,
) (*model.Alert, error) {
	return nil, errStub
}

func TestUnknownDomainEvaluator_BasicFlow(t *testing.T) {
	ctx := context.Background()
	// Build caches: local-only (no redis, no repo)
	seenLocal := NewLocalLRU(DefaultLocalLRUConfig())
	iocLocal := NewLocalLRU(DefaultLocalLRUConfig())
	filesLocal := NewLocalLRU(DefaultLocalLRUConfig())
	c := Caches{
		Seen: NewSeenDomainsCache(
			SeenDomainsCacheDeps{Local: seenLocal, Redis: nil, Repo: nil, TTL: DefaultCacheTTL()},
		),
		IOCs: NewIOCCache(IOCCacheDeps{Local: iocLocal, Redis: nil, Repo: nil, TTL: DefaultCacheTTL()}),
		Files: NewProcessedFilesCache(
			ProcessedFilesCacheDeps{Local: filesLocal, Redis: nil, Repo: nil, TTL: DefaultCacheTTL()},
		),
		AlertOnce: NewAlertOnceCache(NewLocalLRU(DefaultLocalLRUConfig()), nil),
	}

	// Wire alert service via fake repo
	fa := &fakeAlertRepo{}
	alerter := &core.AlertService{
		Repo: fa,
	}
	eval := &UnknownDomainEvaluator{Caches: c, Alerter: alerter, AlertTTL: time.Minute}
	req := UnknownDomainRequest{
		Scope:  ScopeKey{SiteID: "site-1", Scope: "default"},
		Domain: "Example.COM",
		SiteID: "site-1",
	}

	// First time -> alert
	alerted, err := eval.Evaluate(ctx, req)
	require.NoError(t, err)
	assert.True(t, alerted)
	assert.Len(t, fa.created, 1)
	assert.Equal(t, "unknown_domain", fa.created[0].RuleType)
	assert.Equal(t, "medium", fa.created[0].Severity)

	// Second time -> no alert (seen)
	alerted2, err := eval.Evaluate(ctx, req)
	require.NoError(t, err)
	assert.False(t, alerted2)
	assert.Len(t, fa.created, 1)
}

func TestUnknownDomainEvaluator_AlertContextIncludesAttribution(t *testing.T) {
	ctx := context.Background()
	seenLocal := NewLocalLRU(DefaultLocalLRUConfig())
	c := Caches{
		Seen: NewSeenDomainsCache(
			SeenDomainsCacheDeps{Local: seenLocal, Redis: nil, Repo: nil, TTL: DefaultCacheTTL()},
		),
	}

	fa := &fakeAlertRepo{}
	alerter := &core.AlertService{Repo: fa}
	eval := &UnknownDomainEvaluator{Caches: c, Alerter: alerter}

	req := UnknownDomainRequest{
		Scope:      ScopeKey{SiteID: "site-1", Scope: "default"},
		Domain:     "Example.COM",
		SiteID:     "site-1",
		JobID:      "job-1",
		RequestURL: "https://example.com/api",
		PageURL:    "https://example.com/page",
		Referrer:   "https://example.com/",
		UserAgent:  "UA",
		EventID:    "evt-1",
	}

	alerted, err := eval.Evaluate(ctx, req)
	require.NoError(t, err)
	require.True(t, alerted)
	require.Len(t, fa.created, 1)

	var ctxMap map[string]any
	require.NoError(t, json.Unmarshal(fa.created[0].EventContext, &ctxMap))
	assert.Equal(t, "job-1", ctxMap["job_id"])
	assert.Equal(t, "evt-1", ctxMap["event_id"])
	assert.Equal(t, "https://example.com/api", ctxMap["request_url"])
	assert.Equal(t, "https://example.com/page", ctxMap["page_url"])
	assert.Equal(t, "https://example.com/", ctxMap["referrer"])
	assert.Equal(t, "UA", ctxMap["user_agent"])
}

type staticAllowlist struct{ set map[string]bool }

func (s staticAllowlist) Allowed(_ context.Context, _ ScopeKey, domain string) bool {
	return s.set[domain]
}

func TestUnknownDomainEvaluator_AllowlistPreemption(t *testing.T) {
	ctx := context.Background()
	seenLocal := NewLocalLRU(DefaultLocalLRUConfig())
	c := Caches{Seen: NewSeenDomainsCache(SeenDomainsCacheDeps{Local: seenLocal, TTL: DefaultCacheTTL()})}
	fa := &fakeAlertRepo{}
	alerter := &core.AlertService{
		Repo: fa,
	}
	al := staticAllowlist{set: map[string]bool{"allowed.com": true}}
	eval := &UnknownDomainEvaluator{Caches: c, Alerter: alerter, Allowlist: al}
	req := UnknownDomainRequest{Scope: ScopeKey{SiteID: "s", Scope: "default"}, Domain: "allowed.com", SiteID: "s"}
	alerted, err := eval.Evaluate(ctx, req)
	require.NoError(t, err)
	assert.False(t, alerted)
	assert.Empty(t, fa.created)
}

func TestUnknownDomainEvaluator_Preview(t *testing.T) {
	ctx := context.Background()
	seenLocal := NewLocalLRU(DefaultLocalLRUConfig())
	c := Caches{
		Seen:      NewSeenDomainsCache(SeenDomainsCacheDeps{Local: seenLocal, TTL: DefaultCacheTTL()}),
		AlertOnce: NewAlertOnceCache(NewLocalLRU(DefaultLocalLRUConfig()), nil),
	}
	eval := &UnknownDomainEvaluator{Caches: c, AlertTTL: time.Minute}
	scope := ScopeKey{SiteID: "site-1", Scope: "default"}
	req := UnknownDomainRequest{Scope: scope, Domain: "preview.test", SiteID: scope.SiteID}

	wouldAlert, err := eval.Preview(ctx, req)
	require.NoError(t, err)
	assert.True(t, wouldAlert, "first preview should indicate alert")

	seen, err := c.Seen.Exists(ctx, SeenKey{Scope: scope, Domain: "preview.test"})
	require.NoError(t, err)
	assert.True(t, seen, "preview should record domain as seen")

	// Second preview should not alert because domain is now recorded.
	wouldAlertSecond, err := eval.Preview(ctx, req)
	require.NoError(t, err)
	assert.False(t, wouldAlertSecond, "subsequent preview should not alert")

	// Allowlist should short-circuit and still record domain.
	al := staticAllowlist{set: map[string]bool{"allowed.test": true}}
	eval.Allowlist = al
	allowReq := UnknownDomainRequest{Scope: scope, Domain: "allowed.test", SiteID: scope.SiteID}
	wouldAlertAllow, err := eval.Preview(ctx, allowReq)
	require.NoError(t, err)
	assert.False(t, wouldAlertAllow)
	allowSeen, err := c.Seen.Exists(ctx, SeenKey{Scope: scope, Domain: "allowed.test"})
	require.NoError(t, err)
	assert.True(t, allowSeen, "preview should record allowlisted domains as seen")
}

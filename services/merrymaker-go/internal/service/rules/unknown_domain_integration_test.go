package rules

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// helper to create a minimal site with a source.
func createSite(t *testing.T, db *sql.DB, name string) *model.Site {
	t.Helper()
	ctx := context.Background()
	sr := data.NewSourceRepo(db)
	src, err := sr.Create(ctx, &model.CreateSourceRequest{Name: name + "-src", Value: "console.log('hi')", Test: true})
	require.NoError(t, err)
	siteRepo := data.NewSiteRepo(db)
	scope := "default"
	s := &model.CreateSiteRequest{Name: name, RunEveryMinutes: 5, SourceID: src.ID, Scope: &scope}
	site, err := siteRepo.Create(ctx, s)
	require.NoError(t, err)
	return site
}

// build evaluator with full caches (local+redis+postgres) and alert repo.
func buildUnknownEvaluator(
	t *testing.T,
	redisRepo core.CacheRepository,
	seenRepo core.SeenDomainRepository,
	alertRepo core.AlertRepository,
) *UnknownDomainEvaluator {
	t.Helper()
	caches := BuildCaches(CachesOptions{TTL: DefaultCacheTTL(), Redis: redisRepo, SeenRepo: seenRepo})
	alerter := &core.AlertService{
		Repo: alertRepo,
	}
	return &UnknownDomainEvaluator{Caches: caches, Alerter: alerter, AlertTTL: time.Minute}
}

// redis key helper (mirrors seenKeyRedis).
func seenRedisKey(siteID, scope, domain string) string {
	return "rules:seen:site:" + siteID + ":scope:" + scope + ":domain:" + strings.ToLower(strings.TrimSpace(domain))
}

// assert alert exists for site and rule type.
func listAlerts(t *testing.T, db *sql.DB, siteID string) []*model.Alert {
	t.Helper()
	ctx := context.Background()
	aRepo := data.NewAlertRepo(db)
	opts := &model.AlertListOptions{SiteID: &siteID}
	alerts, err := aRepo.List(ctx, opts)
	require.NoError(t, err)
	return alerts
}

func TestUnknownDomain_Integration_FirstTimeCreatesAlertAndPersists(t *testing.T) {
	testutil.WithAutoDB(t, func(db *sql.DB) {
		redisClient := testutil.SetupTestRedis(t)
		redisRepo := data.NewRedisCacheRepo(redisClient)
		ctx := context.Background()

		// Arrange: site, repos, evaluator
		site := createSite(t, db, "site-unknown-int-1")
		seenRepo := data.NewSeenDomainRepo(db)
		alertRepo := data.NewAlertRepo(db)
		eval := buildUnknownEvaluator(t, redisRepo, seenRepo, alertRepo)

		// Act: evaluate first time
		alerted, err := eval.Evaluate(
			ctx,
			UnknownDomainRequest{
				Scope:  ScopeKey{SiteID: site.ID, Scope: "default"},
				Domain: "new-domain.test",
				SiteID: site.ID,
			},
		)
		require.NoError(t, err)
		assert.True(t, alerted)

		// Assert: alert persisted
		alerts := listAlerts(t, db, site.ID)
		require.NotEmpty(t, alerts)
		assert.Equal(t, string(model.AlertRuleTypeUnknownDomain), alerts[0].RuleType)

		// Assert: seen_domains row exists
		sd, err := seenRepo.Lookup(
			ctx,
			model.SeenDomainLookupRequest{SiteID: site.ID, Domain: "new-domain.test", Scope: "default"},
		)
		require.NoError(t, err)
		require.NotNil(t, sd)
		assert.Equal(t, 1, sd.HitCount)

		// Assert: redis key exists
		exists, err := redisRepo.Exists(ctx, seenRedisKey(site.ID, "default", "new-domain.test"))
		require.NoError(t, err)
		assert.True(t, exists)
	})
}

func TestUnknownDomain_Integration_NoAlertWhenSeenInRedis(t *testing.T) {
	testutil.WithAutoDB(t, func(db *sql.DB) {
		redisClient := testutil.SetupTestRedis(t)
		redisRepo := data.NewRedisCacheRepo(redisClient)
		ctx := context.Background()

		site := createSite(t, db, "site-unknown-int-2")
		seenRepo := data.NewSeenDomainRepo(db)
		alertRepo := data.NewAlertRepo(db)
		eval := buildUnknownEvaluator(t, redisRepo, seenRepo, alertRepo)

		// Prepopulate redis seen key
		key := seenRedisKey(site.ID, "default", "cached-domain.test")
		require.NoError(t, redisRepo.Set(ctx, key, []byte("1"), DefaultCacheTTL().SeenDomainsRedis))

		alerted, err := eval.Evaluate(
			ctx,
			UnknownDomainRequest{
				Scope:  ScopeKey{SiteID: site.ID, Scope: "default"},
				Domain: "cached-domain.test",
				SiteID: site.ID,
			},
		)
		require.NoError(t, err)
		assert.False(t, alerted)

		// No alert created
		alerts := listAlerts(t, db, site.ID)
		assert.Empty(t, alerts)

		// No DB seen row created by evaluator (since already seen)
		sd, err := seenRepo.Lookup(
			ctx,
			model.SeenDomainLookupRequest{SiteID: site.ID, Domain: "cached-domain.test", Scope: "default"},
		)
		require.NoError(t, err)
		assert.Nil(t, sd)
	})
}

func TestUnknownDomain_Integration_NoAlertWhenSeenInDBAndCachesWarm(t *testing.T) {
	testutil.WithAutoDB(t, func(db *sql.DB) {
		redisClient := testutil.SetupTestRedis(t)
		redisRepo := data.NewRedisCacheRepo(redisClient)
		ctx := context.Background()

		site := createSite(t, db, "site-unknown-int-3")
		seenRepo := data.NewSeenDomainRepo(db)
		alertRepo := data.NewAlertRepo(db)
		eval := buildUnknownEvaluator(t, redisRepo, seenRepo, alertRepo)

		// Pre-insert seen in DB only
		_, err := seenRepo.RecordSeen(
			ctx,
			model.RecordDomainSeenRequest{SiteID: site.ID, Domain: "db-seen.test", Scope: "default"},
		)
		require.NoError(t, err)

		// Ensure redis does not have it yet
		exists, err := redisRepo.Exists(ctx, seenRedisKey(site.ID, "default", "db-seen.test"))
		require.NoError(t, err)
		assert.False(t, exists)

		// Evaluate -> should detect seen via repo tier and not alert
		alerted, err := eval.Evaluate(
			ctx,
			UnknownDomainRequest{
				Scope:  ScopeKey{SiteID: site.ID, Scope: "default"},
				Domain: "db-seen.test",
				SiteID: site.ID,
			},
		)
		require.NoError(t, err)
		assert.False(t, alerted)

		// Redis should be warmed now
		exists2, err := redisRepo.Exists(ctx, seenRedisKey(site.ID, "default", "db-seen.test"))
		require.NoError(t, err)
		assert.True(t, exists2)

		// Alerts remain empty
		alerts := listAlerts(t, db, site.ID)
		assert.Empty(t, alerts)
	})
}

func TestUnknownDomain_Integration_ScopeIsolation(t *testing.T) {
	testutil.WithAutoDB(t, func(db *sql.DB) {
		redisClient := testutil.SetupTestRedis(t)
		redisRepo := data.NewRedisCacheRepo(redisClient)
		ctx := context.Background()

		site := createSite(t, db, "site-unknown-int-4")
		seenRepo := data.NewSeenDomainRepo(db)
		alertRepo := data.NewAlertRepo(db)
		eval := buildUnknownEvaluator(t, redisRepo, seenRepo, alertRepo)

		// First scope "default"
		alerted1, err := eval.Evaluate(
			ctx,
			UnknownDomainRequest{
				Scope:  ScopeKey{SiteID: site.ID, Scope: "default"},
				Domain: "scopey.test",
				SiteID: site.ID,
			},
		)
		require.NoError(t, err)
		assert.True(t, alerted1)

		// Different scope should alert again
		alerted2, err := eval.Evaluate(
			ctx,
			UnknownDomainRequest{
				Scope:  ScopeKey{SiteID: site.ID, Scope: "blue"},
				Domain: "scopey.test",
				SiteID: site.ID,
			},
		)
		require.NoError(t, err)
		assert.True(t, alerted2)

		// Verify two seen rows
		a, err := seenRepo.Lookup(
			ctx,
			model.SeenDomainLookupRequest{SiteID: site.ID, Domain: "scopey.test", Scope: "default"},
		)
		require.NoError(t, err)
		require.NotNil(t, a)
		b, err := seenRepo.Lookup(
			ctx,
			model.SeenDomainLookupRequest{SiteID: site.ID, Domain: "scopey.test", Scope: "blue"},
		)
		require.NoError(t, err)
		require.NotNil(t, b)
		assert.NotEqual(t, a.ID, b.ID)

		// Verify distinct redis keys exist
		ex1, _ := redisRepo.Exists(ctx, seenRedisKey(site.ID, "default", "scopey.test"))
		ex2, _ := redisRepo.Exists(ctx, seenRedisKey(site.ID, "blue", "scopey.test"))
		assert.True(t, ex1)
		assert.True(t, ex2)
	})
}

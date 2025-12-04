package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func TestAlertRepo_Create(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	db := testutil.SetupTestDB(t)
	repo := NewAlertRepo(db)
	site := createTestSite(t, db)

	t.Run("successful creation", func(t *testing.T) {
		eventContext := json.RawMessage(`{"domain": "example.com", "url": "https://example.com"}`)
		metadata := json.RawMessage(`{"rule_config": {"pattern": "*.example.com"}}`)

		req := &model.CreateAlertRequest{
			SiteID:       site.ID,
			RuleType:     "unknown_domain",
			Severity:     "medium",
			Title:        "Unknown domain detected",
			Description:  "A new domain was observed that hasn't been seen before",
			EventContext: eventContext,
			Metadata:     metadata,
		}

		alert, err := repo.Create(context.Background(), req)
		require.NoError(t, err)
		require.NotNil(t, alert)

		assert.NotEmpty(t, alert.ID)
		assert.Equal(t, site.ID, alert.SiteID)
		assert.Nil(t, alert.RuleID)
		assert.Equal(t, "unknown_domain", alert.RuleType)
		assert.Equal(t, "medium", alert.Severity)
		assert.Equal(t, "Unknown domain detected", alert.Title)
		assert.Equal(t, "A new domain was observed that hasn't been seen before", alert.Description)
		assert.JSONEq(t, string(eventContext), string(alert.EventContext))
		assert.JSONEq(t, string(metadata), string(alert.Metadata))
		assert.Nil(t, alert.ResolvedAt)
		assert.NotZero(t, alert.FiredAt)
		assert.NotZero(t, alert.CreatedAt)
		assert.Equal(t, model.AlertDeliveryStatusPending, alert.DeliveryStatus)
	})

	t.Run("validation error", func(t *testing.T) {
		req := &model.CreateAlertRequest{
			SiteID:      site.ID,
			RuleType:    "invalid_rule_type",
			Severity:    "medium",
			Title:       "",
			Description: "Test description",
		}

		alert, err := repo.Create(context.Background(), req)
		require.Error(t, err)
		assert.Nil(t, alert)
		assert.Contains(t, err.Error(), "invalid rule_type")
	})

	t.Run("foreign key violation", func(t *testing.T) {
		req := &model.CreateAlertRequest{
			SiteID:      "550e8400-e29b-41d4-a716-446655440000", // Valid UUID format but non-existent
			RuleType:    "unknown_domain",
			Severity:    "medium",
			Title:       "Test Alert",
			Description: "Test description",
		}

		alert, err := repo.Create(context.Background(), req)
		require.Error(t, err)
		assert.Nil(t, alert)
		assert.Contains(t, err.Error(), "site not found")
	})
}

func TestAlertRepo_GetByID(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	db := testutil.SetupTestDB(t)
	repo := NewAlertRepo(db)
	site := createTestSite(t, db)

	// Create a test alert
	req := &model.CreateAlertRequest{
		SiteID:      site.ID,
		RuleType:    "ioc_domain",
		Severity:    "critical",
		Title:       "IOC domain detected",
		Description: "A known malicious domain was accessed",
	}

	created, err := repo.Create(context.Background(), req)
	require.NoError(t, err)

	t.Run("successful retrieval", func(t *testing.T) {
		alert, err := repo.GetByID(context.Background(), created.ID)
		require.NoError(t, err)
		require.NotNil(t, alert)

		assert.Equal(t, created.ID, alert.ID)
		assert.Equal(t, created.SiteID, alert.SiteID)
		assert.Equal(t, created.RuleType, alert.RuleType)
		assert.Equal(t, created.Severity, alert.Severity)
		assert.Equal(t, created.Title, alert.Title)
		assert.Equal(t, created.Description, alert.Description)
	})

	t.Run("alert not found", func(t *testing.T) {
		alert, err := repo.GetByID(context.Background(), "550e8400-e29b-41d4-a716-446655440000")
		require.Error(t, err)
		assert.Nil(t, alert)
		assert.Equal(t, ErrAlertNotFound, err)
	})
}

func TestAlertRepo_List(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	db := testutil.SetupTestDB(t)
	repo := NewAlertRepo(db)
	site := createTestSite(t, db)

	// Create test alerts
	alerts := []*model.CreateAlertRequest{
		{
			SiteID:      site.ID,
			RuleType:    "unknown_domain",
			Severity:    "medium",
			Title:       "Unknown domain 1",
			Description: "First unknown domain",
		},
		{
			SiteID:      site.ID,
			RuleType:    "ioc_domain",
			Severity:    "critical",
			Title:       "IOC domain 1",
			Description: "First IOC domain",
		},
		{
			SiteID:      site.ID,
			RuleType:    "yara_rule",
			Severity:    "high",
			Title:       "YARA match 1",
			Description: "First YARA match",
		},
	}

	var createdAlerts []*model.Alert
	for _, req := range alerts {
		alert, err := repo.Create(context.Background(), req)
		require.NoError(t, err)
		createdAlerts = append(createdAlerts, alert)
	}
	_ = createdAlerts // Use the variable to avoid staticcheck warning

	t.Run("list all alerts", func(t *testing.T) {
		opts := &model.AlertListOptions{
			Limit: 10,
		}

		results, err := repo.List(context.Background(), opts)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)

		// Verify ordering (most recent first)
		for i := 1; i < len(results); i++ {
			assert.True(t, results[i-1].FiredAt.After(results[i].FiredAt) ||
				results[i-1].FiredAt.Equal(results[i].FiredAt))
		}
	})

	t.Run("filter by site ID", func(t *testing.T) {
		opts := &model.AlertListOptions{
			SiteID: &site.ID,
			Limit:  10,
		}

		results, err := repo.List(context.Background(), opts)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 3)

		for _, alert := range results {
			assert.Equal(t, site.ID, alert.SiteID)
		}
	})

	t.Run("filter by rule type", func(t *testing.T) {
		ruleType := "ioc_domain"
		opts := &model.AlertListOptions{
			RuleType: &ruleType,
			Limit:    10,
		}

		results, err := repo.List(context.Background(), opts)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)

		for _, alert := range results {
			assert.Equal(t, "ioc_domain", alert.RuleType)
		}
	})

	t.Run("filter by severity", func(t *testing.T) {
		severity := "critical"
		opts := &model.AlertListOptions{
			Severity: &severity,
			Limit:    10,
		}

		results, err := repo.List(context.Background(), opts)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(results), 1)

		for _, alert := range results {
			assert.Equal(t, "critical", alert.Severity)
		}
	})

	t.Run("pagination", func(t *testing.T) {
		opts := &model.AlertListOptions{
			SiteID: &site.ID,
			Limit:  2,
			Offset: 0,
		}

		page1, err := repo.List(context.Background(), opts)
		require.NoError(t, err)
		assert.LessOrEqual(t, len(page1), 2)

		opts.Offset = 2
		page2, err := repo.List(context.Background(), opts)
		require.NoError(t, err)

		// Ensure no overlap between pages
		if len(page1) > 0 && len(page2) > 0 {
			assert.NotEqual(t, page1[0].ID, page2[0].ID)
		}
	})
}

func TestAlertRepo_UpdateDeliveryStatus(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	db := testutil.SetupTestDB(t)
	repo := NewAlertRepo(db)
	site := createTestSite(t, db)

	alert, err := repo.Create(context.Background(), &model.CreateAlertRequest{
		SiteID:      site.ID,
		RuleType:    string(model.AlertRuleTypeUnknownDomain),
		Severity:    string(model.AlertSeverityHigh),
		Title:       "Initial alert",
		Description: "testing delivery status update",
	})
	require.NoError(t, err)
	require.NotNil(t, alert)
	assert.Equal(t, model.AlertDeliveryStatusPending, alert.DeliveryStatus)

	updated, err := repo.UpdateDeliveryStatus(context.Background(), core.UpdateAlertDeliveryStatusParams{
		ID:     alert.ID,
		Status: model.AlertDeliveryStatusMuted,
	})
	require.NoError(t, err)
	require.NotNil(t, updated)
	assert.Equal(t, model.AlertDeliveryStatusMuted, updated.DeliveryStatus)

	_, err = repo.UpdateDeliveryStatus(context.Background(), core.UpdateAlertDeliveryStatusParams{
		ID:     "non-existent-id",
		Status: model.AlertDeliveryStatusMuted,
	})
	require.Error(t, err)
	assert.Equal(t, ErrAlertNotFound, err)
}

func TestAlertRepo_Stats(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	db := testutil.SetupTestDB(t)
	repo := NewAlertRepo(db)
	site := createTestSite(t, db)

	// Create alerts with different severities
	severities := []string{"critical", "high", "medium", "low", "info"}
	for _, severity := range severities {
		req := &model.CreateAlertRequest{
			SiteID:      site.ID,
			RuleType:    "unknown_domain",
			Severity:    severity,
			Title:       "Test Alert " + severity,
			Description: "Test description",
		}
		_, err := repo.Create(context.Background(), req)
		require.NoError(t, err)
	}

	t.Run("stats for specific site", func(t *testing.T) {
		stats, err := repo.Stats(context.Background(), &site.ID)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.Equal(t, 5, stats.Total)
		assert.Equal(t, 1, stats.Critical)
		assert.Equal(t, 1, stats.High)
		assert.Equal(t, 1, stats.Medium)
		assert.Equal(t, 1, stats.Low)
		assert.Equal(t, 1, stats.Info)
		assert.Equal(t, 5, stats.Unresolved) // All alerts are unresolved initially
	})

	t.Run("global stats", func(t *testing.T) {
		stats, err := repo.Stats(context.Background(), nil)
		require.NoError(t, err)
		require.NotNil(t, stats)

		assert.GreaterOrEqual(t, stats.Total, 5)
		assert.GreaterOrEqual(t, stats.Critical, 1)
		assert.GreaterOrEqual(t, stats.High, 1)
		assert.GreaterOrEqual(t, stats.Medium, 1)
		assert.GreaterOrEqual(t, stats.Low, 1)
		assert.GreaterOrEqual(t, stats.Info, 1)
	})
}

func TestAlertRepo_Resolve(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	db := testutil.SetupTestDB(t)
	repo := NewAlertRepo(db)
	site := createTestSite(t, db)

	// Create a test alert
	req := &model.CreateAlertRequest{
		SiteID:      site.ID,
		RuleType:    "unknown_domain",
		Severity:    "medium",
		Title:       "Test Alert",
		Description: "Test description",
	}

	created, err := repo.Create(context.Background(), req)
	require.NoError(t, err)
	assert.Nil(t, created.ResolvedAt)

	t.Run("successful resolution", func(t *testing.T) {
		resolved, err := repo.Resolve(context.Background(), core.ResolveAlertParams{
			ID:         created.ID,
			ResolvedBy: "test@example.com",
		})
		require.NoError(t, err)
		require.NotNil(t, resolved)

		assert.Equal(t, created.ID, resolved.ID)
		assert.NotNil(t, resolved.ResolvedAt)
		assert.True(t, resolved.ResolvedAt.After(created.CreatedAt))
	})

	t.Run("already resolved alert", func(t *testing.T) {
		// Try to resolve again
		resolved, err := repo.Resolve(context.Background(), core.ResolveAlertParams{
			ID:         created.ID,
			ResolvedBy: "test@example.com",
		})
		require.Error(t, err)
		assert.Nil(t, resolved)
		assert.Equal(t, ErrAlertNotFound, err)
	})

	t.Run("non-existent alert", func(t *testing.T) {
		resolved, err := repo.Resolve(context.Background(), core.ResolveAlertParams{
			ID:         "550e8400-e29b-41d4-a716-446655440000",
			ResolvedBy: "test@example.com",
		})
		require.Error(t, err)
		assert.Nil(t, resolved)
		assert.Equal(t, ErrAlertNotFound, err)
	})
}

// Helper function for creating bool pointers.
func boolPtr(b bool) *bool {
	return &b
}

// Helper function to create a test site with source.
func createTestSite(t *testing.T, db *sql.DB) *model.Site {
	t.Helper()

	// Create a test source first
	sourceRepo := NewSourceRepo(db)
	source, err := sourceRepo.Create(context.Background(), &model.CreateSourceRequest{
		Name:  "Test Source",
		Value: "console.log('test')",
		Test:  true,
	})
	require.NoError(t, err)

	// Create a test site
	siteRepo := NewSiteRepo(db)
	site, err := siteRepo.Create(context.Background(), &model.CreateSiteRequest{
		Name:            "Test Site",
		Enabled:         boolPtr(true),
		RunEveryMinutes: 30,
		SourceID:        source.ID,
	})
	require.NoError(t, err)

	return site
}

func createTestSiteWithName(t *testing.T, db *sql.DB, siteName, sourceName string) *model.Site {
	// Create a test source first
	sourceRepo := NewSourceRepo(db)
	source, err := sourceRepo.Create(context.Background(), &model.CreateSourceRequest{
		Name:  sourceName,
		Value: "https://example.com",
		Test:  false,
	})
	require.NoError(t, err)

	// Create a test site
	siteRepo := NewSiteRepo(db)
	site, err := siteRepo.Create(context.Background(), &model.CreateSiteRequest{
		Name:            siteName,
		Enabled:         boolPtr(true),
		RunEveryMinutes: 30,
		SourceID:        source.ID,
	})
	require.NoError(t, err)

	return site
}

func TestAlertRepo_ListWithSiteNames(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	db := testutil.SetupTestDB(t)
	repo := NewAlertRepo(db)

	// Create unique sites with different sources
	site1 := createTestSiteWithName(t, db, "Test Site 1", "test-source-1")
	site2 := createTestSiteWithName(t, db, "Test Site 2", "test-source-2")

	// Create alerts for both sites
	alert1, err := repo.Create(context.Background(), &model.CreateAlertRequest{
		SiteID:      site1.ID,
		RuleType:    "unknown_domain",
		Severity:    "high",
		Title:       "Alert for Site 1",
		Description: "Test alert for site 1",
	})
	require.NoError(t, err)

	alert2, err := repo.Create(context.Background(), &model.CreateAlertRequest{
		SiteID:      site2.ID,
		RuleType:    "ioc_domain",
		Severity:    "critical",
		Title:       "Alert for Site 2",
		Description: "Test alert for site 2",
	})
	require.NoError(t, err)

	t.Run("list all alerts with site names", func(t *testing.T) {
		opts := &model.AlertListOptions{
			Limit:  10,
			Offset: 0,
		}

		alertsWithSiteNames, err := repo.ListWithSiteNames(context.Background(), opts)
		require.NoError(t, err)
		require.Len(t, alertsWithSiteNames, 2)

		// Verify alerts are returned with site names
		alertMap := make(map[string]*model.AlertWithSiteName)
		for _, alert := range alertsWithSiteNames {
			alertMap[alert.ID] = alert
		}

		// Check alert1 with site1 name
		assert.Contains(t, alertMap, alert1.ID)
		alertWithSite1 := alertMap[alert1.ID]
		assert.Equal(t, site1.Name, alertWithSite1.SiteName)
		assert.Equal(t, alert1.Title, alertWithSite1.Title)
		assert.Equal(t, alert1.SiteID, alertWithSite1.SiteID)

		// Check alert2 with site2 name
		assert.Contains(t, alertMap, alert2.ID)
		alertWithSite2 := alertMap[alert2.ID]
		assert.Equal(t, site2.Name, alertWithSite2.SiteName)
		assert.Equal(t, alert2.Title, alertWithSite2.Title)
		assert.Equal(t, alert2.SiteID, alertWithSite2.SiteID)
	})

	t.Run("filter by site ID with site names", func(t *testing.T) {
		opts := &model.AlertListOptions{
			SiteID: &site1.ID,
			Limit:  10,
			Offset: 0,
		}

		alertsWithSiteNames, err := repo.ListWithSiteNames(context.Background(), opts)
		require.NoError(t, err)
		require.Len(t, alertsWithSiteNames, 1)

		alert := alertsWithSiteNames[0]
		assert.Equal(t, alert1.ID, alert.ID)
		assert.Equal(t, site1.Name, alert.SiteName)
		assert.Equal(t, site1.ID, alert.SiteID)
	})

	t.Run("pagination with site names", func(t *testing.T) {
		opts := &model.AlertListOptions{
			Limit:  1,
			Offset: 0,
		}

		alertsWithSiteNames, err := repo.ListWithSiteNames(context.Background(), opts)
		require.NoError(t, err)
		require.Len(t, alertsWithSiteNames, 1)

		// Verify site name is included
		alert := alertsWithSiteNames[0]
		assert.NotEmpty(t, alert.SiteName)
		assert.True(t, alert.SiteName == site1.Name || alert.SiteName == site2.Name)
	})
}

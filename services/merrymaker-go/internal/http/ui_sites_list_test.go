package httpx

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createTestSource creates a test source for use in site tests.
func createTestSource(ctx context.Context, t *testing.T, db *sql.DB) *model.Source {
	t.Helper()
	sourceRepo := data.NewSourceRepo(db)
	source, err := sourceRepo.Create(ctx, &model.CreateSourceRequest{
		Name:  "test-source",
		Value: "console.log('test');",
		Test:  true,
	})
	require.NoError(t, err)
	return source
}

type createSiteParams struct {
	Ctx     context.Context
	DB      *sql.DB
	Request *model.CreateSiteRequest
}

// createTestSite creates a test site with the given request.
func createTestSite(t *testing.T, params createSiteParams) *model.Site {
	t.Helper()
	siteRepo := data.NewSiteRepo(params.DB)
	site, err := siteRepo.Create(params.Ctx, params.Request)
	require.NoError(t, err)
	return site
}

func TestUIHandlers_Sites_ListWithNoFilters(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newSiteUIHandlerForTest(t, db)
		ctx := context.Background()

		// Create a source (required by sites)
		source := createTestSource(ctx, t, db)

		// Create some test sites
		enabled := true
		disabled := false

		site1 := createTestSite(t, createSiteParams{
			Ctx: ctx,
			DB:  db,
			Request: &model.CreateSiteRequest{
				Name:            "site-alpha",
				Enabled:         &enabled,
				RunEveryMinutes: 5,
				SourceID:        source.ID,
			},
		})

		site2 := createTestSite(t, createSiteParams{
			Ctx: ctx,
			DB:  db,
			Request: &model.CreateSiteRequest{
				Name:            "site-beta",
				Enabled:         &disabled,
				RunEveryMinutes: 10,
				SourceID:        source.ID,
			},
		})

		prodScope := "prod"
		site3 := createTestSite(t, createSiteParams{
			Ctx: ctx,
			DB:  db,
			Request: &model.CreateSiteRequest{
				Name:            "site-gamma",
				Enabled:         &enabled,
				Scope:           &prodScope,
				RunEveryMinutes: 15,
				SourceID:        source.ID,
			},
		})

		// Test: List all sites (no filters)
		r := httptest.NewRequest(http.MethodGet, "/sites", nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()

		h.Sites(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		body := w.Body.String()

		// Should contain all three sites
		assert.Contains(t, body, site1.Name)
		assert.Contains(t, body, site2.Name)
		assert.Contains(t, body, site3.Name)
		assert.NotContains(t, body, "No sites found")
	})
}

func TestUIHandlers_Sites_ListWithEnabledFilter(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newSiteUIHandlerForTest(t, db)
		ctx := context.Background()

		// Create a source
		source := createTestSource(ctx, t, db)

		// Create sites
		enabled := true
		disabled := false

		enabledSite := createTestSite(t, createSiteParams{
			Ctx: ctx,
			DB:  db,
			Request: &model.CreateSiteRequest{
				Name:            "enabled-site",
				Enabled:         &enabled,
				RunEveryMinutes: 5,
				SourceID:        source.ID,
			},
		})

		disabledSite := createTestSite(t, createSiteParams{
			Ctx: ctx,
			DB:  db,
			Request: &model.CreateSiteRequest{
				Name:            "disabled-site",
				Enabled:         &disabled,
				RunEveryMinutes: 10,
				SourceID:        source.ID,
			},
		})

		// Test: Filter by enabled=true
		r := httptest.NewRequest(http.MethodGet, "/sites?enabled=true", nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()

		h.Sites(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		body := w.Body.String()

		// Should contain only enabled site
		assert.Contains(t, body, enabledSite.Name)
		assert.NotContains(t, body, disabledSite.Name)
		assert.NotContains(t, body, "No sites found")
	})
}

func TestUIHandlers_Sites_ListWithScopeFilter(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newSiteUIHandlerForTest(t, db)
		ctx := context.Background()

		// Create a source
		source := createTestSource(ctx, t, db)

		// Create sites with different scopes
		enabled := true
		prodScope := "prod"
		stagingScope := "staging"

		prodSite := createTestSite(t, createSiteParams{
			Ctx: ctx,
			DB:  db,
			Request: &model.CreateSiteRequest{
				Name:            "prod-site",
				Enabled:         &enabled,
				Scope:           &prodScope,
				RunEveryMinutes: 5,
				SourceID:        source.ID,
			},
		})

		stagingSite := createTestSite(t, createSiteParams{
			Ctx: ctx,
			DB:  db,
			Request: &model.CreateSiteRequest{
				Name:            "staging-site",
				Enabled:         &enabled,
				Scope:           &stagingScope,
				RunEveryMinutes: 10,
				SourceID:        source.ID,
			},
		})

		// Test: Filter by scope=prod
		r := httptest.NewRequest(http.MethodGet, "/sites?scope=prod", nil)
		r.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()

		h.Sites(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "text/html; charset=utf-8", w.Header().Get("Content-Type"))
		body := w.Body.String()

		// Should contain only prod site
		assert.Contains(t, body, prodSite.Name)
		assert.NotContains(t, body, stagingSite.Name)
		assert.NotContains(t, body, "No sites found")
	})
}

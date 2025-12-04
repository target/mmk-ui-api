package httpx

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func newSiteUIHandlerForTest(t *testing.T, db *sql.DB) *UIHandlers {
	t.Helper()
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	// Repos
	siteRepo := data.NewSiteRepo(db)
	adminRepo := data.NewScheduledJobsAdminRepo(db)
	sourceRepo := data.NewSourceRepo(db)
	jobRepo := data.NewJobRepo(db, data.RepoConfig{})
	alertRepo := data.NewHTTPAlertSinkRepo(db)
	secretRepo := data.NewSecretRepo(db, cryptoutil.NoopEncryptor{})

	// Services
	siteSvc := service.NewSiteService(service.SiteServiceOptions{SiteRepo: siteRepo, Admin: adminRepo})
	sourceSvc := service.NewSourceService(service.SourceServiceOptions{
		SourceRepo: sourceRepo,
		Jobs:       jobRepo,
		SecretRepo: secretRepo,
	})
	alertSvc := service.MustNewHTTPAlertSinkService(service.HTTPAlertSinkServiceOptions{Repo: alertRepo})

	return &UIHandlers{T: tr, SiteSvc: siteSvc, SourceSvc: sourceSvc, Sinks: alertSvc}
}

func createDepsForSite(t *testing.T, db *sql.DB) (string, string) {
	t.Helper()
	sourceRepo := data.NewSourceRepo(db)
	alertRepo := data.NewHTTPAlertSinkRepo(db)

	src, err := sourceRepo.Create(context.Background(), &model.CreateSourceRequest{
		Name:  "src-for-site",
		Value: "console.log('ok')",
	})
	require.NoError(t, err)

	sink, err := alertRepo.Create(context.Background(), &model.CreateHTTPAlertSinkRequest{
		Name:   "sink-for-site",
		URI:    "https://example.com/hook",
		Method: "POST",
	})
	require.NoError(t, err)

	return src.ID, sink.ID
}

func TestUIHandlers_SiteCreate_ValidationAndSuccess(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newSiteUIHandlerForTest(t, db)

		t.Run("validation errors", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "")
			form.Set("run_every_minutes", "")
			form.Set("source_id", "")
			form.Set("alert_sink_id", "")

			r := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			h.SiteCreate(w, r)

			res := w.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
			body := w.Body.String()
			assert.Contains(t, body, "Name is required.")
			assert.Contains(t, body, "Run interval must be &gt; 0.")
			assert.Contains(t, body, "Source is required.")
			assert.Contains(t, body, "Alert Sink is required.")
			assert.Contains(t, body, "Please fix the errors below.")
		})

		t.Run("success", func(t *testing.T) {
			sourceID, sinkID := createDepsForSite(t, db)

			form := url.Values{}
			form.Set("name", "my-site-1")
			form.Set("enabled", "on")
			form.Set("scope", "prod")
			form.Set("alert_sink_id", sinkID)
			form.Set("run_every_minutes", "5")
			form.Set("source_id", sourceID)
			form.Set("alert_mode", "muted")

			r := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			h.SiteCreate(w, r)

			res := w.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/sites", res.Header.Get("Hx-Redirect"))

			site, err := data.NewSiteRepo(db).GetByName(context.Background(), "my-site-1")
			require.NoError(t, err)
			require.NotNil(t, site)
			assert.Equal(t, model.SiteAlertModeMuted, site.AlertMode)
		})
	})
}

func TestUIHandlers_SiteUpdate_ValidationAndSuccess(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newSiteUIHandlerForTest(t, db)

		// Seed a site to edit
		sourceID, sinkID := createDepsForSite(t, db)
		siteRepo := data.NewSiteRepo(db)
		enabled := true
		s, err := siteRepo.Create(context.Background(), &model.CreateSiteRequest{
			Name:            "orig-site",
			Enabled:         &enabled,
			Scope:           nil,
			HTTPAlertSinkID: &sinkID,
			RunEveryMinutes: 10,
			SourceID:        sourceID,
		})
		require.NoError(t, err)

		t.Run("validation errors", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "")
			form.Set("run_every_minutes", "0")
			form.Set("source_id", "")
			form.Set("alert_sink_id", "")

			r := httptest.NewRequest(http.MethodPost, "/sites/"+s.ID, strings.NewReader(form.Encode()))
			r.SetPathValue("id", s.ID)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			h.SiteUpdate(w, r)

			res := w.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
			body := w.Body.String()
			assert.Contains(t, body, "Name is required.")
			assert.Contains(t, body, "Run interval must be &gt; 0.")
			assert.Contains(t, body, "Source is required.")
			assert.Contains(t, body, "Alert Sink is required.")
			assert.Contains(t, body, "Please fix the errors below.")
		})

		t.Run("success", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "updated-site")
			form.Set("enabled", "on")
			form.Set("scope", "stage")
			form.Set("alert_sink_id", sinkID)
			form.Set("run_every_minutes", "15")
			form.Set("source_id", sourceID)
			form.Set("alert_mode", "muted")

			r := httptest.NewRequest(http.MethodPost, "/sites/"+s.ID, strings.NewReader(form.Encode()))
			r.SetPathValue("id", s.ID)
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			w := httptest.NewRecorder()

			h.SiteUpdate(w, r)

			res := w.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/sites", res.Header.Get("Hx-Redirect"))

			updated, err := data.NewSiteRepo(db).GetByID(context.Background(), s.ID)
			require.NoError(t, err)
			require.NotNil(t, updated)
			assert.Equal(t, model.SiteAlertModeMuted, updated.AlertMode)
		})
	})
}

func TestUIHandlers_SiteCreate_HTMX_PartialValidation(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newSiteUIHandlerForTest(t, db)

		form := url.Values{}
		form.Set("name", "")
		form.Set("run_every_minutes", "")
		form.Set("source_id", "")
		form.Set("alert_sink_id", "")

		r := httptest.NewRequest(http.MethodPost, "/sites", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		// Simulate HTMX partial request
		r.Header.Set("Hx-Request", "true")
		w := httptest.NewRecorder()

		h.SiteCreate(w, r)

		res := w.Result()
		t.Cleanup(func() { _ = res.Body.Close() })
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
		body := w.Body.String()
		// Partial should not include full HTML layout
		assert.NotContains(t, body, "<!doctype html>")
		// Should include title and OOB header swap + form and banner
		assert.Contains(t, body, "<title>Merrymaker - New Site</title>")
		assert.Contains(t, body, "hx-swap-oob=\"outerHTML\"")
		assert.Contains(t, body, "id=\"site-form\"")
		assert.Contains(t, body, "Please fix the errors below.")
	})
}

func TestUIHandlers_SiteUpdate_HTMX_PartialValidation(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newSiteUIHandlerForTest(t, db)
		// Seed a site to edit
		sourceID, sinkID := createDepsForSite(t, db)
		siteRepo := data.NewSiteRepo(db)
		enabled := true
		s, err := siteRepo.Create(context.Background(), &model.CreateSiteRequest{
			Name:            "edit-me",
			Enabled:         &enabled,
			Scope:           nil,
			HTTPAlertSinkID: &sinkID,
			RunEveryMinutes: 10,
			SourceID:        sourceID,
		})
		require.NoError(t, err)

		form := url.Values{}
		form.Set("name", "")
		form.Set("run_every_minutes", "0")
		form.Set("source_id", "")
		form.Set("alert_sink_id", "")

		r := httptest.NewRequest(http.MethodPost, "/sites/"+s.ID, strings.NewReader(form.Encode()))
		r.SetPathValue("id", s.ID)
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		r.Header.Set("Hx-Request", "true")
		w := httptest.NewRecorder()

		h.SiteUpdate(w, r)

		res := w.Result()
		t.Cleanup(func() { _ = res.Body.Close() })
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
		body := w.Body.String()
		assert.NotContains(t, body, "<!doctype html>")
		assert.Contains(t, body, "<title>Merrymaker - Edit Site</title>")
		assert.Contains(t, body, "hx-swap-oob=\"outerHTML\"")
		assert.Contains(t, body, "id=\"site-form\"")
		assert.Contains(t, body, "Please fix the errors below.")
	})
}

package httpx

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIHandlers_AlertSinkNew(t *testing.T) {
	h := CreateUIHandlersForTest(t)
	require.NotNil(t, h)
	rq := httptest.NewRequest(http.MethodGet, "/alert-sinks/new", nil)
	rr := httptest.NewRecorder()

	h.AlertSinkNew(rr, rq)

	res := rr.Result()
	t.Cleanup(func() { _ = res.Body.Close() })

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
	body := rr.Body.String()
	assert.Contains(t, body, "HTTP Alert Sink")
	assert.Contains(t, body, `name="name"`)
	assert.Contains(t, body, `name="method"`)
	assert.Contains(t, body, `name="uri"`)

	// Verify JMESPath Body Builder elements are present
	assert.Contains(t, body, "JMESPath Body Builder")
	assert.Contains(t, body, `id="body"`)
	assert.Contains(t, body, `id="sample_data"`)
	assert.Contains(t, body, `id="jmes_result"`)
	assert.Contains(t, body, `id="jmes-validity"`)
	assert.Contains(t, body, `data-component="alert-sink-form"`)
}

func TestUIHandlers_AlertSinkNew_HTMXPartial(t *testing.T) {
	h := CreateUIHandlersForTest(t)
	require.NotNil(t, h)

	// Create HTMX request
	rq := httptest.NewRequest(http.MethodGet, "/alert-sinks/new", nil)
	rq.Header.Set("Hx-Request", "true") // Mark as HTMX request
	rr := httptest.NewRecorder()

	h.AlertSinkNew(rr, rq)

	res := rr.Result()
	t.Cleanup(func() { _ = res.Body.Close() })

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
	body := rr.Body.String()

	// Verify form elements are present in partial render
	assert.Contains(t, body, "HTTP Alert Sink")
	assert.Contains(t, body, `id="alert-sink-form"`)

	// Verify JMESPath Body Builder elements are present in partial render
	assert.Contains(t, body, "JMESPath Body Builder")
	assert.Contains(t, body, `id="body"`)
	assert.Contains(t, body, `id="sample_data"`)
	assert.Contains(t, body, `id="jmes_result"`)
	assert.Contains(t, body, `id="jmes-validity"`)

	// Most importantly: verify the JavaScript is included in HTMX partial renders
	// This is critical for the fix to work when navigating via HTMX swap
	assert.Contains(t, body, `data-component="alert-sink-form"`)
}

func TestUIHandlers_AlertSinkEdit_And_Update(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup services
		tr := RequireTemplateRenderer(t)
		require.NotNil(t, tr)

		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretSvc := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		repo := data.NewHTTPAlertSinkRepo(db)
		sinkSvc := service.MustNewHTTPAlertSinkService(service.HTTPAlertSinkServiceOptions{Repo: repo})

		h := &UIHandlers{T: tr, Sinks: sinkSvc, SecretSvc: secretSvc}

		// Seed one secret and one sink
		ctx := context.Background()
		sec, err := secretRepo.Create(ctx, model.CreateSecretRequest{Name: "API_KEY", Value: "secret"})
		require.NoError(t, err)
		_ = sec

		created, err := repo.Create(
			ctx,
			&model.CreateHTTPAlertSinkRequest{
				Name:    "sink-1",
				Method:  "POST",
				URI:     "https://example.com/hook",
				Secrets: []string{"API_KEY"},
			},
		)
		require.NoError(t, err)

		t.Run("edit existing", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/alert-sinks/"+created.ID+"/edit", nil)
			r.SetPathValue("id", created.ID)
			rr := httptest.NewRecorder()

			h.AlertSinkEdit(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()
			assert.Contains(t, body, "sink-1")
			assert.Contains(t, body, `name="name"`)
			// Verify that nil pointer fields don't render as "<nil>"
			assert.NotContains(t, body, "<nil>")
		})

		t.Run("edit missing id => 404", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/alert-sinks//edit", nil)
			rr := httptest.NewRecorder()
			h.AlertSinkEdit(rr, r)
			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})

		t.Run("update valid", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "sink-1-upd")
			form.Set("method", "PUT")
			form.Set("uri", "https://example.org/new")
			form.Add("secrets", "API_KEY")
			form.Set("ok_status", "201")
			form.Set("retry", "2")

			r := httptest.NewRequest(http.MethodPost, "/alert-sinks/"+created.ID, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.SetPathValue("id", created.ID)
			rr := httptest.NewRecorder()

			h.AlertSinkUpdate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/alert-sinks", res.Header.Get("Hx-Redirect"))

			// Verify persisted
			upd, err := repo.GetByID(ctx, created.ID)
			require.NoError(t, err)
			assert.Equal(t, "sink-1-upd", upd.Name)
			assert.Equal(t, 201, upd.OkStatus)
			assert.Equal(t, 2, upd.Retry)
		})

		t.Run("update non-existent => 404", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "x")
			form.Set("method", "GET")
			form.Set("uri", "https://x")
			form.Add("secrets", "API_KEY")

			r := httptest.NewRequest(http.MethodPost, "/alert-sinks/non-existent", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.SetPathValue("id", "non-existent")
			rr := httptest.NewRecorder()

			h.AlertSinkUpdate(rr, r)
			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})
	})
}

func TestUIHandlers_AlertSinkCreate(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup services
		tr, err := NewTemplateRenderer(TemplateRendererConfig{
			TemplateFS: os.DirFS("../../frontend/templates"),
		})
		require.NoError(t, err)

		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretSvc := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		repo := data.NewHTTPAlertSinkRepo(db)
		sinkSvc := service.MustNewHTTPAlertSinkService(service.HTTPAlertSinkServiceOptions{Repo: repo})

		h := &UIHandlers{T: tr, Sinks: sinkSvc, SecretSvc: secretSvc}

		ctx := context.Background()
		_, err = secretRepo.Create(ctx, model.CreateSecretRequest{Name: "API_KEY", Value: "secret"})
		require.NoError(t, err)

		t.Run("valid creation", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "sink-a")
			form.Set("method", "POST")
			form.Set("uri", "https://example.com/hook")
			form.Add("secrets", "API_KEY")

			r := httptest.NewRequest(http.MethodPost, "/alert-sinks", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.AlertSinkCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/alert-sinks", res.Header.Get("Hx-Redirect"))
		})

		t.Run("valid creation with lowercase method", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "sink-lowercase")
			form.Set("method", "post") // lowercase method should be accepted
			form.Set("uri", "https://example.com/hook")
			form.Add("secrets", "API_KEY")

			r := httptest.NewRequest(http.MethodPost, "/alert-sinks", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.AlertSinkCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/alert-sinks", res.Header.Get("Hx-Redirect"))

			// Verify the method was normalized to uppercase in the database
			sinks, err := repo.List(ctx, 10, 0)
			require.NoError(t, err)
			var found *model.HTTPAlertSink
			for _, s := range sinks {
				if s.Name == "sink-lowercase" {
					found = s
					break
				}
			}
			require.NotNil(t, found, "sink-lowercase should exist")
			assert.Equal(t, "POST", found.Method, "method should be normalized to uppercase")
		})

		t.Run("invalid method", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "sink-b")
			form.Set("method", "PATCH")
			form.Set("uri", "https://example.com/hook")
			form.Add("secrets", "API_KEY")

			r := httptest.NewRequest(http.MethodPost, "/alert-sinks", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.AlertSinkCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Contains(t, rr.Body.String(), "Method must be one of: GET, POST, PUT, DELETE")
		})

		t.Run("duplicate name", func(t *testing.T) {
			// Seed a sink with the same name
			_, err := repo.Create(
				ctx,
				&model.CreateHTTPAlertSinkRequest{
					Name:    "dup",
					Method:  "GET",
					URI:     "https://e.com",
					Secrets: []string{"API_KEY"},
				},
			)
			require.NoError(t, err)

			form := url.Values{}
			form.Set("name", "dup")
			form.Set("method", "GET")
			form.Set("uri", "https://e.com")
			form.Add("secrets", "API_KEY")

			r := httptest.NewRequest(http.MethodPost, "/alert-sinks", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.AlertSinkCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Contains(t, rr.Body.String(), "already exists")
		})

		t.Run("no secrets (optional)", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "sink-c")
			form.Set("method", "GET")
			form.Set("uri", "https://example.com")
			// no secrets - this should be allowed now

			r := httptest.NewRequest(http.MethodPost, "/alert-sinks", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.AlertSinkCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/alert-sinks", res.Header.Get("Hx-Redirect"))
		})
	})
}

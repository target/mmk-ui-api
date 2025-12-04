package httpx

import (
	"context"
	"database/sql"
	"errors"
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

type stubSecretsService struct {
	createErr error
}

func (s *stubSecretsService) List(context.Context, int, int) ([]*model.Secret, error) {
	return nil, nil
}

func (s *stubSecretsService) GetByID(context.Context, string) (*model.Secret, error) {
	return nil, data.ErrSecretNotFound
}

func (s *stubSecretsService) Create(context.Context, model.CreateSecretRequest) (*model.Secret, error) {
	if s.createErr != nil {
		return nil, s.createErr
	}
	return &model.Secret{ID: "fake"}, nil
}

func (s *stubSecretsService) Update(context.Context, string, model.UpdateSecretRequest) (*model.Secret, error) {
	return &model.Secret{ID: "fake"}, nil
}

func (s *stubSecretsService) Delete(context.Context, string) (bool, error) {
	return false, nil
}

func TestValidateSecret(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		secretName   string
		value        string
		requireValue bool
		wantErrs     map[string]string
	}{
		{
			name:         "valid secret",
			secretName:   "TEST_SECRET",
			value:        "secret-value-123",
			requireValue: true,
			wantErrs:     map[string]string{},
		},
		{
			name:         "valid secret without value requirement - empty value",
			secretName:   "TEST_SECRET",
			value:        "",
			requireValue: false,
			wantErrs:     map[string]string{},
		},
		{
			name:         "optional value but provided - should validate",
			secretName:   "TEST_SECRET",
			value:        "new-secret-value",
			requireValue: false,
			wantErrs:     map[string]string{},
		},
		{
			name:         "optional value but too long - should fail",
			secretName:   "TEST_SECRET",
			value:        strings.Repeat("x", 10001),
			requireValue: false,
			wantErrs:     map[string]string{"value": "Secret cannot exceed 10000 characters."},
		},
		{
			name:         "empty name",
			secretName:   "",
			value:        "secret-value-123",
			requireValue: true,
			wantErrs: map[string]string{
				"name": "Use letters, digits, underscore, and hyphens. Max 255 characters.",
			},
		},
		{
			name:         "valid name - starts with digit",
			secretName:   "1TEST_SECRET",
			value:        "secret-value-123",
			requireValue: true,
			wantErrs:     map[string]string{},
		},
		{
			name:         "valid name - contains hyphens",
			secretName:   "TEST-SECRET",
			value:        "secret-value-123",
			requireValue: true,
			wantErrs:     map[string]string{},
		},
		{
			name:         "invalid name - contains special chars",
			secretName:   "TEST@SECRET",
			value:        "secret-value-123",
			requireValue: true,
			wantErrs: map[string]string{
				"name": "Use letters, digits, underscore, and hyphens. Max 255 characters.",
			},
		},
		{
			name:         "name too long",
			secretName:   strings.Repeat("A", 256),
			value:        "secret-value-123",
			requireValue: true,
			wantErrs: map[string]string{
				"name": "Use letters, digits, underscore, and hyphens. Max 255 characters.",
			},
		},
		{
			name:         "valid name with underscore start",
			secretName:   "_TEST_SECRET",
			value:        "secret-value-123",
			requireValue: true,
			wantErrs:     map[string]string{},
		},
		{
			name:         "valid name with mixed case and numbers",
			secretName:   "Test_Secret_123",
			value:        "secret-value-123",
			requireValue: true,
			wantErrs:     map[string]string{},
		},
		{
			name:         "valid name with hyphens and underscores",
			secretName:   "API-KEY_V2",
			value:        "secret-value-123",
			requireValue: true,
			wantErrs:     map[string]string{},
		},
		{
			name:         "empty value when required",
			secretName:   "TEST_SECRET",
			value:        "",
			requireValue: true,
			wantErrs:     map[string]string{"value": "Secret is required."},
		},
		{
			name:         "whitespace only value when required",
			secretName:   "TEST_SECRET",
			value:        "   ",
			requireValue: true,
			wantErrs:     map[string]string{"value": "Secret is required."},
		},
		{
			name:         "multiple validation errors",
			secretName:   "",
			value:        "",
			requireValue: true,
			wantErrs: map[string]string{
				"name":  "Use letters, digits, underscore, and hyphens. Max 255 characters.",
				"value": "Secret is required.",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			errs := validateSecret(tt.secretName, tt.value, tt.requireValue)
			assert.Equal(t, tt.wantErrs, errs)
		})
	}
}

func TestUIHandlers_SecretNew(t *testing.T) {
	h := CreateUIHandlersForTest(t)
	require.NotNil(t, h)
	r := httptest.NewRequest(http.MethodGet, "/secrets/new", nil)
	rr := httptest.NewRecorder()

	h.SecretNew(rr, r)

	res := rr.Result()
	t.Cleanup(func() { _ = res.Body.Close() })

	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Type"), "text/html")

	body := rr.Body.String()
	assert.Contains(t, body, "Secret")
	assert.Contains(t, body, `name="name"`)
	assert.Contains(t, body, `name="value"`)
}

func TestUIHandlers_SecretEdit(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		tr := RequireTemplateRenderer(t)
		require.NotNil(t, tr)

		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})

		// Create a test secret
		ctx := context.Background()
		created, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "TEST_SECRET",
			Value: "secret-value-123",
		})
		require.NoError(t, err)

		h := &UIHandlers{T: tr, SecretSvc: secretService}

		t.Run("existing secret", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/secrets/"+created.ID+"/edit", nil)
			r.SetPathValue("id", created.ID)
			rr := httptest.NewRecorder()

			h.SecretEdit(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()
			assert.Contains(t, body, "TEST_SECRET")
			assert.Contains(t, body, `name="replace"`)
			// Should not contain the actual secret value
			assert.NotContains(t, body, "secret-value-123")
		})

		t.Run("non-existent secret", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/secrets/non-existent/edit", nil)
			r.SetPathValue("id", "non-existent")
			rr := httptest.NewRecorder()

			h.SecretEdit(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})

		t.Run("missing id", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/secrets//edit", nil)
			rr := httptest.NewRecorder()

			h.SecretEdit(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})
	})
}

func TestUIHandlers_SecretCreate(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		tr := RequireTemplateRenderer(t)
		require.NotNil(t, tr)

		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})

		h := &UIHandlers{T: tr, SecretSvc: secretService}

		t.Run("valid creation", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "TEST_SECRET")
			form.Set("value", "secret-value-123")

			r := httptest.NewRequest(http.MethodPost, "/secrets", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.SecretCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/secrets", res.Header.Get("Hx-Redirect"))
		})

		t.Run("invalid name", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "INVALID@NAME") // contains invalid character
			form.Set("value", "secret-value-123")

			r := httptest.NewRequest(http.MethodPost, "/secrets", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.SecretCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusOK, res.StatusCode) // Returns form with errors
			body := rr.Body.String()
			assert.Contains(t, body, "Use letters, digits, underscore, and hyphens")
			assert.Contains(t, body, "INVALID@NAME") // Preserves form value
		})

		t.Run("empty value", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "TEST_SECRET")
			form.Set("value", "")

			r := httptest.NewRequest(http.MethodPost, "/secrets", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.SecretCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()
			assert.Contains(t, body, "Secret is required")
		})

		t.Run("duplicate name", func(t *testing.T) {
			// First, create a secret
			ctx := context.Background()
			_, err := secretRepo.Create(ctx, model.CreateSecretRequest{
				Name:  "DUPLICATE_SECRET",
				Value: "secret-value-123",
			})
			require.NoError(t, err)

			// Try to create another with the same name
			form := url.Values{}
			form.Set("name", "DUPLICATE_SECRET")
			form.Set("value", "another-secret-value")

			r := httptest.NewRequest(http.MethodPost, "/secrets", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.SecretCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()
			assert.Contains(t, body, "already exists")
		})

		t.Run("dynamic secret provider script failure surfaces details", func(t *testing.T) {
			stubSvc := &stubSecretsService{
				createErr: service.NewSecretProviderScriptError("/opt/scripts/fetch.sh",
					errors.New("script failed (exit 1): connection refused")),
			}
			h.SecretSvc = stubSvc
			t.Cleanup(func() { h.SecretSvc = secretService })

			form := url.Values{}
			form.Set("name", "DYNAMIC_SECRET")
			form.Set("refresh_enabled", "1")
			form.Set("provider_script_path", "/opt/scripts/fetch.sh")
			form.Set("refresh_interval_minutes", "15")

			r := httptest.NewRequest(http.MethodPost, "/secrets", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.SecretCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()
			assert.Contains(t, body, "Refresh script failed during validation")
			assert.Contains(t, body, "connection refused")
		})

		t.Run("empty form submission", func(t *testing.T) {
			// Empty form should trigger validation errors
			form := url.Values{}
			// Don't set any values, should trigger validation errors

			r := httptest.NewRequest(http.MethodPost, "/secrets", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.SecretCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			// UI handlers return form with validation errors
			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()
			assert.Contains(t, body, "Use letters, digits, underscore, and hyphens")
			assert.Contains(t, body, "Secret is required")
		})
	})
}

func TestUIHandlers_SecretUpdate(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		tr := RequireTemplateRenderer(t)
		require.NotNil(t, tr)

		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})

		h := &UIHandlers{T: tr, SecretSvc: secretService}

		// Create a test secret
		ctx := context.Background()
		created, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "TEST_SECRET",
			Value: "original-secret-value",
		})
		require.NoError(t, err)

		t.Run("update name only", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "UPDATED_SECRET")
			// No replace checkbox, so value should not be updated

			r := httptest.NewRequest(http.MethodPost, "/secrets/"+created.ID, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.SetPathValue("id", created.ID)
			rr := httptest.NewRecorder()

			h.SecretUpdate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/secrets", res.Header.Get("Hx-Redirect"))

			// Verify the secret was updated
			updated, err := secretRepo.GetByID(ctx, created.ID)
			require.NoError(t, err)
			assert.Equal(t, "UPDATED_SECRET", updated.Name)
			assert.Equal(t, "original-secret-value", updated.Value) // Value unchanged
		})

		t.Run("replace secret value", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "UPDATED_SECRET")
			form.Set("replace", "on")
			form.Set("value", "new-secret-value")

			r := httptest.NewRequest(http.MethodPost, "/secrets/"+created.ID, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.SetPathValue("id", created.ID)
			rr := httptest.NewRecorder()

			h.SecretUpdate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/secrets", res.Header.Get("Hx-Redirect"))

			// Verify the secret value was updated
			updated, err := secretRepo.GetByID(ctx, created.ID)
			require.NoError(t, err)
			assert.Equal(t, "new-secret-value", updated.Value)
		})

		t.Run("replace with empty value", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "TEST_SECRET")
			form.Set("replace", "on")
			form.Set("value", "") // Empty value when replace is checked

			r := httptest.NewRequest(http.MethodPost, "/secrets/"+created.ID, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.SetPathValue("id", created.ID)
			rr := httptest.NewRecorder()

			h.SecretUpdate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusOK, res.StatusCode) // Returns form with errors
			body := rr.Body.String()
			assert.Contains(t, body, "Secret is required")
		})

		t.Run("invalid name", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "INVALID@NAME") // contains invalid character

			r := httptest.NewRequest(http.MethodPost, "/secrets/"+created.ID, strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.SetPathValue("id", created.ID)
			rr := httptest.NewRecorder()

			h.SecretUpdate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()
			assert.Contains(t, body, "Use letters, digits, underscore, and hyphens")
		})

		t.Run("non-existent secret", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "TEST_SECRET")

			r := httptest.NewRequest(http.MethodPost, "/secrets/non-existent", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.SetPathValue("id", "non-existent")
			rr := httptest.NewRecorder()

			h.SecretUpdate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			// UI handlers return form with error message for service errors
			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()
			// Generic form handler shows database error for non-existent records
			assert.Contains(t, body, "database error")
		})

		t.Run("missing id", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "TEST_SECRET")

			r := httptest.NewRequest(http.MethodPost, "/secrets/", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.SecretUpdate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusNotFound, res.StatusCode)
		})
	})
}

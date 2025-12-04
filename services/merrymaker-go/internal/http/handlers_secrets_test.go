package httpx

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func TestSecretHandlers_Create(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		handlers := &SecretHandlers{Svc: secretService}

		// Test data
		req := model.CreateSecretRequest{
			Name:  "TEST_SECRET",
			Value: "secret-value-123",
		}

		// Create request
		body, err := json.Marshal(req)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/secrets", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		// Execute
		handlers.Create(w, r)

		// Assert
		assert.Equal(t, http.StatusCreated, w.Code)

		var response model.Secret
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, req.Name, response.Name)
		assert.Equal(t, req.Value, response.Value)
		assert.NotEmpty(t, response.ID)
		assert.False(t, response.CreatedAt.IsZero())
		assert.False(t, response.UpdatedAt.IsZero())
	})
}

func TestSecretHandlers_List(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		handlers := &SecretHandlers{Svc: secretService}

		// Create a test secret
		ctx := context.Background()
		_, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "TEST_SECRET",
			Value: "secret-value-123",
		})
		require.NoError(t, err)

		// Create request
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/secrets", nil)

		// Execute
		handlers.List(w, r)

		// Assert
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]any
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		secrets, ok := response["secrets"].([]any)
		require.True(t, ok)
		assert.Len(t, secrets, 1)

		secret := secrets[0].(map[string]any)
		assert.Equal(t, "TEST_SECRET", secret["name"])
		// Value should be empty in list response (comes back as empty string from DB)
		value, exists := secret["value"]
		if exists {
			assert.Empty(t, value)
		}
	})
}

func TestSecretHandlers_GetByID(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		handlers := &SecretHandlers{Svc: secretService}

		// Create a test secret
		ctx := context.Background()
		created, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "TEST_SECRET",
			Value: "secret-value-123",
		})
		require.NoError(t, err)

		// Create request
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/secrets/"+created.ID, nil)
		r.SetPathValue("id", created.ID)

		// Execute
		handlers.GetByID(w, r)

		// Assert
		assert.Equal(t, http.StatusOK, w.Code)

		var response model.Secret
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, created.ID, response.ID)
		assert.Equal(t, created.Name, response.Name)
		assert.Equal(t, created.Value, response.Value)
	})
}

func TestSecretHandlers_Update(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		handlers := &SecretHandlers{Svc: secretService}

		// Create a test secret
		ctx := context.Background()
		created, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "TEST_SECRET",
			Value: "secret-value-123",
		})
		require.NoError(t, err)

		// Update request
		newValue := "updated-secret-value"
		updateReq := model.UpdateSecretRequest{
			Value: &newValue,
		}

		body, err := json.Marshal(updateReq)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/secrets/"+created.ID, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.SetPathValue("id", created.ID)

		// Execute
		handlers.Update(w, r)

		// Assert
		assert.Equal(t, http.StatusOK, w.Code)

		var response model.Secret
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.Equal(t, created.ID, response.ID)
		assert.Equal(t, created.Name, response.Name)
		assert.Equal(t, newValue, response.Value)
	})
}

func TestSecretHandlers_Delete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		handlers := &SecretHandlers{Svc: secretService}

		// Create a test secret
		ctx := context.Background()
		created, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "TEST_SECRET",
			Value: "secret-value-123",
		})
		require.NoError(t, err)

		// Create request
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/secrets/"+created.ID, nil)
		r.SetPathValue("id", created.ID)

		// Execute
		handlers.Delete(w, r)

		// Assert
		assert.Equal(t, http.StatusOK, w.Code)

		var response map[string]bool
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)

		assert.True(t, response["deleted"])

		// Verify secret is deleted
		_, err = secretRepo.GetByID(ctx, created.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestSecretHandlers_Create_ValidationError(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		handlers := &SecretHandlers{Svc: secretService}

		// Test data with validation error
		req := model.CreateSecretRequest{
			Name:  "", // Empty name should cause validation error
			Value: "secret-value-123",
		}

		// Create request
		body, err := json.Marshal(req)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/secrets", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		// Execute
		handlers.Create(w, r)

		// Assert - should return 400 for validation error
		assert.Equal(t, http.StatusBadRequest, w.Code)

		var response map[string]string
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "validation_failed", response["error"])
	})
}

func TestSecretHandlers_Create_NameConflict(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		handlers := &SecretHandlers{Svc: secretService}

		// Create first secret
		ctx := context.Background()
		_, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:  "DUPLICATE_NAME",
			Value: "secret-value-123",
		})
		require.NoError(t, err)

		// Try to create second secret with same name
		req := model.CreateSecretRequest{
			Name:  "DUPLICATE_NAME",
			Value: "different-value",
		}

		body, err := json.Marshal(req)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/secrets", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		// Execute
		handlers.Create(w, r)

		// Assert - should return 409 for name conflict
		assert.Equal(t, http.StatusConflict, w.Code)

		var response map[string]string
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		assert.Equal(t, "name_conflict", response["error"])
	})
}

func TestSecretHandlers_List_PaginationLimits(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo})
		handlers := &SecretHandlers{Svc: secretService}

		tests := []struct {
			name           string
			limitParam     string
			offsetParam    string
			expectedLimit  int
			expectedOffset int
		}{
			{
				name:           "negative limit clamped to 1",
				limitParam:     "-5",
				offsetParam:    "0",
				expectedLimit:  1,
				expectedOffset: 0,
			},
			{
				name:           "zero limit clamped to 1",
				limitParam:     "0",
				offsetParam:    "0",
				expectedLimit:  1,
				expectedOffset: 0,
			},
			{
				name:           "excessive limit clamped to max",
				limitParam:     "500",
				offsetParam:    "0",
				expectedLimit:  100, // maxSecretListLimit
				expectedOffset: 0,
			},
			{
				name:           "negative offset clamped to 0",
				limitParam:     "10",
				offsetParam:    "-5",
				expectedLimit:  10,
				expectedOffset: 0,
			},
			{
				name:           "valid parameters unchanged",
				limitParam:     "25",
				offsetParam:    "10",
				expectedLimit:  25,
				expectedOffset: 10,
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				// Create request with query parameters
				url := "/api/secrets?limit=" + tt.limitParam + "&offset=" + tt.offsetParam
				w := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodGet, url, nil)

				// Execute
				handlers.List(w, r)

				// Assert
				assert.Equal(t, http.StatusOK, w.Code)

				var response map[string]any
				err := json.Unmarshal(w.Body.Bytes(), &response)
				require.NoError(t, err)

				assert.Equal(t, tt.expectedLimit, int(response["limit"].(float64)))
				assert.Equal(t, tt.expectedOffset, int(response["offset"].(float64)))
			})
		}
	})
}

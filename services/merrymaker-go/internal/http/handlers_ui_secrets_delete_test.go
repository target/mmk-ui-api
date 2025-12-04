package httpx

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUIHandlers_SecretDelete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup
		tr := RequireTemplateRenderer(t)
		require.NotNil(t, tr)

		encryptor := &cryptoutil.NoopEncryptor{}
		secretRepo := data.NewSecretRepo(db, encryptor)
		secretService := service.MustNewSecretService(service.SecretServiceOptions{
			Repo: secretRepo,
		})

		h := &UIHandlers{
			T:         tr,
			SecretSvc: secretService,
		}

		ctx := context.Background()

		t.Run("successful deletion", func(t *testing.T) {
			// Create a secret
			created, err := secretService.Create(ctx, model.CreateSecretRequest{
				Name:  "DELETE_ME",
				Value: "secret-value",
			})
			require.NoError(t, err)

			// Create HTMX delete request
			r := httptest.NewRequest(http.MethodPost, "/secrets/"+created.ID+"/delete", nil)
			r.SetPathValue("id", created.ID)
			r.Header.Set("Hx-Request", "true") // Mark as HTMX request
			rr := httptest.NewRecorder()

			// Execute
			h.SecretDelete(rr, r)

			// Verify successful deletion (HTMX uses 200 OK with empty content and success toast)
			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusOK, res.StatusCode)
			assert.Contains(t, res.Header.Get("Hx-Trigger"), "showToast")

			// Verify secret is deleted
			_, err = secretService.GetByID(ctx, created.ID)
			assert.Error(t, err)
		})

		t.Run("non-existent secret", func(t *testing.T) {
			// Try to delete non-existent secret
			r := httptest.NewRequest(http.MethodPost, "/secrets/non-existent/delete", nil)
			r.SetPathValue("id", "non-existent")
			rr := httptest.NewRecorder()

			// Execute
			h.SecretDelete(rr, r)

			// Should render error page
			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Contains(t, rr.Body.String(), "Secrets")
		})

		t.Run("missing id", func(t *testing.T) {
			// Try to delete without ID
			r := httptest.NewRequest(http.MethodPost, "/secrets//delete", nil)
			rr := httptest.NewRecorder()

			// Execute
			h.SecretDelete(rr, r)

			// Should return 404
			assert.Equal(t, http.StatusNotFound, rr.Code)
		})

		t.Run("delete fails due to foreign key constraint (HTMX)", func(t *testing.T) {
			// Create a secret
			secret, err := secretService.Create(ctx, model.CreateSecretRequest{
				Name:  "USED_SECRET",
				Value: "secret-value",
			})
			require.NoError(t, err)

			// Create a source that uses this secret to establish FK constraint
			sourceRepo := data.NewSourceRepo(db)
			jobRepo := data.NewJobRepo(db, data.RepoConfig{})
			sourceService := service.NewSourceService(service.SourceServiceOptions{
				SourceRepo: sourceRepo,
				Jobs:       jobRepo,
				SecretRepo: secretRepo,
			})
			_, err = sourceService.Create(ctx, &model.CreateSourceRequest{
				Name:    "test-source",
				Value:   "console.log('test');",
				Secrets: []string{secret.Name}, // Use secret name, not ID
			})
			require.NoError(t, err)

			// Create HTMX delete request
			r := httptest.NewRequest(http.MethodPost, "/secrets/"+secret.ID+"/delete", nil)
			r.SetPathValue("id", secret.ID)
			r.Header.Set("Hx-Request", "true") // Mark as HTMX request
			rr := httptest.NewRecorder()

			// Execute
			h.SecretDelete(rr, r)

			// Should return 204 No Content (prevents swap) and trigger toast
			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })

			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Contains(t, res.Header.Get("Hx-Trigger"), "showToast")

			// Verify secret still exists (wasn't deleted)
			_, err = secretService.GetByID(ctx, secret.ID)
			assert.NoError(t, err)
		})
	})
}

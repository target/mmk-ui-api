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
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func newHTTPAlertSinksHandlerForTest(t *testing.T, db *sql.DB) *HTTPAlertSinkHandlers {
	t.Helper()

	repo := data.NewHTTPAlertSinkRepo(db)
	svc := service.MustNewHTTPAlertSinkService(service.HTTPAlertSinkServiceOptions{Repo: repo})
	return &HTTPAlertSinkHandlers{Svc: svc}
}

func TestHTTPAlertSinkHandlers_Create(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newHTTPAlertSinksHandlerForTest(t, db)

		ok := 201
		retry := 2
		req := &model.CreateHTTPAlertSinkRequest{
			Name:     "alerts-primary",
			URI:      "https://example.com/webhook",
			Method:   "POST",
			OkStatus: &ok,
			Retry:    &retry,
			Secrets:  []string{"API_KEY"},
		}

		body, err := json.Marshal(req)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/http-alert-sinks", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		h.Create(w, r)

		require.Equal(t, http.StatusCreated, w.Code)

		var got model.HTTPAlertSink
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, req.Name, got.Name)
		assert.Equal(t, req.URI, got.URI)
		assert.Equal(t, "POST", got.Method)
		assert.Equal(t, *req.OkStatus, got.OkStatus)
		assert.Equal(t, *req.Retry, got.Retry)
		assert.Contains(t, got.Secrets, "API_KEY")
		assert.NotEmpty(t, got.ID)
		assert.False(t, got.CreatedAt.IsZero())
	})
}

func TestHTTPAlertSinkHandlers_List(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newHTTPAlertSinksHandlerForTest(t, db)

		// Create one sink first
		repo := data.NewHTTPAlertSinkRepo(db)
		_, err := repo.Create(context.Background(), &model.CreateHTTPAlertSinkRequest{
			Name:   "alerts-list-1",
			URI:    "https://example.com/a",
			Method: "GET",
		})
		require.NoError(t, err)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/http-alert-sinks", nil)

		h.List(w, r)

		require.Equal(t, http.StatusOK, w.Code)

		var resp map[string]any
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		sinks, ok := resp["http_alert_sinks"].([]any)
		require.True(t, ok)
		assert.GreaterOrEqual(t, len(sinks), 1)
	})
}

func TestHTTPAlertSinkHandlers_GetByID_Update_Delete(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newHTTPAlertSinksHandlerForTest(t, db)
		repo := data.NewHTTPAlertSinkRepo(db)

		created, err := repo.Create(context.Background(), &model.CreateHTTPAlertSinkRequest{
			Name:   "alerts-get-1",
			URI:    "https://example.com/get",
			Method: "POST",
		})
		require.NoError(t, err)

		// GetByID
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/http-alert-sinks/"+created.ID, nil)
		r.SetPathValue("id", created.ID)
		h.GetByID(w, r)
		require.Equal(t, http.StatusOK, w.Code)
		var got model.HTTPAlertSink
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, created.ID, got.ID)

		// Update name
		newName := "alerts-updated"
		upd := model.UpdateHTTPAlertSinkRequest{Name: &newName}
		b, _ := json.Marshal(upd)
		w = httptest.NewRecorder()
		r = httptest.NewRequest(http.MethodPut, "/api/http-alert-sinks/"+created.ID, bytes.NewReader(b))
		r.Header.Set("Content-Type", "application/json")
		r.SetPathValue("id", created.ID)
		h.Update(w, r)
		require.Equal(t, http.StatusOK, w.Code)
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
		assert.Equal(t, newName, got.Name)

		// Delete
		w = httptest.NewRecorder()
		r = httptest.NewRequest(http.MethodDelete, "/api/http-alert-sinks/"+created.ID, nil)
		r.SetPathValue("id", created.ID)
		h.Delete(w, r)
		require.Equal(t, http.StatusOK, w.Code)
		var delResp map[string]bool
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &delResp))
		assert.True(t, delResp["deleted"])

		// Verify not found
		_, err = repo.GetByID(context.Background(), created.ID)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestHTTPAlertSinkHandlers_Create_ValidationError_And_NameConflict(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newHTTPAlertSinksHandlerForTest(t, db)
		repo := data.NewHTTPAlertSinkRepo(db)

		// Validation error (invalid URL)
		bad := &model.CreateHTTPAlertSinkRequest{
			Name:   "a",
			URI:    "::::",
			Method: "POST",
		}
		body, _ := json.Marshal(bad)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/http-alert-sinks", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		h.Create(w, r)
		assert.Equal(t, http.StatusBadRequest, w.Code)

		// Name conflict
		_, err := repo.Create(context.Background(), &model.CreateHTTPAlertSinkRequest{
			Name:   "dup-name",
			URI:    "https://example.com/x",
			Method: "GET",
		})
		require.NoError(t, err)

		conf := &model.CreateHTTPAlertSinkRequest{
			Name:   "dup-name",
			URI:    "https://example.com/y",
			Method: "POST",
		}
		body, _ = json.Marshal(conf)
		w = httptest.NewRecorder()
		r = httptest.NewRequest(http.MethodPost, "/api/http-alert-sinks", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		h.Create(w, r)
		assert.Equal(t, http.StatusConflict, w.Code)
	})
}

func TestHTTPAlertSinkHandlers_List_PaginationLimits(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newHTTPAlertSinksHandlerForTest(t, db)

		tests := []struct {
			name           string
			limitParam     string
			offsetParam    string
			expectedLimit  int
			expectedOffset int
		}{
			{"negative limit clamped to 1", "-5", "0", 1, 0},
			{"zero limit clamped to 1", "0", "0", 1, 0},
			{"excessive limit clamped to max", "500", "0", 100, 0},
			{"negative offset clamped to 0", "10", "-5", 10, 0},
			{"valid parameters unchanged", "25", "10", 25, 10},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				url := "/api/http-alert-sinks?limit=" + tt.limitParam + "&offset=" + tt.offsetParam
				w := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodGet, url, nil)
				h.List(w, r)
				assert.Equal(t, http.StatusOK, w.Code)
				var response map[string]any
				require.NoError(t, json.Unmarshal(w.Body.Bytes(), &response))
				assert.Equal(t, tt.expectedLimit, int(response["limit"].(float64)))
				assert.Equal(t, tt.expectedOffset, int(response["offset"].(float64)))
			})
		}
	})
}

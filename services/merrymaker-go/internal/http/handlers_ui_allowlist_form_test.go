package httpx

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAllowlistUIHandlerForTest(t *testing.T, db *sql.DB) *UIHandlers {
	t.Helper()
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr)

	// Repo
	allowlistRepo := data.NewDomainAllowlistRepo(db)

	// Service
	allowlistSvc := service.NewDomainAllowlistService(service.DomainAllowlistServiceOptions{
		Repo: allowlistRepo,
	})

	return &UIHandlers{T: tr, AllowlistSvc: allowlistSvc}
}

// uniquePattern generates a unique domain pattern for testing.
func uniquePattern(prefix string) string {
	return fmt.Sprintf("%s-%d.com", prefix, time.Now().UnixNano())
}

func TestAllowlistNew(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newAllowlistUIHandlerForTest(t, db)

		req := httptest.NewRequest(http.MethodGet, "/allowlist/new", nil)
		w := httptest.NewRecorder()

		h.AllowlistNew(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "Domain Allow List")
		assert.Contains(t, body, `name="pattern"`)
		assert.Contains(t, body, `name="pattern_type"`)
	})
}

func TestAllowlistCreate_Success(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newAllowlistUIHandlerForTest(t, db)

		form := url.Values{
			"pattern":      {uniquePattern("create-test")},
			"pattern_type": {"exact"},
			"scope":        {"global"},
			"description":  {"Test allowlist entry"},
			"priority":     {"100"},
			"enabled":      {"on"},
		}

		req := httptest.NewRequest(http.MethodPost, "/allowlist", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		h.AllowlistCreate(w, req)

		if w.Code != http.StatusNoContent {
			t.Logf("Response body: %s", w.Body.String())
		}
		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "/allowlist", w.Header().Get("Hx-Redirect"))
	})
}

func TestAllowlistCreate_ValidationError(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newAllowlistUIHandlerForTest(t, db)

		form := url.Values{
			"pattern":      {""}, // Empty pattern should fail validation
			"pattern_type": {"exact"},
			"priority":     {"100"},
		}

		req := httptest.NewRequest(http.MethodPost, "/allowlist", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		w := httptest.NewRecorder()

		h.AllowlistCreate(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, "Domain pattern is required")
	})
}

func TestAllowlistEdit(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newAllowlistUIHandlerForTest(t, db)

		// Create a test allowlist entry
		pattern := uniquePattern("edit-test")
		req := &model.CreateDomainAllowlistRequest{
			Pattern:     pattern,
			PatternType: "exact",
			Scope:       "global",
			Description: "Test entry",
			Enabled:     &[]bool{true}[0],
			Priority:    &[]int{100}[0],
		}
		allowlist, err := h.AllowlistSvc.Create(context.Background(), req)
		require.NoError(t, err)

		// Test edit form
		editReq := httptest.NewRequest(http.MethodGet, "/allowlist/"+allowlist.ID+"/edit", nil)
		editReq.SetPathValue("id", allowlist.ID)
		w := httptest.NewRecorder()

		h.AllowlistEdit(w, editReq)

		assert.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, pattern)
		assert.Contains(t, body, "Test entry")
	})
}

func TestAllowlistUpdate_Success(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newAllowlistUIHandlerForTest(t, db)

		// Create a test allowlist entry
		req := &model.CreateDomainAllowlistRequest{
			Pattern:     uniquePattern("update-test"),
			PatternType: "exact",
			Scope:       "global",
			Description: "Test entry",
			Enabled:     &[]bool{true}[0],
			Priority:    &[]int{100}[0],
		}
		allowlist, err := h.AllowlistSvc.Create(context.Background(), req)
		require.NoError(t, err)

		// Update the entry
		updatedPattern := uniquePattern("updated")
		form := url.Values{
			"pattern":      {updatedPattern},
			"pattern_type": {"wildcard"},
			"scope":        {"global"}, // Must remain global for global entries
			"description":  {"Updated description"},
			"priority":     {"200"},
			"enabled":      {"on"},
		}

		updateReq := httptest.NewRequest(http.MethodPost, "/allowlist/"+allowlist.ID, strings.NewReader(form.Encode()))
		updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		updateReq.SetPathValue("id", allowlist.ID)
		w := httptest.NewRecorder()

		h.AllowlistUpdate(w, updateReq)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "/allowlist", w.Header().Get("Hx-Redirect"))
	})
}

func TestAllowlistDelete_Success(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		h := newAllowlistUIHandlerForTest(t, db)

		// Create a test allowlist entry
		req := &model.CreateDomainAllowlistRequest{
			Pattern:     uniquePattern("delete-test"),
			PatternType: "exact",
			Scope:       "global",
			Description: "Test entry",
			Enabled:     &[]bool{true}[0],
			Priority:    &[]int{100}[0],
		}
		allowlist, err := h.AllowlistSvc.Create(context.Background(), req)
		require.NoError(t, err)

		// Delete the entry
		deleteReq := httptest.NewRequest(http.MethodPost, "/allowlist/"+allowlist.ID+"/delete", nil)
		deleteReq.SetPathValue("id", allowlist.ID)
		w := httptest.NewRecorder()

		h.AllowlistDelete(w, deleteReq)

		assert.Equal(t, http.StatusNoContent, w.Code)
		assert.Equal(t, "/allowlist", w.Header().Get("Hx-Redirect"))
	})
}

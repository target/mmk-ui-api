package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/stretchr/testify/assert"
)

func TestUIHandlers_NotFound_BrowserRequest_Unauthenticated(t *testing.T) {
	// Create template renderer
	tr := RequireTemplateRenderer(t)
	if tr == nil {
		return
	}

	handlers := &UIHandlers{T: tr}

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Header.Set("Accept", "text/html")

	// Apply browser detection middleware
	ctx := context.WithValue(req.Context(), browserRequestKey{}, true)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handlers.NotFound(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")

	body := w.Body.String()
	assert.Contains(t, body, "404")
	assert.Contains(t, body, "Page Not Found")
	assert.Contains(t, body, "/auth/login") // Should show login link for unauthenticated users
}

func TestUIHandlers_NotFound_BrowserRequest_Authenticated(t *testing.T) {
	// Create template renderer
	tr := RequireTemplateRenderer(t)
	if tr == nil {
		return
	}

	handlers := &UIHandlers{T: tr}

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Header.Set("Accept", "text/html")

	// Apply browser detection middleware and add session
	ctx := context.WithValue(req.Context(), browserRequestKey{}, true)
	session := &domainauth.Session{
		ID:        "test-session",
		UserID:    "test-user",
		Email:     "test@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	}
	ctx = SetSessionInContext(ctx, session)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handlers.NotFound(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")

	body := w.Body.String()
	assert.Contains(t, body, "404")
	assert.Contains(t, body, "Page Not Found")
	// Should not show login link for authenticated users
	assert.NotContains(t, body, "/auth/login")
}

func TestUIHandlers_NotFound_APIRequest(t *testing.T) {
	// Template renderer not needed for API requests
	handlers := &UIHandlers{T: nil}

	req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
	req.Header.Set("Accept", "application/json")

	// Apply browser detection middleware
	ctx := context.WithValue(req.Context(), browserRequestKey{}, false)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handlers.NotFound(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")

	body := w.Body.String()
	assert.Contains(t, body, "not_found")
	assert.Contains(t, body, "not found")
}

func TestUIHandlers_NotFound_BrowserRequest_TemplateError(t *testing.T) {
	// Use invalid template renderer to test fallback
	handlers := &UIHandlers{T: nil}

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	req.Header.Set("Accept", "text/html")

	// Apply browser detection middleware
	ctx := context.WithValue(req.Context(), browserRequestKey{}, true)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handlers.NotFound(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)

	body := w.Body.String()
	assert.Contains(t, body, "Page not found")
}

func TestNotFoundHandler_Integration(t *testing.T) {
	// Create a simple mux with one route
	mux := http.NewServeMux()
	mux.HandleFunc("GET /existing", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Create template renderer
	tr := RequireTemplateRenderer(t)
	if tr == nil {
		return
	}

	uiHandlers := &UIHandlers{T: tr}

	// Create the notFoundHandler
	handler := &notFoundHandler{
		mux:        mux,
		uiHandlers: uiHandlers,
	}

	// Wrap with browser detection
	wrappedHandler := BrowserDetection()(handler)

	t.Run("existing route", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/existing", nil)
		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "OK", w.Body.String())
	})

	t.Run("non-existing route - browser", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
		req.Header.Set("Accept", "text/html")
		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
		assert.Contains(t, w.Body.String(), "404")
	})

	t.Run("non-existing route - API", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/nonexistent", nil)
		req.Header.Set("Accept", "application/json")
		w := httptest.NewRecorder()

		wrappedHandler.ServeHTTP(w, req)

		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
		assert.Contains(t, w.Body.String(), "not_found")
	})
}

func TestCaptureWriter_StatusCapture(t *testing.T) {
	w := httptest.NewRecorder()
	cw := newCaptureWriter(w)

	// Test default status
	assert.Equal(t, http.StatusOK, cw.status)

	// Test status capture
	cw.WriteHeader(http.StatusNotFound)
	assert.Equal(t, http.StatusNotFound, cw.status)

	// Underlying writer not written until flush
	cw.flushTo(w)
	assert.Equal(t, http.StatusNotFound, w.Code)
}

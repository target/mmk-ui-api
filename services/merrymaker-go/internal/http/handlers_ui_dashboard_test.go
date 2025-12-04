package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUIHandlers_FullPage_Renders(t *testing.T) {
	// Create template renderer
	tr, err := NewTemplateRenderer(TemplateRendererConfig{
		TemplateFS: os.DirFS("../../frontend/templates"),
	})
	if err != nil {
		t.Skipf("Templates not available, skipping test: %v", err)
		return
	}

	handlers := &UIHandlers{T: tr}

	tests := []struct {
		name         string
		path         string
		handler      func(http.ResponseWriter, *http.Request)
		wantContains []string
	}{
		{
			name:         "Index full page",
			path:         "/",
			handler:      handlers.Index,
			wantContains: []string{"dashboard-layout", "sidebar", "main-content", "Dashboard", "stats-grid"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Accept", "text/html")
			ctx := context.WithValue(req.Context(), browserRequestKey{}, true)
			req = req.WithContext(ctx)
			w := httptest.NewRecorder()
			tt.handler(w, req)
			assert.Equal(t, http.StatusOK, w.Code)
			assert.Contains(t, w.Header().Get("Content-Type"), "text/html")
			body := w.Body.String()
			for _, s := range tt.wantContains {
				assert.Contains(t, body, s)
			}
		})
	}
}

func TestUIHandlers_Dashboard_Redirect(t *testing.T) {
	// Create template renderer
	tr, err := NewTemplateRenderer(TemplateRendererConfig{
		TemplateFS: os.DirFS("../../frontend/templates"),
	})
	if err != nil {
		t.Skipf("Templates not available, skipping test: %v", err)
		return
	}

	handlers := &UIHandlers{T: tr}

	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Header.Set("Accept", "text/html")
	ctx := context.WithValue(req.Context(), browserRequestKey{}, true)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handlers.Dashboard(w, req)

	// Dashboard should redirect to / with 301 Moved Permanently
	assert.Equal(t, http.StatusMovedPermanently, w.Code)
	assert.Equal(t, "/", w.Header().Get("Location"))
}

func TestUIHandlers_Index_HTMXPartial(t *testing.T) {
	// Create template renderer
	tr := RequireTemplateRenderer(t)
	if tr == nil {
		return
	}

	handlers := &UIHandlers{T: tr}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept", "text/html")
	req.Header.Set("Hx-Request", "true") // HTMX request

	// Apply browser detection middleware
	ctx := context.WithValue(req.Context(), browserRequestKey{}, true)
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handlers.Index(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "text/html")

	body := w.Body.String()
	// Should contain dashboard content but not the full layout
	assert.Contains(t, body, "stats-grid")
	assert.Contains(t, body, "recent-browser-jobs-panel")
	// Should NOT contain the full layout elements
	assert.NotContains(t, body, "dashboard-layout")
	assert.NotContains(t, body, "sidebar")
}

func TestUIHandlers_WantsPartial_Logic(t *testing.T) {
	tests := []struct {
		name           string
		headers        map[string]string
		expectedResult bool
	}{
		{
			name:           "Regular request",
			headers:        map[string]string{},
			expectedResult: false,
		},
		{
			name: "HTMX request",
			headers: map[string]string{
				"Hx-Request": "true",
			},
			expectedResult: true,
		},
		{
			name: "HTMX history restore",
			headers: map[string]string{
				"Hx-Request":                 "true",
				"Hx-History-Restore-Request": "true",
			},
			expectedResult: true, // Still partial on history restore
		},
		{
			name: "Boosted request",
			headers: map[string]string{
				"Hx-Request": "true",
				"Hx-Boosted": "true",
			},
			expectedResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			result := WantsPartial(req)
			assert.Equal(t, tt.expectedResult, result)
		})
	}
}

func TestUIHandlers_NavigationActiveStates(t *testing.T) {
	// Create template renderer
	tr := RequireTemplateRenderer(t)
	if tr == nil {
		return
	}

	handlers := &UIHandlers{T: tr}

	tests := []struct {
		path         string
		handler      func(w http.ResponseWriter, r *http.Request)
		expectedPage string
	}{
		{"/", handlers.Index, PageHome},
		{"/alerts", handlers.Alerts, PageAlerts},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Accept", "text/html")

			// Apply browser detection middleware
			ctx := context.WithValue(req.Context(), browserRequestKey{}, true)
			req = req.WithContext(ctx)

			w := httptest.NewRecorder()
			tt.handler(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			body := w.Body.String()
			// Check that the correct navigation item is marked as active
			expectedActiveClass := `class="nav-link is-active"`
			assert.Contains(t, body, expectedActiveClass)

			// Count how many active nav links there are (should be exactly 1)
			activeCount := strings.Count(body, expectedActiveClass)
			assert.Equal(t, 1, activeCount, "Should have exactly one active navigation item")
		})
	}
}

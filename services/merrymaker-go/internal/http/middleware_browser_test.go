package httpx

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBrowserDetection(t *testing.T) {
	middleware := BrowserDetection()

	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		isBrowser := IsBrowserRequest(r)
		if isBrowser {
			w.Header().Set("Content-Type", "text/html")
		} else {
			w.Header().Set("Content-Type", "application/json")
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware(testHandler)

	tests := []struct {
		name            string
		path            string
		acceptHeader    string
		htmxHeader      string
		expectedBrowser bool
	}{
		{
			name:            "API route with JSON accept",
			path:            "/api/users",
			acceptHeader:    "application/json",
			expectedBrowser: false,
		},
		{
			name:            "API route with HTML accept",
			path:            "/api/users",
			acceptHeader:    "text/html",
			expectedBrowser: false, // API routes are never browser requests
		},
		{
			name:            "Static asset",
			path:            "/static/css/styles.css",
			acceptHeader:    "text/css",
			expectedBrowser: false,
		},
		{
			name:            "Browser request with HTML accept",
			path:            "/dashboard",
			acceptHeader:    "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
			expectedBrowser: true,
		},
		{
			name:            "HTMX request",
			path:            "/dashboard",
			acceptHeader:    "text/html",
			htmxHeader:      "true",
			expectedBrowser: true,
		},
		{
			name:            "Root path with HTML accept",
			path:            "/",
			acceptHeader:    "text/html",
			expectedBrowser: true,
		},
		{
			name:            "No accept header on non-API route",
			path:            "/dashboard",
			acceptHeader:    "",
			expectedBrowser: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}
			if tt.htmxHeader != "" {
				req.Header.Set("Hx-Request", tt.htmxHeader)
			}

			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if tt.expectedBrowser {
				assert.Equal(t, "text/html", w.Header().Get("Content-Type"))
			} else {
				assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
			}
		})
	}
}

func TestIsBrowserRequest_WithoutMiddleware(t *testing.T) {
	tests := []struct {
		name            string
		path            string
		acceptHeader    string
		expectedBrowser bool
	}{
		{
			name:            "API route",
			path:            "/api/users",
			acceptHeader:    "application/json",
			expectedBrowser: false,
		},
		{
			name:            "Browser request",
			path:            "/dashboard",
			acceptHeader:    "text/html",
			expectedBrowser: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.acceptHeader != "" {
				req.Header.Set("Accept", tt.acceptHeader)
			}

			result := IsBrowserRequest(req)
			assert.Equal(t, tt.expectedBrowser, result)
		})
	}
}

func TestIsBrowserRequest_WithContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/test", nil)

	// Test with context value set to true
	ctx := context.WithValue(req.Context(), browserRequestKey{}, true)
	req = req.WithContext(ctx)
	assert.True(t, IsBrowserRequest(req))

	// Test with context value set to false
	ctx = context.WithValue(req.Context(), browserRequestKey{}, false)
	req = req.WithContext(ctx)
	assert.False(t, IsBrowserRequest(req))

	// Test with invalid context value type
	ctx = context.WithValue(req.Context(), browserRequestKey{}, "invalid")
	req = req.WithContext(ctx)
	// Should fallback to direct detection
	req.Header.Set("Accept", "text/html")
	assert.True(t, IsBrowserRequest(req))
}

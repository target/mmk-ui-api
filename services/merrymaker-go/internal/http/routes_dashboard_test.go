package httpx

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestDashboardRoutes_Integration(t *testing.T) {
	// Skip tests if templates are not available from the expected location
	SkipIfNoTemplates(t)

	// Create mock for SecretService
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSecretRepo := mocks.NewMockSecretRepository(ctrl)

	// Create mock HTTP alert sink repo
	mockHTTPAlertSinkRepo := mocks.NewMockHTTPAlertSinkRepository(ctrl)

	// Create mock repositories using the same controller
	mockJobRepo := mocks.NewMockJobRepository(ctrl)
	mockEventRepo := mocks.NewMockEventRepository(ctrl)

	// Create a minimal router with UI handlers
	services := RouterServices{
		Jobs: service.MustNewJobService(service.JobServiceOptions{Repo: mockJobRepo, DefaultLease: 30 * time.Second}),
		Events: service.MustNewEventService(service.EventServiceOptions{
			Repo:   mockEventRepo,
			Config: service.DefaultEventServiceConfig(),
		}),
		Secrets: service.MustNewSecretService(service.SecretServiceOptions{Repo: mockSecretRepo}),
		HTTPAlertSinks: service.MustNewHTTPAlertSinkService(
			service.HTTPAlertSinkServiceOptions{Repo: mockHTTPAlertSinkRepo},
		),
		Sources: &service.SourceService{},
		Auth:    nil, // No auth for this test
	}

	// Create a custom router that uses the correct template path for tests
	mux := http.NewServeMux()

	// Register API routes (these don't need templates)
	jobHandlers := &JobHandlers{Svc: services.Jobs}
	eventHandlers := &EventHandlers{Svc: services.Events}
	secretHandlers := &SecretHandlers{Svc: services.Secrets}
	alertHandlers := &HTTPAlertSinkHandlers{Svc: services.HTTPAlertSinks}
	sourceHandlers := &SourceHandlers{Svc: services.Sources}

	registerJobRoutes(mux, jobHandlers)
	registerEventRoutes(mux, eventHandlers)
	registerSecretRoutes(mux, secretHandlers, services.Auth)
	registerAlertSinkRoutes(mux, alertHandlers, services.Auth)
	registerSourceRoutes(mux, sourceHandlers)

	// Static assets (dev mode for tests)
	mux.Handle("GET /static/", staticWithFallback(true))

	// UI routes with template renderer using test-appropriate path
	uiHandlers := CreateUIHandlersForTest(t)
	if uiHandlers == nil {
		return
	}
	registerUIRoutes(mux, uiHandlers, uiRouteConfig{Auth: nil, CookieDomain: ""})

	// Wrap with NotFound handler and browser detection middleware
	handler := &notFoundHandler{
		mux:        mux,
		uiHandlers: uiHandlers,
	}

	// Apply browser detection middleware
	router := BrowserDetection()(handler)

	tests := []struct {
		name           string
		path           string
		method         string
		headers        map[string]string
		expectedStatus int
		expectedBody   []string
		notExpected    []string
	}{
		{
			name:           "Home page full load (now shows dashboard)",
			path:           "/",
			method:         http.MethodGet,
			headers:        map[string]string{"Accept": "text/html"},
			expectedStatus: http.StatusOK,
			expectedBody: []string{
				"dashboard-layout",
				"sidebar",
				"main-content",
				"Dashboard",
				"stats-grid",
				"recent-browser-jobs-panel",
			},
		},
		{
			name:           "Dashboard route redirects to home",
			path:           "/dashboard",
			method:         http.MethodGet,
			headers:        map[string]string{"Accept": "text/html"},
			expectedStatus: http.StatusMovedPermanently,
			expectedBody:   []string{}, // Redirect response, no body check needed
		},
		{
			name:           "Alerts page full load",
			path:           "/alerts",
			method:         http.MethodGet,
			headers:        map[string]string{"Accept": "text/html"},
			expectedStatus: http.StatusOK,
			expectedBody:   []string{"dashboard-layout", "sidebar", "main-content", "Alerts"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, tt.expectedStatus, w.Code, "Status code mismatch for %s", tt.path)

			// For redirect responses, verify Location header
			if tt.expectedStatus == http.StatusMovedPermanently || tt.expectedStatus == http.StatusFound {
				assert.Equal(t, "/", w.Header().Get("Location"), "Redirect location mismatch for %s", tt.path)
			}

			body := w.Body.String()
			for _, expected := range tt.expectedBody {
				assert.Contains(t, body, expected, "Expected content missing for %s: %s", tt.path, expected)
			}

			for _, notExpected := range tt.notExpected {
				assert.NotContains(t, body, notExpected, "Unexpected content found for %s: %s", tt.path, notExpected)
			}
		})
	}
}

func TestDashboardRoutes_NavigationActiveStates(t *testing.T) {
	// Skip tests if templates are not available
	SkipIfNoTemplates(t)

	// Create a custom router that uses the correct template path for tests
	mux := http.NewServeMux()

	// UI routes with template renderer using test-appropriate path
	uiHandlers := CreateUIHandlersForTest(t)
	if uiHandlers == nil {
		return
	}
	registerUIRoutes(mux, uiHandlers, uiRouteConfig{Auth: nil, CookieDomain: ""})

	// Wrap with NotFound handler and browser detection middleware
	handler := &notFoundHandler{
		mux:        mux,
		uiHandlers: uiHandlers,
	}

	// Apply browser detection middleware
	router := BrowserDetection()(handler)

	tests := []struct {
		path         string
		expectedPage string
	}{
		{"/", "home"}, // Home page now shows dashboard content
		{"/alerts", "alerts"},
		{"/alert-sinks", "alert-sinks"},
		{"/sources", "sources"},
		{"/sites", "sites"},
		{"/secrets", "secrets"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			req.Header.Set("Accept", "text/html")

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			body := w.Body.String()
			// Verify that exactly one navigation item is marked as active and it's the expected one
			// Count occurrences of is-active
			activeCount := strings.Count(body, `class="nav-link is-active"`)
			assert.Equal(t, 1, activeCount, "Should have exactly one active navigation item")

			// Now verify that the expected nav link has is-active within its <a> tag
			var href string
			if tt.expectedPage == "home" {
				href = "/"
			} else {
				href = "/" + tt.expectedPage
			}
			hrefToken := fmt.Sprintf(`href="%s"`, href)
			i := strings.Index(body, hrefToken)
			if i == -1 {
				t.Fatalf("expected href not found: %s", hrefToken)
			}
			// Find end of anchor
			j := strings.Index(body[i:], "</a>")
			if j == -1 {
				t.Fatalf("closing </a> not found after href: %s", hrefToken)
			}
			anchor := body[i : i+j]
			assert.Contains(t, anchor, `class="nav-link is-active"`, "expected nav link should be active: %s", href)
		})
	}
}

func TestDashboardRoutes_HTMXHeaders(t *testing.T) {
	// Skip tests if templates are not available
	SkipIfNoTemplates(t)

	// Create a custom router that uses the correct template path for tests
	mux := http.NewServeMux()

	// UI routes with template renderer using test-appropriate path
	uiHandlers := CreateUIHandlersForTest(t)
	if uiHandlers == nil {
		return
	}
	registerUIRoutes(mux, uiHandlers, uiRouteConfig{Auth: nil, CookieDomain: ""})

	// Wrap with NotFound handler and browser detection middleware
	handler := &notFoundHandler{
		mux:        mux,
		uiHandlers: uiHandlers,
	}

	// Apply browser detection middleware
	router := BrowserDetection()(handler)

	t.Run("HTMX history restore should return partial (home page)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Hx-Request", "true")
		req.Header.Set("Hx-History-Restore-Request", "true")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		body := w.Body.String()
		// Should return partial content even for history restore
		assert.Contains(t, body, "stats-grid")
		assert.NotContains(t, body, "dashboard-layout")
		assert.NotContains(t, body, "sidebar")
	})

	t.Run("Regular HTMX request should return partial (home page)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Header.Set("Accept", "text/html")
		req.Header.Set("Hx-Request", "true")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)

		body := w.Body.String()
		// Should return partial content only
		assert.Contains(t, body, "stats-grid")
		assert.NotContains(t, body, "dashboard-layout")
		assert.NotContains(t, body, "sidebar")
	})
}

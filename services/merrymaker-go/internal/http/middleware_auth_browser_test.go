package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/stretchr/testify/assert"
)

func TestRequireAuthBrowser_APIRequest(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return nil, errors.New("session not found")
		},
	}

	middleware := RequireAuthBrowser(mockSvc)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := BrowserDetection()(middleware(testHandler))

	// Test API request without authentication
	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	req.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
}

func TestRequireAuthBrowser_BrowserRequest_Unauthenticated(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return nil, errors.New("session not found")
		},
	}

	middleware := RequireAuthBrowser(mockSvc)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := BrowserDetection()(middleware(testHandler))

	// Test browser request without authentication
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Header.Set("Accept", "text/html")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusSeeOther, w.Code)
	location := w.Header().Get("Location")
	assert.Contains(t, location, "/auth/login")
	assert.Contains(t, location, "redirect_uri=%2Fdashboard")
}

func TestRequireAuthBrowser_HTMXRequest_Unauthenticated(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return nil, errors.New("session not found")
		},
	}

	middleware := RequireAuthBrowser(mockSvc)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := BrowserDetection()(middleware(testHandler))

	req := httptest.NewRequest(http.MethodGet, "/dashboard/recent-alerts", nil)
	req.Header.Set("Hx-Request", "true")
	req.Header.Set("Hx-Current-Url", "/dashboard")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/auth/signed-out?redirect_uri=%2Fdashboard", w.Header().Get("Hx-Redirect"))
	assert.Empty(t, w.Header().Get("Location"))
}

func TestRequireAuthBrowser_HTMXRequest_Unauthenticated_NoCurrentURL(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return nil, errors.New("session not found")
		},
	}

	middleware := RequireAuthBrowser(mockSvc)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := BrowserDetection()(middleware(testHandler))

	req := httptest.NewRequest(http.MethodGet, "/alerts/partial", nil)
	req.Header.Set("Hx-Request", "true")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "/auth/signed-out?redirect_uri=%2Falerts%2Fpartial", w.Header().Get("Hx-Redirect"))
	assert.Empty(t, w.Header().Get("Location"))
}

func TestRedirectPathForRequestPrefersCurrentURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/fragments/recent", nil)
	req.Header.Set("Hx-Request", "true")
	req.Header.Set("Hx-Current-Url", "https://example.com/dashboard?page=2")

	result := redirectPathForRequest(req)

	assert.Equal(t, "/dashboard?page=2", result)
}

func TestRedirectPathForRequestFallsBackToReferer(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/fragments/recent", nil)
	req.Header.Set("Hx-Request", "true")
	req.Header.Set("Referer", "https://example.com/alerts")

	result := redirectPathForRequest(req)

	assert.Equal(t, "/alerts", result)
}

func TestRedirectPathForRequestRejectsSchemeRelative(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/fragments/recent", nil)
	req.Header.Set("Hx-Request", "true")
	req.Header.Set("Hx-Current-Url", "//evil.example.com/steal")
	req.Header.Set("Referer", "https://example.com/fallback")

	result := redirectPathForRequest(req)

	assert.Equal(t, "/fallback", result)
}

func TestRedirectPathForRequestHandlesMalformedCurrentURL(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/alerts/view", nil)
	req.Header.Set("Hx-Request", "true")
	req.Header.Set("Hx-Current-Url", "http://%zz")
	req.Header.Set("Referer", "https://example.com/alerts?id=5")

	result := redirectPathForRequest(req)

	assert.Equal(t, "/alerts?id=5", result)
}

func TestRedirectPathForRequestFallsBackToRequestURI(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/alerts/list?page=3", nil)
	req.Header.Set("Hx-Request", "true")
	req.Header.Set("Hx-Current-Url", "//evil.example.com/steal")
	req.Header.Set("Referer", "http://%zz")

	result := redirectPathForRequest(req)

	assert.Equal(t, "/alerts/list?page=3", result)
}

func TestRedirectToLoginDefaultsToRootRedirect(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.URL.Path = ""
	w := httptest.NewRecorder()

	redirectToLogin(w, req)

	assert.Equal(t, http.StatusSeeOther, w.Code)
	assert.Equal(t, "/auth/login?redirect_uri=%2F", w.Header().Get("Location"))
}

func TestRequireAuthBrowser_BrowserRequest_Authenticated(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return &domainauth.Session{
				ID:        sessionID,
				UserID:    "test-user",
				Email:     "test@example.com",
				Role:      domainauth.RoleUser,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}

	middleware := RequireAuthBrowser(mockSvc)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := GetSessionFromContext(r.Context())
		assert.NotNil(t, session)
		assert.Equal(t, "test-user", session.UserID)
		w.WriteHeader(http.StatusOK)
	})

	handler := BrowserDetection()(middleware(testHandler))

	// Test browser request with valid session
	req := httptest.NewRequest(http.MethodGet, "/dashboard", nil)
	req.Header.Set("Accept", "text/html")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRoleBrowser_InsufficientRole_BrowserRequest(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return &domainauth.Session{
				ID:        sessionID,
				UserID:    "test-user",
				Email:     "test@example.com",
				Role:      domainauth.RoleUser, // User role, but admin required
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}

	middleware := RequireRoleBrowser(mockSvc, domainauth.RoleAdmin)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	handler := BrowserDetection()(middleware(testHandler))

	// Test browser request with insufficient role
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Accept", "text/html")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Body.String(), "Access Denied")
}

func TestRequireRoleBrowser_InsufficientRole_APIRequest(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return &domainauth.Session{
				ID:        sessionID,
				UserID:    "test-user",
				Email:     "test@example.com",
				Role:      domainauth.RoleUser, // User role, but admin required
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}

	middleware := RequireRoleBrowser(mockSvc, domainauth.RoleAdmin)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	handler := BrowserDetection()(middleware(testHandler))

	// Test API request with insufficient role
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "valid-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
	assert.Contains(t, w.Header().Get("Content-Type"), "application/json")
}

func TestRequireRoleBrowser_SufficientRole(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return &domainauth.Session{
				ID:        sessionID,
				UserID:    "test-admin",
				Email:     "admin@example.com",
				Role:      domainauth.RoleAdmin,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}

	middleware := RequireRoleBrowser(mockSvc, domainauth.RoleAdmin)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := GetSessionFromContext(r.Context())
		assert.NotNil(t, session)
		assert.Equal(t, domainauth.RoleAdmin, session.Role)
		w.WriteHeader(http.StatusOK)
	})

	handler := BrowserDetection()(middleware(testHandler))

	// Test with sufficient role
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.Header.Set("Accept", "text/html")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "admin-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

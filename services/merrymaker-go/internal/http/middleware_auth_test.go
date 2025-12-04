package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/service"
)

// mockAuthServiceForMiddleware is a test double for AuthServiceInterface.
type mockAuthServiceForMiddleware struct {
	getSessionFunc func(ctx context.Context, sessionID string) (*domainauth.Session, error)
}

func (m *mockAuthServiceForMiddleware) GetSession(
	ctx context.Context,
	sessionID string,
) (*domainauth.Session, error) {
	if m.getSessionFunc != nil {
		return m.getSessionFunc(ctx, sessionID)
	}
	return &domainauth.Session{
		ID:        sessionID,
		UserID:    "test-user",
		Email:     "test@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(time.Hour),
	}, nil
}

// Implement other methods to satisfy the interface.
func (m *mockAuthServiceForMiddleware) BeginLogin(
	_ctx context.Context,
	_redirectURL string,
) (*service.BeginLoginResult, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthServiceForMiddleware) CompleteLogin(
	_ctx context.Context,
	_input service.CompleteLoginInput,
) (*service.CompleteLoginResult, error) {
	return nil, errors.New("not implemented")
}

func (m *mockAuthServiceForMiddleware) Logout(_ctx context.Context, _sessionID string) error {
	return errors.New("not implemented")
}

func TestRequireAuth_Success(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{}
	middleware := RequireAuth(mockSvc)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := GetSessionFromContext(r.Context())
		assert.NotNil(t, session)
		assert.Equal(t, "test-session-id", session.ID)
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the handler with middleware
	handler := middleware(testHandler)

	// Create request with session cookie
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "test-session-id"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireAuth_NoSession(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{}
	middleware := RequireAuth(mockSvc)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	// Wrap the handler with middleware
	handler := middleware(testHandler)

	// Create request without session cookie
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireAuth_InvalidSession(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return nil, errors.New("session not found")
		},
	}
	middleware := RequireAuth(mockSvc)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	// Wrap the handler with middleware
	handler := middleware(testHandler)

	// Create request with invalid session cookie
	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "invalid-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestRequireRole_Success(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return &domainauth.Session{
				ID:        sessionID,
				UserID:    "admin-user",
				Email:     "admin@example.com",
				Role:      domainauth.RoleAdmin,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}
	middleware := RequireRole(mockSvc, domainauth.RoleUser)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := GetSessionFromContext(r.Context())
		assert.NotNil(t, session)
		assert.Equal(t, domainauth.RoleAdmin, session.Role)
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the handler with middleware
	handler := middleware(testHandler)

	// Create request with session cookie
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "admin-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestRequireRole_InsufficientRole(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return &domainauth.Session{
				ID:        sessionID,
				UserID:    "regular-user",
				Email:     "user@example.com",
				Role:      domainauth.RoleUser,
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil
		},
	}
	middleware := RequireRole(mockSvc, domainauth.RoleAdmin)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called")
	})

	// Wrap the handler with middleware
	handler := middleware(testHandler)

	// Create request with session cookie
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "user-session"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestOptionalAuth_WithSession(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{}
	middleware := OptionalAuth(mockSvc)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := GetSessionFromContext(r.Context())
		assert.NotNil(t, session)
		assert.Equal(t, "test-session-id", session.ID)
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the handler with middleware
	handler := middleware(testHandler)

	// Create request with session cookie
	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "test-session-id"})
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestOptionalAuth_WithoutSession(t *testing.T) {
	mockSvc := &mockAuthServiceForMiddleware{}
	middleware := OptionalAuth(mockSvc)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		session := GetSessionFromContext(r.Context())
		assert.Nil(t, session)
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the handler with middleware
	handler := middleware(testHandler)

	// Create request without session cookie
	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}

func TestHasRequiredRole(t *testing.T) {
	tests := []struct {
		name         string
		userRole     domainauth.Role
		requiredRole domainauth.Role
		expected     bool
	}{
		{"Guest accessing Guest", domainauth.RoleGuest, domainauth.RoleGuest, true},
		{"User accessing Guest", domainauth.RoleUser, domainauth.RoleGuest, true},
		{"Admin accessing Guest", domainauth.RoleAdmin, domainauth.RoleGuest, true},
		{"Guest accessing User", domainauth.RoleGuest, domainauth.RoleUser, false},
		{"User accessing User", domainauth.RoleUser, domainauth.RoleUser, true},
		{"Admin accessing User", domainauth.RoleAdmin, domainauth.RoleUser, true},
		{"Guest accessing Admin", domainauth.RoleGuest, domainauth.RoleAdmin, false},
		{"User accessing Admin", domainauth.RoleUser, domainauth.RoleAdmin, false},
		{"Admin accessing Admin", domainauth.RoleAdmin, domainauth.RoleAdmin, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasRequiredRole(tt.userRole, tt.requiredRole)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetSessionFromContext(t *testing.T) {
	session := &domainauth.Session{
		ID:     "test-session",
		UserID: "test-user",
		Email:  "test@example.com",
		Role:   domainauth.RoleUser,
	}

	// Test with session in context
	ctx := SetSessionInContext(context.Background(), session)
	result := GetSessionFromContext(ctx)
	assert.Equal(t, session, result)

	// Test without session in context
	emptyCtx := context.Background()
	result = GetSessionFromContext(emptyCtx)
	assert.Nil(t, result)
}

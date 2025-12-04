package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/service"
)

// mockAuthService is a test double for service.AuthService.
type mockAuthService struct {
	beginLoginFunc    func(ctx context.Context, redirectURL string) (*service.BeginLoginResult, error)
	completeLoginFunc func(ctx context.Context, input service.CompleteLoginInput) (*service.CompleteLoginResult, error)
	getSessionFunc    func(ctx context.Context, sessionID string) (*domainauth.Session, error)
	logoutFunc        func(ctx context.Context, sessionID string) error
}

func (m *mockAuthService) BeginLogin(
	ctx context.Context,
	redirectURL string,
) (*service.BeginLoginResult, error) {
	if m.beginLoginFunc != nil {
		return m.beginLoginFunc(ctx, redirectURL)
	}
	return &service.BeginLoginResult{
		AuthURL: "https://example.com/auth?state=test-state&nonce=test-nonce",
		State:   "test-state",
		Nonce:   "test-nonce",
	}, nil
}

func (m *mockAuthService) CompleteLogin(
	ctx context.Context,
	input service.CompleteLoginInput,
) (*service.CompleteLoginResult, error) {
	if m.completeLoginFunc != nil {
		return m.completeLoginFunc(ctx, input)
	}
	return &service.CompleteLoginResult{
		Session: domainauth.Session{
			ID:        "test-session-id",
			UserID:    "test-user",
			Email:     "test@example.com",
			Role:      domainauth.RoleUser,
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}, nil
}

func (m *mockAuthService) GetSession(
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

func (m *mockAuthService) Logout(ctx context.Context, sessionID string) error {
	if m.logoutFunc != nil {
		return m.logoutFunc(ctx, sessionID)
	}
	return nil
}

func TestAuthHandlers_Login_Success(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(http.MethodGet, "/auth/login", nil)
	w := httptest.NewRecorder()

	handlers.Login(w, req)

	assert.Equal(t, http.StatusFound, w.Code)

	// Check that cookies were set
	resp := w.Result()
	defer resp.Body.Close()
	cookies := resp.Cookies()
	assert.Len(t, cookies, 3) // oauth_state, oauth_nonce, post_login_redirect

	// Check redirect location
	location := w.Header().Get("Location")
	assert.Contains(t, location, "https://example.com/auth")
}

func TestAuthHandlers_Login_WithRedirectURI(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(http.MethodGet, "/auth/login?redirect_uri=/dashboard", nil)
	w := httptest.NewRecorder()

	handlers.Login(w, req)

	assert.Equal(t, http.StatusFound, w.Code)

	// Check that redirect URI was stored in cookie
	resp := w.Result()
	defer resp.Body.Close()
	cookies := resp.Cookies()
	var redirectCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "post_login_redirect" {
			redirectCookie = cookie
			break
		}
	}
	require.NotNil(t, redirectCookie)
	assert.Equal(t, "/dashboard", redirectCookie.Value)
}

func TestAuthHandlers_Login_InvalidRedirectURI(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(http.MethodGet, "/auth/login?redirect_uri=://invalid", nil)
	w := httptest.NewRecorder()

	handlers.Login(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
}

func TestAuthHandlers_Callback_Success(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(
		http.MethodGet,
		"/auth/callback?code=test-code&state=test-state",
		nil,
	)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})
	req.AddCookie(&http.Cookie{Name: "oauth_nonce", Value: "test-nonce"})
	req.AddCookie(&http.Cookie{Name: "post_login_redirect", Value: "/dashboard"})

	w := httptest.NewRecorder()

	handlers.Callback(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/dashboard", w.Header().Get("Location"))

	// Check that session cookie was set
	resp := w.Result()
	defer resp.Body.Close()
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "session_id" {
			sessionCookie = cookie
			break
		}
	}
	require.NotNil(t, sessionCookie)
	assert.Equal(t, "test-session-id", sessionCookie.Value)
}

func TestAuthHandlers_Callback_MissingCode(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(http.MethodGet, "/auth/callback?state=test-state", nil)
	w := httptest.NewRecorder()

	handlers.Callback(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandlers_Callback_InvalidState(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(
		http.MethodGet,
		"/auth/callback?code=test-code&state=wrong-state",
		nil,
	)
	req.AddCookie(&http.Cookie{Name: "oauth_state", Value: "test-state"})

	w := httptest.NewRecorder()

	handlers.Callback(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestAuthHandlers_Logout_Success(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "test-session-id"})

	w := httptest.NewRecorder()

	handlers.Logout(w, req)

	assert.Equal(t, http.StatusFound, w.Code)
	assert.Equal(t, "/auth/signed-out?redirect_uri=%2F", w.Header().Get("Location"))

	// Check that session cookie was cleared
	resp := w.Result()
	defer resp.Body.Close()
	cookies := resp.Cookies()
	var sessionCookie *http.Cookie
	for _, cookie := range cookies {
		if cookie.Name == "session_id" {
			sessionCookie = cookie
			break
		}
	}
	require.NotNil(t, sessionCookie)
	assert.Empty(t, sessionCookie.Value)
	assert.Equal(t, -1, sessionCookie.MaxAge)
}

func TestAuthHandlers_Logout_AJAX(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(http.MethodPost, "/auth/logout", nil)
	req.Header.Set("Accept", "application/json")
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "test-session-id"})

	w := httptest.NewRecorder()

	handlers.Logout(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"status":"success"`)
	assert.Contains(t, w.Body.String(), `"redirect_to":"/auth/signed-out?redirect_uri=%2F"`)
}

func TestAuthHandlers_Status_Authenticated(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "test-session-id"})

	w := httptest.NewRecorder()

	handlers.Status(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"authenticated":true`)
	assert.Contains(t, w.Body.String(), `"test@example.com"`)
}

func TestAuthHandlers_Status_NotAuthenticated(t *testing.T) {
	mockSvc := &mockAuthService{
		getSessionFunc: func(ctx context.Context, sessionID string) (*domainauth.Session, error) {
			return nil, errors.New("session not found")
		},
	}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	req.AddCookie(&http.Cookie{Name: "session_id", Value: "invalid-session"})

	w := httptest.NewRecorder()

	handlers.Status(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"authenticated":false`)
}

func TestAuthHandlers_Status_NoSession(t *testing.T) {
	mockSvc := &mockAuthService{}
	handlers := &AuthHandlers{Svc: mockSvc}

	req := httptest.NewRequest(http.MethodGet, "/auth/status", nil)
	w := httptest.NewRecorder()

	handlers.Status(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), `"authenticated":false`)
}

package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	mocks "github.com/target/mmk-ui-api/internal/mocks/auth"
	"github.com/target/mmk-ui-api/internal/ports"
)

// mockSessionStore is a test helper for testing session store errors.
type mockSessionStore struct {
	saveFunc   func(context.Context, domainauth.Session) error
	getFunc    func(context.Context, string) (domainauth.Session, error)
	deleteFunc func(context.Context, string) error
}

func (m *mockSessionStore) Save(ctx context.Context, sess domainauth.Session) error {
	if m.saveFunc != nil {
		return m.saveFunc(ctx, sess)
	}
	return nil
}

func (m *mockSessionStore) Get(ctx context.Context, id string) (domainauth.Session, error) {
	if m.getFunc != nil {
		return m.getFunc(ctx, id)
	}
	return domainauth.Session{}, nil
}

func (m *mockSessionStore) Delete(ctx context.Context, id string) error {
	if m.deleteFunc != nil {
		return m.deleteFunc(ctx, id)
	}
	return nil
}

func TestNewAuthService(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	opts := AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	}

	service := NewAuthService(opts)

	assert.NotNil(t, service)
	assert.Equal(t, provider, service.provider)
	assert.Equal(t, sessions, service.sessions)
	assert.Equal(t, roles, service.roles)
}

func TestAuthService_BeginLogin_Success(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()
	redirectURL := "http://localhost:8080/callback"

	result, err := service.BeginLogin(ctx, redirectURL)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "https://mock-idp/auth", result.AuthURL)
	assert.Equal(t, "state-1", result.State)
	assert.Equal(t, "nonce-1", result.Nonce)
}

func TestAuthService_BeginLogin_EmptyRedirectURL(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()

	result, err := service.BeginLogin(ctx, "")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "redirect URL is required")
}

func TestAuthService_BeginLogin_ProviderError(t *testing.T) {
	provider := &mocks.MockAuthProvider{
		BeginFunc: func(_ context.Context, _ ports.BeginInput) (string, string, string, error) {
			return "", "", "", errors.New("provider error")
		},
	}
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()
	redirectURL := "http://localhost:8080/callback"

	result, err := service.BeginLogin(ctx, redirectURL)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "begin auth flow")
	assert.Contains(t, err.Error(), "provider error")
}

func TestAuthService_CompleteLogin_Success(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()
	input := CompleteLoginInput{
		Code:  "auth-code",
		State: "state-1",
		Nonce: "nonce-1",
	}

	result, err := service.CompleteLogin(ctx, input)

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.NotEmpty(t, result.Session.ID)
	assert.Equal(t, "mock-user-1", result.Session.UserID)
	assert.Equal(t, "mock.user@example.com", result.Session.Email)
	assert.Equal(t, domainauth.RoleUser, result.Session.Role)
	assert.True(t, result.Session.ExpiresAt.After(time.Now()))
}

func TestAuthService_CompleteLogin_PopulatesNames(t *testing.T) {
	provider := &mocks.MockAuthProvider{DefaultUser: domainauth.Identity{
		UserID:    "mock-user-1",
		FirstName: "Mock",
		LastName:  "User",
		Email:     "mock.user@example.com",
		Groups:    []string{"users"},
		ExpiresAt: time.Now().Add(time.Hour),
	}}
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{Provider: provider, Sessions: sessions, Roles: roles})
	ctx := context.Background()
	input := CompleteLoginInput{Code: "code", State: "state", Nonce: "nonce"}
	result, err := service.CompleteLogin(ctx, input)
	require.NoError(t, err)
	assert.Equal(t, "Mock", result.Session.FirstName)
	assert.Equal(t, "User", result.Session.LastName)
}

func TestAuthService_CompleteLogin_AdminRole(t *testing.T) {
	provider := &mocks.MockAuthProvider{
		DefaultUser: domainauth.Identity{
			UserID:    "admin-user",
			FirstName: "Admin",
			LastName:  "User",
			Email:     "admin@example.com",
			Groups:    []string{"admins", "users"},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()
	input := CompleteLoginInput{
		Code:  "auth-code",
		State: "state-1",
		Nonce: "nonce-1",
	}

	result, err := service.CompleteLogin(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, domainauth.RoleAdmin, result.Session.Role)
}

func TestAuthService_CompleteLogin_MissingCode(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()
	input := CompleteLoginInput{
		Code:  "", // Missing code
		State: "state-1",
		Nonce: "nonce-1",
	}

	result, err := service.CompleteLogin(ctx, input)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "authorization code is required")
}

func TestAuthService_CompleteLogin_MissingState(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()
	input := CompleteLoginInput{
		Code:  "auth-code",
		State: "", // Missing state
		Nonce: "nonce-1",
	}

	result, err := service.CompleteLogin(ctx, input)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "state parameter is required")
}

func TestAuthService_CompleteLogin_MissingNonce(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()
	input := CompleteLoginInput{
		Code:  "auth-code",
		State: "state-1",
		Nonce: "", // Missing nonce
	}

	result, err := service.CompleteLogin(ctx, input)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "nonce parameter is required")
}

func TestAuthService_CompleteLogin_ExchangeError(t *testing.T) {
	provider := &mocks.MockAuthProvider{
		ExchangeFunc: func(_ context.Context, _ ports.ExchangeInput) (domainauth.Identity, error) {
			return domainauth.Identity{}, errors.New("exchange error")
		},
	}
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()
	input := CompleteLoginInput{
		Code:  "auth-code",
		State: "state-1",
		Nonce: "nonce-1",
	}

	result, err := service.CompleteLogin(ctx, input)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "exchange authorization code")
	assert.Contains(t, err.Error(), "exchange error")
}

func TestAuthService_CompleteLogin_SessionSaveError(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := &mockSessionStore{
		saveFunc: func(_ context.Context, _ domainauth.Session) error {
			return errors.New("save error")
		},
	}
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()
	input := CompleteLoginInput{
		Code:  "auth-code",
		State: "state-1",
		Nonce: "nonce-1",
	}

	result, err := service.CompleteLogin(ctx, input)

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "save session")
	assert.Contains(t, err.Error(), "save error")
}

func TestAuthService_GetSession_Success(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()

	// Create a session first
	session := domainauth.Session{
		ID:        "test-session-1",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	err := sessions.Save(ctx, session)
	require.NoError(t, err)

	// Get the session
	result, err := service.GetSession(ctx, "test-session-1")

	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, session.ID, result.ID)
	assert.Equal(t, session.UserID, result.UserID)
	assert.Equal(t, session.Email, result.Email)
	assert.Equal(t, session.Role, result.Role)
}

func TestAuthService_GetSession_EmptyID(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()

	result, err := service.GetSession(ctx, "")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "session ID is required")
}

func TestAuthService_GetSession_NotFound(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()

	result, err := service.GetSession(ctx, "non-existent")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "get session")
}

func TestAuthService_GetSession_Expired(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()

	// Create an expired session
	session := domainauth.Session{
		ID:        "expired-session",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(-1 * time.Hour), // Expired 1 hour ago
	}
	err := sessions.Save(ctx, session)
	require.NoError(t, err)

	// Try to get the expired session
	result, err := service.GetSession(ctx, "expired-session")

	require.Error(t, err)
	assert.Nil(t, result)
	assert.Contains(t, err.Error(), "session expired")

	// Verify the expired session was cleaned up
	_, err = sessions.Get(ctx, "expired-session")
	assert.Equal(t, mocks.ErrNotFound, err)
}

func TestAuthService_Logout_Success(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()

	// Create a session first
	session := domainauth.Session{
		ID:        "test-session-1",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}
	err := sessions.Save(ctx, session)
	require.NoError(t, err)

	// Logout
	err = service.Logout(ctx, "test-session-1")

	require.NoError(t, err)

	// Verify session was deleted
	_, err = sessions.Get(ctx, "test-session-1")
	assert.Equal(t, mocks.ErrNotFound, err)
}

func TestAuthService_Logout_EmptyID(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := mocks.NewMemorySessionStore()
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()

	// Logout with empty ID should not error
	err := service.Logout(ctx, "")

	assert.NoError(t, err)
}

func TestAuthService_Logout_DeleteError(t *testing.T) {
	provider := mocks.NewMockAuthProvider()
	sessions := &mockSessionStore{
		deleteFunc: func(_ context.Context, _ string) error {
			return errors.New("delete error")
		},
	}
	roles := mocks.StaticRoleMapper{AdminGroup: "admins", UserGroup: "users"}

	service := NewAuthService(AuthServiceOptions{
		Provider: provider,
		Sessions: sessions,
		Roles:    roles,
	})

	ctx := context.Background()

	err := service.Logout(ctx, "test-session")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete session")
	assert.Contains(t, err.Error(), "delete error")
}

func TestGenerateSessionID(t *testing.T) {
	id1 := generateSessionID()
	id2 := generateSessionID()

	assert.NotEmpty(t, id1)
	assert.NotEmpty(t, id2)
	assert.NotEqual(t, id1, id2) // Should generate unique IDs

	// Should be valid UUID format
	assert.Len(t, id1, 36) // UUID string length
	assert.Contains(t, id1, "-")
}

package auth

import (
	"context"
	"testing"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/ports"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMockAuthProvider_Begin_Defaults(t *testing.T) {
	provider := NewMockAuthProvider()
	ctx := context.Background()

	input := ports.BeginInput{RedirectURL: "http://localhost:8080/callback"}
	authURL, state, nonce, err := provider.Begin(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "https://mock-idp/auth", authURL)
	assert.Equal(t, "state-1", state)
	assert.Equal(t, "nonce-1", nonce)

	// Second call should increment counters
	authURL2, state2, nonce2, err2 := provider.Begin(ctx, input)
	require.NoError(t, err2)
	assert.Equal(t, "https://mock-idp/auth", authURL2)
	assert.Equal(t, "state-2", state2)
	assert.Equal(t, "nonce-2", nonce2)
}

func TestMockAuthProvider_Begin_CustomValues(t *testing.T) {
	provider := &MockAuthProvider{
		AuthURL:     "https://custom-idp/login",
		StatePrefix: "custom-state",
		NoncePrefix: "custom-nonce",
	}
	ctx := context.Background()

	input := ports.BeginInput{RedirectURL: "http://localhost:8080/callback"}
	authURL, state, nonce, err := provider.Begin(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "https://custom-idp/login", authURL)
	assert.Equal(t, "custom-state-1", state)
	assert.Equal(t, "custom-nonce-1", nonce)
}

func TestMockAuthProvider_Begin_CustomFunc(t *testing.T) {
	provider := &MockAuthProvider{
		BeginFunc: func(_ context.Context, _ ports.BeginInput) (string, string, string, error) {
			return "custom-url", "custom-state", "custom-nonce", nil
		},
	}
	ctx := context.Background()

	input := ports.BeginInput{RedirectURL: "http://localhost:8080/callback"}
	authURL, state, nonce, err := provider.Begin(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "custom-url", authURL)
	assert.Equal(t, "custom-state", state)
	assert.Equal(t, "custom-nonce", nonce)
}

func TestMockAuthProvider_Exchange_Defaults(t *testing.T) {
	provider := NewMockAuthProvider()
	ctx := context.Background()

	input := ports.ExchangeInput{
		Code:  "auth-code",
		State: "state-1",
		Nonce: "nonce-1",
	}
	identity, err := provider.Exchange(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "mock-user-1", identity.UserID)
	assert.Equal(t, "Mock", identity.FirstName)
	assert.Equal(t, "User", identity.LastName)
	assert.Equal(t, "mock.user@example.com", identity.Email)
	assert.Equal(t, []string{"users"}, identity.Groups)
	assert.True(t, identity.ExpiresAt.After(time.Now()))
}

func TestMockAuthProvider_Exchange_CustomUser(t *testing.T) {
	customUser := domainauth.Identity{
		UserID:    "custom-user",
		FirstName: "Custom",
		LastName:  "Person",
		Email:     "custom@example.com",
		Groups:    []string{"admins", "users"},
	}
	provider := &MockAuthProvider{DefaultUser: customUser}
	ctx := context.Background()

	input := ports.ExchangeInput{
		Code:  "auth-code",
		State: "state-1",
		Nonce: "nonce-1",
	}
	identity, err := provider.Exchange(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "custom-user", identity.UserID)
	assert.Equal(t, "Custom", identity.FirstName)
	assert.Equal(t, "Person", identity.LastName)
	assert.Equal(t, "custom@example.com", identity.Email)
	assert.Equal(t, []string{"admins", "users"}, identity.Groups)
	assert.True(t, identity.ExpiresAt.After(time.Now()))
}

func TestMockAuthProvider_Exchange_CustomFunc(t *testing.T) {
	provider := &MockAuthProvider{
		ExchangeFunc: func(_ context.Context, _ ports.ExchangeInput) (domainauth.Identity, error) {
			return domainauth.Identity{
				UserID: "func-user",
				Email:  "func@example.com",
			}, nil
		},
	}
	ctx := context.Background()

	input := ports.ExchangeInput{
		Code:  "auth-code",
		State: "state-1",
		Nonce: "nonce-1",
	}
	identity, err := provider.Exchange(ctx, input)

	require.NoError(t, err)
	assert.Equal(t, "func-user", identity.UserID)
	assert.Equal(t, "func@example.com", identity.Email)
}

func TestStaticRoleMapper_AdminRole(t *testing.T) {
	mapper := StaticRoleMapper{
		AdminGroup: "admins",
		UserGroup:  "users",
	}

	role := mapper.Map([]string{"admins", "users"})
	assert.Equal(t, domainauth.RoleAdmin, role)

	role = mapper.Map([]string{"admins"})
	assert.Equal(t, domainauth.RoleAdmin, role)
}

func TestStaticRoleMapper_UserRole(t *testing.T) {
	mapper := StaticRoleMapper{
		AdminGroup: "admins",
		UserGroup:  "users",
	}

	role := mapper.Map([]string{"users"})
	assert.Equal(t, domainauth.RoleUser, role)

	role = mapper.Map([]string{"users", "other"})
	assert.Equal(t, domainauth.RoleUser, role)
}

func TestStaticRoleMapper_GuestRole(t *testing.T) {
	mapper := StaticRoleMapper{
		AdminGroup: "admins",
		UserGroup:  "users",
	}

	role := mapper.Map([]string{"other", "groups"})
	assert.Equal(t, domainauth.RoleGuest, role)

	role = mapper.Map([]string{})
	assert.Equal(t, domainauth.RoleGuest, role)

	role = mapper.Map(nil)
	assert.Equal(t, domainauth.RoleGuest, role)
}

func TestStaticRoleMapper_EmptyConfig(t *testing.T) {
	mapper := StaticRoleMapper{}

	role := mapper.Map([]string{"admins", "users"})
	assert.Equal(t, domainauth.RoleGuest, role)
}

func TestMemorySessionStore_SaveAndGet(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	session := domainauth.Session{
		ID:        "test-session-1",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	// Save session
	err := store.Save(ctx, session)
	require.NoError(t, err)

	// Get session
	retrieved, err := store.Get(ctx, "test-session-1")
	require.NoError(t, err)
	assert.Equal(t, session.ID, retrieved.ID)
	assert.Equal(t, session.UserID, retrieved.UserID)
	assert.Equal(t, session.Email, retrieved.Email)
	assert.Equal(t, session.Role, retrieved.Role)
	assert.WithinDuration(t, session.ExpiresAt, retrieved.ExpiresAt, time.Second)
}

func TestMemorySessionStore_GetNonExistent(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	_, err := store.Get(ctx, "non-existent")
	assert.Equal(t, ErrNotFound, err)
}

func TestMemorySessionStore_GetEmptyID(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	_, err := store.Get(ctx, "")
	assert.Equal(t, ErrNotFound, err)
}

func TestMemorySessionStore_SaveEmptyID(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	session := domainauth.Session{
		ID:        "", // Empty ID should cause error
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	err := store.Save(ctx, session)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session ID cannot be empty")
}

func TestMemorySessionStore_Delete(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	session := domainauth.Session{
		ID:        "test-session-1",
		UserID:    "user-123",
		Email:     "user@example.com",
		Role:      domainauth.RoleUser,
		ExpiresAt: time.Now().Add(30 * time.Minute),
	}

	// Save session
	err := store.Save(ctx, session)
	require.NoError(t, err)

	// Delete session
	err = store.Delete(ctx, "test-session-1")
	require.NoError(t, err)

	// Verify session was deleted
	_, err = store.Get(ctx, "test-session-1")
	assert.Equal(t, ErrNotFound, err)
}

func TestMemorySessionStore_DeleteEmptyID(t *testing.T) {
	store := NewMemorySessionStore()
	ctx := context.Background()

	// Delete with empty ID should not error
	err := store.Delete(ctx, "")
	assert.NoError(t, err)
}

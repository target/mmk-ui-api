package auth

// Package auth contains simple hand-written test doubles for auth ports.
// These are lightweight and suitable for unit tests without codegen.

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/ports"
)

// Ensure compile-time conformance to ports.
var (
	_ ports.AuthProvider = (*MockAuthProvider)(nil)
	_ ports.SessionStore = (*MemorySessionStore)(nil)
	_ ports.RoleMapper   = (*StaticRoleMapper)(nil)
)

// MockAuthProvider simulates an IdP for tests with deterministic state/nonce handling.
type MockAuthProvider struct {
	BeginFunc    func(ctx context.Context, in ports.BeginInput) (authURL, state, nonce string, err error)
	ExchangeFunc func(ctx context.Context, in ports.ExchangeInput) (domainauth.Identity, error)

	// Deterministic values for predictable testing
	AuthURL     string
	StatePrefix string
	NoncePrefix string
	DefaultUser domainauth.Identity

	// Internal state tracking for deterministic behavior
	callCount int
}

// NewMockAuthProvider creates a MockAuthProvider with sensible defaults.
func NewMockAuthProvider() *MockAuthProvider {
	return &MockAuthProvider{
		AuthURL:     "https://mock-idp/auth",
		StatePrefix: "state",
		NoncePrefix: "nonce",
		DefaultUser: domainauth.Identity{
			UserID:    "mock-user-1",
			FirstName: "Mock",
			LastName:  "User",
			Email:     "mock.user@example.com",
			Groups:    []string{"users"},
			ExpiresAt: time.Now().Add(time.Hour),
		},
	}
}

func (m *MockAuthProvider) Begin(ctx context.Context, in ports.BeginInput) (string, string, string, error) {
	if m.BeginFunc != nil {
		return m.BeginFunc(ctx, in)
	}

	m.callCount++
	authURL := m.AuthURL
	if authURL == "" {
		authURL = "https://mock-idp/auth"
	}

	// Generate deterministic state and nonce based on call count and redirect URL
	statePrefix := m.StatePrefix
	if statePrefix == "" {
		statePrefix = "state"
	}
	noncePrefix := m.NoncePrefix
	if noncePrefix == "" {
		noncePrefix = "nonce"
	}

	state := fmt.Sprintf("%s-%d", statePrefix, m.callCount)
	nonce := fmt.Sprintf("%s-%d", noncePrefix, m.callCount)

	return authURL, state, nonce, nil
}

func (m *MockAuthProvider) Exchange(ctx context.Context, in ports.ExchangeInput) (domainauth.Identity, error) {
	if m.ExchangeFunc != nil {
		return m.ExchangeFunc(ctx, in)
	}

	// Return a copy of the default user with a fresh expiration time
	user := m.DefaultUser
	if user.UserID == "" {
		user = domainauth.Identity{
			UserID:    "mock-user-1",
			FirstName: "Mock",
			LastName:  "User",
			Email:     "mock.user@example.com",
			Groups:    []string{"users"},
		}
	}
	user.ExpiresAt = time.Now().Add(time.Hour)

	return user, nil
}

// MemorySessionStore is an in-memory session store for unit tests.
type MemorySessionStore struct {
	sessions map[string]domainauth.Session
}

// NewMemorySessionStore creates a new in-memory session store.
func NewMemorySessionStore() *MemorySessionStore {
	return &MemorySessionStore{
		sessions: make(map[string]domainauth.Session),
	}
}

func (m *MemorySessionStore) Save(_ context.Context, sess domainauth.Session) error {
	if sess.ID == "" {
		return errors.New("session ID cannot be empty")
	}
	m.sessions[sess.ID] = sess
	return nil
}

func (m *MemorySessionStore) Get(_ context.Context, id string) (domainauth.Session, error) {
	if id == "" {
		return domainauth.Session{}, ErrNotFound
	}
	sess, ok := m.sessions[id]
	if !ok {
		return domainauth.Session{}, ErrNotFound
	}
	return sess, nil
}

func (m *MemorySessionStore) Delete(_ context.Context, id string) error {
	if id == "" {
		return nil
	}
	delete(m.sessions, id)
	return nil
}

// ErrNotFound is returned by mocks when an entity is not present.
type notFoundError struct{}

func (notFoundError) Error() string { return "not found" }

var ErrNotFound error = notFoundError{}

// StaticRoleMapper maps groups by simple string membership rules.
type StaticRoleMapper struct {
	AdminGroup string
	UserGroup  string
}

func (m StaticRoleMapper) Map(groups []string) domainauth.Role {
	for _, g := range groups {
		if m.AdminGroup != "" && g == m.AdminGroup {
			return domainauth.RoleAdmin
		}
	}
	for _, g := range groups {
		if m.UserGroup != "" && g == m.UserGroup {
			return domainauth.RoleUser
		}
	}
	return domainauth.RoleGuest
}

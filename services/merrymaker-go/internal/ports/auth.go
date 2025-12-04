package ports

// Package ports defines interfaces (hexagonal ports) for auth-related behavior.
// Implementations live in internal/adapters; orchestration in internal/service.

import (
	"context"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
)

// BeginInput carries inputs for initiating an auth flow.
type BeginInput struct {
	RedirectURL string
}

// AuthProvider initiates and completes an authentication flow against an IdP.
type AuthProvider interface {
	// Begin starts the login flow and returns the provider auth URL, an opaque state, and a nonce.
	Begin(ctx context.Context, in BeginInput) (authURL, state, nonce string, err error)

	// Exchange completes the login flow, verifying state and nonce, and returns the authenticated identity.
	Exchange(ctx context.Context, in ExchangeInput) (domainauth.Identity, error)
}

// ExchangeInput groups parameters for the code/token exchange.
type ExchangeInput struct {
	Code  string
	State string
	Nonce string
}

// SessionStore persists and retrieves user sessions.
type SessionStore interface {
	Save(ctx context.Context, sess domainauth.Session) error
	Get(ctx context.Context, id string) (domainauth.Session, error)
	Delete(ctx context.Context, id string) error
}

// RoleMapper maps provider groups to application roles.
type RoleMapper interface {
	Map(groups []string) domainauth.Role
}

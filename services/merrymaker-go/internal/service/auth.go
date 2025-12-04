package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/ports"
	"github.com/google/uuid"
)

// AuthServiceOptions groups dependencies for AuthService.
type AuthServiceOptions struct {
	Provider ports.AuthProvider
	Sessions ports.SessionStore
	Roles    ports.RoleMapper
}

// AuthService orchestrates authentication flows by coordinating provider, role mapping, and session persistence.
type AuthService struct {
	provider ports.AuthProvider
	sessions ports.SessionStore
	roles    ports.RoleMapper
}

var errSessionExpired = errors.New("session expired")

// NewAuthService constructs a new AuthService.
func NewAuthService(opts AuthServiceOptions) *AuthService {
	return &AuthService{
		provider: opts.Provider,
		sessions: opts.Sessions,
		roles:    opts.Roles,
	}
}

// BeginLoginResult contains the result of beginning a login flow.
type BeginLoginResult struct {
	AuthURL string
	State   string
	Nonce   string
}

// BeginLogin initiates an authentication flow and returns the provider auth URL with state and nonce.
func (s *AuthService) BeginLogin(ctx context.Context, redirectURL string) (*BeginLoginResult, error) {
	if redirectURL == "" {
		return nil, errors.New("redirect URL is required")
	}

	input := ports.BeginInput{RedirectURL: redirectURL}
	authURL, state, nonce, err := s.provider.Begin(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("begin auth flow: %w", err)
	}

	return &BeginLoginResult{
		AuthURL: authURL,
		State:   state,
		Nonce:   nonce,
	}, nil
}

// CompleteLoginInput groups parameters for completing a login flow.
type CompleteLoginInput struct {
	Code  string
	State string
	Nonce string
}

// CompleteLoginResult contains the result of completing a login flow.
type CompleteLoginResult struct {
	Session domainauth.Session
}

// CompleteLogin completes an authentication flow by exchanging the code for an identity,
// mapping roles, and persisting a session.
func (s *AuthService) CompleteLogin(ctx context.Context, input CompleteLoginInput) (*CompleteLoginResult, error) {
	if input.Code == "" {
		return nil, errors.New("authorization code is required")
	}
	if input.State == "" {
		return nil, errors.New("state parameter is required")
	}
	if input.Nonce == "" {
		return nil, errors.New("nonce parameter is required")
	}

	// Exchange authorization code for identity
	exchangeInput := ports.ExchangeInput{
		Code:  input.Code,
		State: input.State,
		Nonce: input.Nonce,
	}
	identity, err := s.provider.Exchange(ctx, exchangeInput)
	if err != nil {
		return nil, fmt.Errorf("exchange authorization code: %w", err)
	}

	// Map provider groups to application role
	role := s.roles.Map(identity.Groups)

	// Generate session ID
	sessionID := generateSessionID()

	// Create session
	session := domainauth.Session{
		ID:        sessionID,
		UserID:    identity.UserID,
		FirstName: identity.FirstName,
		LastName:  identity.LastName,
		Email:     identity.Email,
		Role:      role,
		ExpiresAt: identity.ExpiresAt,
	}

	// Persist session
	if saveErr := s.sessions.Save(ctx, session); saveErr != nil {
		return nil, fmt.Errorf("save session: %w", saveErr)
	}

	return &CompleteLoginResult{
		Session: session,
	}, nil
}

// GetSession retrieves a session by ID.
func (s *AuthService) GetSession(ctx context.Context, sessionID string) (*domainauth.Session, error) {
	if sessionID == "" {
		return nil, errors.New("session ID is required")
	}

	session, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("get session: %w", err)
	}

	// Check if session is expired
	if time.Now().After(session.ExpiresAt) {
		// Clean up expired session
		if deleteErr := s.sessions.Delete(ctx, sessionID); deleteErr != nil {
			return nil, errors.Join(errSessionExpired, fmt.Errorf("delete session: %w", deleteErr))
		}
		return nil, errSessionExpired
	}

	return &session, nil
}

// Logout removes a session.
func (s *AuthService) Logout(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil // Nothing to logout
	}

	if err := s.sessions.Delete(ctx, sessionID); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}

	return nil
}

// generateSessionID creates a cryptographically secure random session ID.
func generateSessionID() string {
	// Use UUID for session ID - it's URL-safe and has good entropy
	id := uuid.New()
	return id.String()
}

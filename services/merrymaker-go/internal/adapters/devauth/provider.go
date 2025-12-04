package devauth

// Package devauth provides a simple, config-driven AuthProvider for local development.

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/ports"
)

// Config controls the dev auth provider behavior.
// All fields are required except Groups, which may be empty.
type Config struct {
	UserID          string
	Email           string
	Groups          []string
	SessionDuration time.Duration // default 8h when zero
}

// Provider implements ports.AuthProvider for local development.
// It short-circuits the OAuth flow by redirecting back to our own callback
// with locally generated state and nonce.
// Exchange ignores the code and returns the configured identity.
type Provider struct {
	identity        domainauth.Identity
	sessionDuration time.Duration
}

// NewProvider constructs a dev auth provider from Config.
func NewProvider(cfg Config) (*Provider, error) {
	if cfg.UserID == "" {
		return nil, errors.New("dev auth: UserID is required")
	}
	if cfg.Email == "" {
		return nil, errors.New("dev auth: Email is required")
	}
	dur := cfg.SessionDuration
	if dur == 0 {
		dur = 8 * time.Hour
	}
	return &Provider{
		identity: domainauth.Identity{
			UserID:    cfg.UserID,
			Email:     cfg.Email,
			Groups:    append([]string(nil), cfg.Groups...),
			ExpiresAt: time.Now().Add(dur),
		},
		sessionDuration: dur,
	}, nil
}

// Begin returns a local callback URL and cryptographically secure state and nonce.
func (p *Provider) Begin(_ context.Context, _ ports.BeginInput) (string, string, string, error) {
	state, err := randomString(24)
	if err != nil {
		return "", "", "", fmt.Errorf("generate state: %w", err)
	}
	nonce, err := randomString(24)
	if err != nil {
		return "", "", "", fmt.Errorf("generate nonce: %w", err)
	}
	// Our standard handler expects GET /auth/callback?code=...&state=...
	authURL := "/auth/callback?code=dev&state=" + state
	return authURL, state, nonce, nil
}

// Exchange ignores the provided code/state/nonce (validation handled by handler) and returns the dev identity.
func (p *Provider) Exchange(_ context.Context, _ ports.ExchangeInput) (domainauth.Identity, error) {
	// Refresh expiry on each exchange for convenience
	if time.Until(p.identity.ExpiresAt) < 5*time.Minute {
		p.identity.ExpiresAt = time.Now().Add(p.sessionDuration)
	}
	return p.identity, nil
}

func randomString(n int) (string, error) {
	if n <= 0 {
		return "", nil
	}
	// Compute number of random bytes needed to produce at least n base64 URL chars
	bLen := (n*3 + 3) / 4
	b := make([]byte, bLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	s := base64.RawURLEncoding.EncodeToString(b)
	if len(s) < n {
		// pad
		extra := make([]byte, 1)
		if _, err := rand.Read(extra); err != nil {
			return "", err
		}
		s += base64.RawURLEncoding.EncodeToString(extra)
	}
	return s[:n], nil
}

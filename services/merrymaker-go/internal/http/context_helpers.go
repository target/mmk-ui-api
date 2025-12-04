package httpx

import (
	"context"

	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
)

// sessionKey is an unexported context key type to avoid collisions across packages.
// Centralized in this file so all handlers/middleware use the same key.
type sessionKey struct{}

// SetSessionInContext returns a child context that carries the given session.
// If session is nil, the original ctx is returned unchanged.
func SetSessionInContext(ctx context.Context, session *domainauth.Session) context.Context {
	if session == nil {
		return ctx
	}
	return context.WithValue(ctx, sessionKey{}, session)
}

// GetUserSessionFromContext returns the user session from context and a boolean indicating presence.
func GetUserSessionFromContext(ctx context.Context) (*domainauth.Session, bool) {
	if session, ok := ctx.Value(sessionKey{}).(*domainauth.Session); ok && session != nil {
		return session, true
	}
	return nil, false
}

// GetSessionFromContext retrieves the session from the request context.
// Maintained for convenience; prefer GetUserSessionFromContext when you need presence info.
func GetSessionFromContext(ctx context.Context) *domainauth.Session {
	if s, ok := GetUserSessionFromContext(ctx); ok {
		return s
	}
	return nil
}

// IsGuestUser reports whether the current request context is unauthenticated or a guest session.
func IsGuestUser(ctx context.Context) bool {
	s, ok := GetUserSessionFromContext(ctx)
	if !ok || s == nil {
		return true
	}
	return s.IsGuest()
}

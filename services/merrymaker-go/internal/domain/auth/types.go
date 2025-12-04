package auth

// Package auth contains domain-level types for authentication and sessions.
// It is pure and free of framework/adapter concerns.

import "time"

// Role represents an application's authorization role.
// Keep string form for easy persistence and cookies.
// Valid values are defined as constants below.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
	RoleGuest Role = "guest"
)

// Identity represents the authenticated principal returned by an IdP.
// Adapters map provider-specific claims into this shape.
type Identity struct {
	UserID    string // stable user identifier (e.g., samAccountName or sub)
	FirstName string
	LastName  string
	Email     string
	Groups    []string
	ExpiresAt time.Time // absolute expiry from IdP token
}

// Session is the server-side record we persist for an authenticated user.
// ID is an opaque session identifier (e.g., random URL-safe string).
type Session struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	Email     string    `json:"email"`
	Role      Role      `json:"role"`
	ExpiresAt time.Time `json:"expires_at"`
}

// IsGuest returns true if the session role is guest.
func (s Session) IsGuest() bool { return s.Role == RoleGuest }

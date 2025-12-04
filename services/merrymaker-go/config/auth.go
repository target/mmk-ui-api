package config

import (
	"fmt"
	"strings"
)

// AuthMode represents the authentication mode for the application.
type AuthMode string

const (
	// AuthModeOAuth uses OAuth/OIDC for authentication.
	AuthModeOAuth AuthMode = "oauth"
	// AuthModeMock uses mock/dev authentication (for development only).
	AuthModeMock AuthMode = "mock"
)

// UnmarshalText implements encoding.TextUnmarshaler for AuthMode.
func (a *AuthMode) UnmarshalText(text []byte) error {
	v := strings.ToLower(string(text))
	switch v {
	case "oauth", "mock":
		*a = AuthMode(v)
		return nil
	default:
		return fmt.Errorf("invalid AuthMode: %q (valid options: oauth, mock)", v)
	}
}

// OAuthConfig contains OAuth/OIDC configuration.
type OAuthConfig struct {
	ClientID     string `env:"CLIENT_ID"     envDefault:"merrymaker"`
	ClientSecret string `env:"CLIENT_SECRET" envDefault:"merrymaker"`
	RedirectURL  string `env:"REDIRECT_URL"  envDefault:"http://localhost:8080/auth/callback"`
	Scope        string `env:"SCOPE"         envDefault:"openid profile email groups"`
	DiscoveryURL string `env:"DISCOVERY_URL"`
	LogoutURL    string `env:"LOGOUT_URL"`
}

// DevAuthConfig controls mock/dev authentication identity.
// Used when AUTH_MODE=mock for development and testing.
type DevAuthConfig struct {
	UserID string   `env:"USER_ID" envDefault:"dev-user"`
	Email  string   `env:"EMAIL"   envDefault:"dev@example.com"`
	Groups []string `env:"GROUPS"  envDefault:"admins"          envSeparator:";"`
}

// AuthConfig groups all authentication-related configuration.
type AuthConfig struct {
	// Mode determines which authentication provider to use.
	Mode AuthMode `env:"AUTH_MODE" envDefault:"oauth"`

	// OAuth configuration (used when Mode=oauth).
	OAuth OAuthConfig `envPrefix:"OAUTH_"`

	// DevAuth configuration (used when Mode=mock).
	DevAuth DevAuthConfig `envPrefix:"DEV_AUTH_"`

	// AdminGroup is the LDAP/AD group DN for admin users.
	AdminGroup string `env:"ADMIN_GROUP,required"`

	// UserGroup is the LDAP/AD group DN for regular users.
	UserGroup string `env:"USER_GROUP,required"`
}

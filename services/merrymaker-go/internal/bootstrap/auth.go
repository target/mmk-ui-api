package bootstrap

import (
	"log/slog"

	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/adapters/authroles"
	"github.com/target/mmk-ui-api/internal/adapters/devauth"
	"github.com/target/mmk-ui-api/internal/adapters/oidc"
	redisadapter "github.com/target/mmk-ui-api/internal/adapters/redis"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/redis/go-redis/v9"
)

// AuthConfig contains configuration for auth service.
type AuthConfig struct {
	Auth        config.AuthConfig
	RedisClient redis.UniversalClient
	Logger      *slog.Logger
}

// BuildAuthService creates an auth service based on the configured auth mode.
// Returns nil if auth is not configured or configuration is invalid.
func BuildAuthService(cfg AuthConfig) *service.AuthService {
	if cfg.RedisClient == nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn("auth service disabled: redis client not configured", "mode", cfg.Auth.Mode)
		}
		return nil
	}

	// Create Redis session store shared by both modes
	sessionStore := redisadapter.NewSessionStoreWithPrefix(cfg.RedisClient, "session:")

	// Role mapper is shared
	roleMapper := authroles.StaticRoleMapper{
		AdminGroup: cfg.Auth.AdminGroup,
		UserGroup:  cfg.Auth.UserGroup,
	}

	switch cfg.Auth.Mode {
	case config.AuthModeMock:
		return buildDevAuthService(cfg, sessionStore, roleMapper)

	case config.AuthModeOAuth:
		return buildOAuthService(cfg, sessionStore, roleMapper)

	default:
		return nil
	}
}

func buildDevAuthService(
	cfg AuthConfig,
	sessionStore *redisadapter.SessionStore,
	roleMapper authroles.StaticRoleMapper,
) *service.AuthService {
	// Explicitly enabled dev auth mode; build a local provider.
	prov, err := devauth.NewProvider(devauth.Config{
		UserID: cfg.Auth.DevAuth.UserID,
		Email:  cfg.Auth.DevAuth.Email,
		Groups: cfg.Auth.DevAuth.Groups,
		// session duration defaults inside provider
	})
	if err != nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn("failed to create dev auth provider, auth disabled", "error", err)
		}
		return nil
	}

	return service.NewAuthService(service.AuthServiceOptions{
		Provider: prov,
		Sessions: sessionStore,
		Roles:    roleMapper,
	})
}

func buildOAuthService(
	cfg AuthConfig,
	sessionStore *redisadapter.SessionStore,
	roleMapper authroles.StaticRoleMapper,
) *service.AuthService {
	// Only enable when fully configured
	oauth := cfg.Auth.OAuth
	if oauth.DiscoveryURL == "" || oauth.ClientID == "" || oauth.ClientSecret == "" {
		if cfg.Logger != nil {
			cfg.Logger.Warn("AuthModeOAuth selected but required config missing; auth disabled",
				"discovery_url_empty", oauth.DiscoveryURL == "",
				"client_id_empty", oauth.ClientID == "",
				"client_secret_empty", oauth.ClientSecret == "",
			)
		}
		return nil
	}

	prov, err := oidc.NewProvider(oidc.ProviderConfig{
		ClientID:     oauth.ClientID,
		ClientSecret: oauth.ClientSecret,
		RedirectURL:  oauth.RedirectURL,
		Scope:        oauth.Scope,
		DiscoveryURL: oauth.DiscoveryURL,
	})
	if err != nil {
		if cfg.Logger != nil {
			cfg.Logger.Warn("failed to create OIDC provider, auth disabled", "error", err)
		}
		return nil
	}

	return service.NewAuthService(service.AuthServiceOptions{
		Provider: prov,
		Sessions: sessionStore,
		Roles:    roleMapper,
	})
}

package bootstrap

import (
	"io"
	"log/slog"
	"testing"

	"github.com/target/mmk-ui-api/config"
)

func TestBuildAuthServiceReturnsNilWithoutRedis(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	tests := []struct {
		name string
		auth config.AuthConfig
	}{
		{
			name: "dev auth mode",
			auth: config.AuthConfig{
				Mode:       config.AuthModeMock,
				AdminGroup: "admins",
				UserGroup:  "users",
				DevAuth: config.DevAuthConfig{
					UserID: "dev",
					Email:  "dev@example.com",
					Groups: []string{"admins"},
				},
			},
		},
		{
			name: "oauth mode",
			auth: config.AuthConfig{
				Mode:       config.AuthModeOAuth,
				AdminGroup: "admins",
				UserGroup:  "users",
				OAuth: config.OAuthConfig{
					ClientID:     "client-id",
					ClientSecret: "client-secret",
					DiscoveryURL: "https://issuer.example.com",
					RedirectURL:  "https://app.example.com/auth/callback",
					Scope:        "openid",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AuthConfig{
				Auth:        tt.auth,
				RedisClient: nil,
				Logger:      logger,
			}

			if svc := BuildAuthService(cfg); svc != nil {
				t.Fatalf("BuildAuthService() = %v, want nil", svc)
			}
		})
	}
}

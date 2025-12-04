package config

import (
	"reflect"
	"testing"

	env "github.com/caarlos0/env/v11"
)

func TestParseServices(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    map[ServiceMode]bool
		expectError bool
	}{
		{
			name:  "single service - http",
			input: "http",
			expected: map[ServiceMode]bool{
				ServiceModeHTTP: true,
			},
			expectError: false,
		},
		{
			name:  "single service - rules-engine",
			input: "rules-engine",
			expected: map[ServiceMode]bool{
				ServiceModeRulesEngine: true,
			},
			expectError: false,
		},
		{
			name:  "single service - scheduler",
			input: "scheduler",
			expected: map[ServiceMode]bool{
				ServiceModeScheduler: true,
			},
			expectError: false,
		},
		{
			name:  "multiple services - http and rules-engine",
			input: "http,rules-engine",
			expected: map[ServiceMode]bool{
				ServiceModeHTTP:        true,
				ServiceModeRulesEngine: true,
			},
			expectError: false,
		},
		{
			name:  "all services",
			input: "http,rules-engine,scheduler",
			expected: map[ServiceMode]bool{
				ServiceModeHTTP:        true,
				ServiceModeRulesEngine: true,
				ServiceModeScheduler:   true,
			},
			expectError: false,
		},
		{
			name:  "services with spaces",
			input: " http , rules-engine , scheduler ",
			expected: map[ServiceMode]bool{
				ServiceModeHTTP:        true,
				ServiceModeRulesEngine: true,
				ServiceModeScheduler:   true,
			},
			expectError: false,
		},
		{
			name:  "duplicate services",
			input: "http,http,rules-engine",
			expected: map[ServiceMode]bool{
				ServiceModeHTTP:        true,
				ServiceModeRulesEngine: true,
			},
			expectError: false,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "only spaces and commas",
			input:       " , , ",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "invalid service name",
			input:       "http,invalid-service",
			expected:    nil,
			expectError: true,
		},
		{
			name:        "mixed valid and invalid",
			input:       "http,rules-engine,invalid",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseServices(tt.input)

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d services, got %d", len(tt.expected), len(result))
				return
			}

			for service, expected := range tt.expected {
				if result[service] != expected {
					t.Errorf("expected service %s to be %v, got %v", service, expected, result[service])
				}
			}
		})
	}
}

func TestConfig_GetEnabledServices(t *testing.T) {
	tests := []struct {
		name        string
		services    string
		expected    map[ServiceMode]bool
		expectError bool
	}{
		{
			name:     "default configuration",
			services: "http",
			expected: map[ServiceMode]bool{
				ServiceModeHTTP: true,
			},
			expectError: false,
		},
		{
			name:     "multiple services",
			services: "http,rules-engine",
			expected: map[ServiceMode]bool{
				ServiceModeHTTP:        true,
				ServiceModeRulesEngine: true,
			},
			expectError: false,
		},
		{
			name:        "invalid configuration",
			services:    "invalid-service",
			expected:    nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AppConfig{Services: tt.services}
			result, err := cfg.GetEnabledServices()

			if tt.expectError {
				if err == nil {
					t.Errorf("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(result) != len(tt.expected) {
				t.Errorf("expected %d services, got %d", len(tt.expected), len(result))
				return
			}

			for service, expected := range tt.expected {
				if result[service] != expected {
					t.Errorf("expected service %s to be %v, got %v", service, expected, result[service])
				}
			}
		})
	}
}

func TestAppConfig_ParseAuthEnv(t *testing.T) {
	t.Setenv("AUTH_MODE", "oauth")
	t.Setenv("ADMIN_GROUP", "cn=admins,ou=groups,dc=example,dc=org")
	t.Setenv("USER_GROUP", "cn=users,ou=groups,dc=example,dc=org")
	t.Setenv("OAUTH_CLIENT_ID", "app-client")
	t.Setenv("OAUTH_CLIENT_SECRET", "super-secret")
	t.Setenv("OAUTH_REDIRECT_URL", "https://app.example.com/auth/callback")
	t.Setenv("OAUTH_DISCOVERY_URL", "https://login.example.com/.well-known/openid-configuration")
	t.Setenv("OAUTH_SCOPE", "openid profile email")
	t.Setenv("DEV_AUTH_USER_ID", "dev-user")
	t.Setenv("DEV_AUTH_EMAIL", "dev@example.com")
	t.Setenv("DEV_AUTH_GROUPS", "admins;devs")

	var cfg AppConfig
	if err := env.Parse(&cfg); err != nil {
		t.Fatalf("parse config: %v", err)
	}

	expected := AuthConfig{
		Mode: AuthModeOAuth,
		OAuth: OAuthConfig{
			ClientID:     "app-client",
			ClientSecret: "super-secret",
			RedirectURL:  "https://app.example.com/auth/callback",
			Scope:        "openid profile email",
			DiscoveryURL: "https://login.example.com/.well-known/openid-configuration",
		},
		DevAuth: DevAuthConfig{
			UserID: "dev-user",
			Email:  "dev@example.com",
			Groups: []string{"admins", "devs"},
		},
		AdminGroup: "cn=admins,ou=groups,dc=example,dc=org",
		UserGroup:  "cn=users,ou=groups,dc=example,dc=org",
	}

	if !reflect.DeepEqual(cfg.Auth, expected) {
		t.Fatalf("unexpected auth configuration:\nexpected: %#v\ngot:      %#v", expected, cfg.Auth)
	}
}

func TestConfig_ServiceEnabledMethods(t *testing.T) {
	tests := []struct {
		name                string
		services            string
		expectedHTTP        bool
		expectedRulesEngine bool
		expectedScheduler   bool
	}{
		{
			name:                "default - http only",
			services:            "http",
			expectedHTTP:        true,
			expectedRulesEngine: false,
			expectedScheduler:   false,
		},
		{
			name:                "http and rules-engine",
			services:            "http,rules-engine",
			expectedHTTP:        true,
			expectedRulesEngine: true,
			expectedScheduler:   false,
		},
		{
			name:                "all services",
			services:            "http,rules-engine,scheduler",
			expectedHTTP:        true,
			expectedRulesEngine: true,
			expectedScheduler:   true,
		},
		{
			name:                "rules-engine only",
			services:            "rules-engine",
			expectedHTTP:        false,
			expectedRulesEngine: true,
			expectedScheduler:   false,
		},
		{
			name:                "scheduler only",
			services:            "scheduler",
			expectedHTTP:        false,
			expectedRulesEngine: false,
			expectedScheduler:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := AppConfig{Services: tt.services}

			if cfg.IsHTTPServerEnabled() != tt.expectedHTTP {
				t.Errorf("IsHTTPServerEnabled(): expected %v, got %v", tt.expectedHTTP, cfg.IsHTTPServerEnabled())
			}

			if cfg.IsRulesEngineEnabled() != tt.expectedRulesEngine {
				t.Errorf(
					"IsRulesEngineEnabled(): expected %v, got %v",
					tt.expectedRulesEngine,
					cfg.IsRulesEngineEnabled(),
				)
			}

			if cfg.IsSchedulerEnabled() != tt.expectedScheduler {
				t.Errorf("IsSchedulerEnabled(): expected %v, got %v", tt.expectedScheduler, cfg.IsSchedulerEnabled())
			}
		})
	}
}

func TestConfig_ServiceEnabledMethodsWithInvalidConfig(t *testing.T) {
	cfg := AppConfig{Services: "invalid-service"}

	// All methods should return false when configuration is invalid
	if cfg.IsHTTPServerEnabled() != false {
		t.Errorf("IsHTTPServerEnabled() with invalid config: expected false, got true")
	}

	if cfg.IsRulesEngineEnabled() != false {
		t.Errorf("IsRulesEngineEnabled() with invalid config: expected false, got true")
	}

	if cfg.IsSchedulerEnabled() != false {
		t.Errorf("IsSchedulerEnabled() with invalid config: expected false, got true")
	}
}

func TestValidServiceModes(t *testing.T) {
	modes := ValidServiceModes()
	expected := []ServiceMode{
		ServiceModeHTTP,
		ServiceModeRulesEngine,
		ServiceModeScheduler,
		ServiceModeReaper,
		ServiceModeAlertRunner,
		ServiceModeSecretRefreshRunner,
	}

	if len(modes) != len(expected) {
		t.Errorf("expected %d service modes, got %d", len(expected), len(modes))
	}

	for i, mode := range modes {
		if mode != expected[i] {
			t.Errorf("expected service mode %s at index %d, got %s", expected[i], i, mode)
		}
	}
}

func TestObservabilityMetricsConfig_Sanitize(t *testing.T) {
	cfg := ObservabilityMetricsConfig{
		Enabled:       true,
		StatsdAddress: " ",
	}

	cfg.Sanitize()

	if cfg.Enabled {
		t.Fatalf("expected enabled to be false when address is empty")
	}

	cfg = ObservabilityMetricsConfig{
		Enabled:       true,
		StatsdAddress: " statsd:1234 ",
	}

	cfg.Sanitize()

	if !cfg.IsEnabled() {
		t.Fatalf("expected metrics to remain enabled")
	}
	if cfg.StatsdAddress != "statsd:1234" {
		t.Fatalf("expected address to be trimmed, got %q", cfg.StatsdAddress)
	}
}

func TestObservabilityNotificationsConfig_Sanitize(t *testing.T) {
	cfg := ObservabilityNotificationsConfig{
		Enabled:    true,
		Timeout:    0,
		RetryLimit: -1,
		Slack: SlackNotificationConfig{
			Enabled:    true,
			WebhookURL: " ",
			Channel:    "  ",
			Username:   "",
		},
		PagerDuty: PagerDutyNotificationConfig{
			Enabled:    true,
			RoutingKey: " ",
			Source:     "",
			Component:  "",
		},
	}

	cfg.Sanitize()

	if cfg.Timeout <= 0 {
		t.Fatalf("expected timeout to fall back to default, got %v", cfg.Timeout)
	}
	if cfg.RetryLimit < 0 {
		t.Fatalf("expected retry limit to be clamped to >= 0, got %d", cfg.RetryLimit)
	}
	if cfg.Slack.Enabled {
		t.Fatal("expected slack to be disabled without a webhook url")
	}
	if cfg.PagerDuty.Enabled {
		t.Fatal("expected pagerduty to be disabled without a routing key")
	}
	if cfg.PagerDuty.Source != "merrymaker" {
		t.Fatalf("expected pagerduty source default, got %q", cfg.PagerDuty.Source)
	}
	if cfg.PagerDuty.Component != "merrymaker" {
		t.Fatalf("expected pagerduty component default, got %q", cfg.PagerDuty.Component)
	}

	// Disabled top-level should disable child sinks.
	cfg = ObservabilityNotificationsConfig{
		Enabled: false,
		Slack: SlackNotificationConfig{
			Enabled:    true,
			WebhookURL: "https://hooks.slack.com/services/test",
		},
		PagerDuty: PagerDutyNotificationConfig{
			Enabled:    true,
			RoutingKey: "abc",
		},
	}
	cfg.Sanitize()

	if cfg.Slack.Enabled {
		t.Fatal("expected slack to be disabled when top-level notifications disabled")
	}
	if cfg.PagerDuty.Enabled {
		t.Fatal("expected pagerduty to be disabled when top-level notifications disabled")
	}
}

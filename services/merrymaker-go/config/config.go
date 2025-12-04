package config

import (
	"os"
	"strings"
)

// AppConfig is the main application configuration struct that composes
// domain-specific configuration from separate files.
//
// Configuration is loaded from environment variables using the
// github.com/caarlos0/env library. See individual domain config
// files for details on available environment variables:
//   - auth.go: Authentication configuration
//   - database.go: Database and cache configuration
//   - http.go: HTTP server configuration
//   - services.go: Service mode and worker configuration
type AppConfig struct {
	// IsDev controls development mode behavior (hot reloading, caching, etc.)
	// Set DEV=true or NODE_ENV=development for development mode.
	IsDev bool `env:"DEV" envDefault:"false"`

	// SecretsEncryptionKey is the encryption key for secrets storage.
	// Required for production, optional for development.
	SecretsEncryptionKey string `env:"SECRETS_ENCRYPTION_KEY"`

	// Authentication configuration
	Auth AuthConfig

	// Database configuration
	Postgres DBConfig    `envPrefix:"DB_"`
	Redis    RedisConfig `envPrefix:"REDIS_"`
	Cache    CacheConfig

	// HTTP server configuration
	HTTP HTTPConfig

	// Service mode configuration
	Services string `env:"SERVICES" envDefault:"http"`

	// Scheduler configuration
	Scheduler SchedulerConfig

	// Rules engine configuration
	RulesEngine RulesEngineConfig

	// Alert runner configuration
	AlertRunner AlertRunnerConfig

	// Secret refresh runner configuration
	SecretRefreshRunner SecretRefreshRunnerConfig

	// Reaper configuration
	Reaper ReaperConfig

	// Observability configuration
	Observability ObservabilityConfig
}

// Sanitize applies guardrails to configuration values loaded from env.
// This should be called after loading configuration from environment variables.
func (c *AppConfig) Sanitize() {
	// Sanitize HTTP server configuration
	c.HTTP.Sanitize()

	// Sanitize scheduler, rules engine, alert runner, secret refresh runner, and reaper configs
	c.Scheduler.Sanitize()
	c.RulesEngine.Sanitize()
	c.AlertRunner.Sanitize()
	c.SecretRefreshRunner.Sanitize()
	c.Reaper.Sanitize()
	c.Observability.Sanitize()

	// Check NODE_ENV for dev mode
	c.detectDevMode()
}

// detectDevMode checks both DEV and NODE_ENV environment variables.
// This is called by Sanitize() to ensure IsDev is set correctly.
// NODE_ENV is checked as a fallback (common in frontend tooling).
func (c *AppConfig) detectDevMode() {
	if !c.IsDev {
		nodeEnv := strings.ToLower(os.Getenv("NODE_ENV"))
		c.IsDev = nodeEnv == "development" || nodeEnv == "dev"
	}
}

// GetEnabledServices returns the enabled services based on the Services field.
func (c *AppConfig) GetEnabledServices() (map[ServiceMode]bool, error) {
	return ParseServices(c.Services)
}

// IsHTTPServerEnabled returns true if the HTTP server service is enabled.
func (c *AppConfig) IsHTTPServerEnabled() bool {
	services, err := c.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeHTTP]
}

// IsRulesEngineEnabled returns true if the rules engine service is enabled.
func (c *AppConfig) IsRulesEngineEnabled() bool {
	services, err := c.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeRulesEngine]
}

// IsSchedulerEnabled returns true if the scheduler service is enabled.
func (c *AppConfig) IsSchedulerEnabled() bool {
	services, err := c.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeScheduler]
}

// IsReaperEnabled returns true if the reaper service is enabled.
func (c *AppConfig) IsReaperEnabled() bool {
	services, err := c.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeReaper]
}

// IsAlertRunnerEnabled returns true if the alert runner service is enabled.
func (c *AppConfig) IsAlertRunnerEnabled() bool {
	services, err := c.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeAlertRunner]
}

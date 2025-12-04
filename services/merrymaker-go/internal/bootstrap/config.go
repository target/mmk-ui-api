package bootstrap

import (
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
	"github.com/target/mmk-ui-api/config"
)

// InitLogger initializes the structured logger.
func InitLogger() *slog.Logger {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)
	return logger
}

// LoadConfig loads configuration from environment variables.
func LoadConfig() (config.AppConfig, error) {
	// Load .env file if it exists (development)
	if err := godotenv.Load(); err != nil {
		var pathErr *os.PathError
		if !errors.As(err, &pathErr) {
			return config.AppConfig{}, fmt.Errorf("load .env file: %w", err)
		}
	}

	var cfg config.AppConfig
	if err := env.Parse(&cfg); err != nil {
		return cfg, fmt.Errorf("parse config: %w", err)
	}

	cfg.Sanitize()
	return cfg, nil
}

// ValidateServiceConfig validates that at least one service is enabled.
func ValidateServiceConfig(cfg *config.AppConfig) error {
	if cfg == nil {
		return errors.New("service config is required")
	}
	services, err := cfg.GetEnabledServices()
	if err != nil {
		return fmt.Errorf("invalid service configuration: %w", err)
	}

	if len(services) == 0 {
		return errors.New("no services enabled")
	}

	return nil
}

// GetEnabledServices returns a list of enabled service names.
func GetEnabledServices(cfg *config.AppConfig) []string {
	if cfg == nil {
		return []string{}
	}
	services, err := cfg.GetEnabledServices()
	if err != nil {
		// Return empty list on error - validation will catch this
		return []string{}
	}

	enabledServices := make([]string, 0, len(services))
	for svc := range services {
		switch svc {
		case config.ServiceModeHTTP:
			enabledServices = append(enabledServices, "http")
		case config.ServiceModeRulesEngine:
			enabledServices = append(enabledServices, "rules-engine")
		case config.ServiceModeScheduler:
			enabledServices = append(enabledServices, "scheduler")
		case config.ServiceModeAlertRunner:
			enabledServices = append(enabledServices, "alert-runner")
		case config.ServiceModeSecretRefreshRunner:
			enabledServices = append(enabledServices, "secret-refresh-runner")
		case config.ServiceModeReaper:
			enabledServices = append(enabledServices, "reaper")
		}
	}

	return enabledServices
}

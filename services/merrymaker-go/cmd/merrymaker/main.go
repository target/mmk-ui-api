package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/bootstrap"
)

func main() {
	ctx := context.Background()
	logger := bootstrap.InitLogger()
	if err := run(ctx, logger); err != nil {
		logger.ErrorContext(ctx, "fatal error", "error", err)
		os.Exit(1) //nolint:forbidigo // Main entrypoint should exit with non-zero status on fatal errors.
	}
}

func run(ctx context.Context, logger *slog.Logger) error {
	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		return err
	}

	// Log startup info
	logStartupInfo(ctx, logger, &cfg)

	cfgPtr := &cfg

	// Validate configuration
	if err = bootstrap.ValidateServiceConfig(cfgPtr); err != nil {
		return err
	}

	// Initialize infrastructure
	db, redisClient, err := initInfrastructure(ctx, &cfg, logger)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			logger.ErrorContext(ctx, "close database failed", "error", cerr)
		}
	}()
	if redisClient != nil {
		defer func() {
			if cerr := redisClient.Close(); cerr != nil {
				logger.ErrorContext(ctx, "close redis failed", "error", cerr)
			}
		}()
	}

	// Run migrations if enabled
	if cfg.Postgres.RunMigrationsOnStart {
		if err = bootstrap.RunMigrations(ctx, db, logger); err != nil {
			return err
		}
	} else {
		logger.InfoContext(ctx, "skipping database migrations on startup", "reason", "disabled via config")
	}

	// Initialize and run services
	services := bootstrap.NewServices(&bootstrap.ServiceDeps{
		Config:      cfgPtr,
		DB:          db,
		RedisClient: redisClient,
		Logger:      logger,
	})

	return bootstrap.RunServicesWithShutdown(&bootstrap.ServiceOrchestrationConfig{
		Config:      cfgPtr,
		Services:    services,
		DB:          db,
		RedisClient: redisClient,
		Logger:      logger,
	})
}

func logStartupInfo(ctx context.Context, logger *slog.Logger, cfg *config.AppConfig) {
	enabledServices := bootstrap.GetEnabledServices(cfg)
	logger.InfoContext(ctx, "starting merrymaker service",
		"db_host", cfg.Postgres.Host,
		"db_port", cfg.Postgres.Port,
		"db_name", cfg.Postgres.Name,
		"enabled_services", enabledServices)
}

// initInfrastructure connects shared dependencies used by the service runtime.
//
//nolint:ireturn // returning redis.UniversalClient keeps sentinel/cluster support flexible.
func initInfrastructure(
	ctx context.Context,
	cfg *config.AppConfig,
	logger *slog.Logger,
) (*sql.DB, redis.UniversalClient, error) {
	db, err := bootstrap.ConnectDB(bootstrap.DatabaseConfig{
		DBConfig:    cfg.Postgres,
		RedisConfig: cfg.Redis,
		Logger:      logger,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("connect db: %w", err)
	}

	redisClient, err := bootstrap.ConnectRedis(bootstrap.DatabaseConfig{
		DBConfig:    cfg.Postgres,
		RedisConfig: cfg.Redis,
		Logger:      logger,
	})
	if err != nil {
		if cerr := db.Close(); cerr != nil {
			logger.ErrorContext(ctx, "close database after redis connect failure", "error", cerr)
			return nil, nil, fmt.Errorf("connect redis: %w", errors.Join(err, fmt.Errorf("close database: %w", cerr)))
		}
		return nil, nil, fmt.Errorf("connect redis: %w", err)
	}

	return db, redisClient, nil
}

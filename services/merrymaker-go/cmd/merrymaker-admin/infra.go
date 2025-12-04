package main

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/bootstrap"
	"github.com/redis/go-redis/v9"
)

type connectInfraOptions struct {
	Logger    *slog.Logger
	Config    *config.AppConfig
	WantDB    bool
	WantRedis bool
}

var (
	errRedisNotConfigured = errors.New("redis not configured")
	errRedisNotWanted     = errors.New("redis not wanted")
)

// connectInfra wires up infrastructure dependencies based on CLI options.
//
//nolint:ireturn // returning redis.UniversalClient keeps sentinel/cluster support flexible.
func connectInfra(logger *slog.Logger, cfg *config.AppConfig) (*sql.DB, redis.UniversalClient, error) {
	return connectInfraWithOptions(&connectInfraOptions{
		Logger:    logger,
		Config:    cfg,
		WantDB:    true,
		WantRedis: true,
	})
}

// connectInfraWithOptions allows tests/commands to control which dependencies are created.
//
//nolint:ireturn // returning redis.UniversalClient keeps sentinel/cluster support flexible.
func connectInfraWithOptions(opts *connectInfraOptions) (*sql.DB, redis.UniversalClient, error) {
	var (
		db          *sql.DB
		err         error
		redisClient redis.UniversalClient
	)

	if opts.WantDB {
		db, err = bootstrap.ConnectDB(bootstrap.DatabaseConfig{DBConfig: opts.Config.Postgres, Logger: opts.Logger})
		if err != nil {
			return nil, nil, fmt.Errorf("connect db: %w", err)
		}
	}

	redisClient, err = attachRedisClient(&attachRedisClientRequest{
		Logger:    opts.Logger,
		Config:    &opts.Config.Redis,
		DB:        db,
		WantRedis: opts.WantRedis,
	})
	if err != nil && !errors.Is(err, errRedisNotWanted) && !errors.Is(err, errRedisNotConfigured) {
		return nil, nil, err
	}

	return db, redisClient, nil
}

type attachRedisClientRequest struct {
	Logger    *slog.Logger
	Config    *config.RedisConfig
	DB        *sql.DB
	WantRedis bool
}

// attachRedisClient attaches a Redis client when configuration and CLI flags request it.
//
//nolint:ireturn // returning redis.UniversalClient keeps sentinel/cluster support flexible.
func attachRedisClient(req *attachRedisClientRequest) (redis.UniversalClient, error) {
	if !req.WantRedis {
		return nil, errRedisNotWanted
	}

	client, err := maybeConnectRedis(req.Logger, req.Config)
	if err == nil {
		return client, nil
	}

	if errors.Is(err, errRedisNotConfigured) {
		req.Logger.Info("no redis configuration detected; skipping redis connection")
		return nil, errRedisNotConfigured
	}

	if req.DB != nil {
		if closeErr := req.DB.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close db: %w", closeErr))
		}
	}
	return nil, err
}

// maybeConnectRedis returns a connected client when configuration is present.
//
//nolint:ireturn // returning redis.UniversalClient keeps sentinel/cluster support flexible.
func maybeConnectRedis(logger *slog.Logger, cfg *config.RedisConfig) (redis.UniversalClient, error) {
	if !hasRedisConfig(cfg) {
		return nil, errRedisNotConfigured
	}
	client, err := bootstrap.ConnectRedis(bootstrap.DatabaseConfig{RedisConfig: *cfg, Logger: logger})
	if err != nil {
		return nil, fmt.Errorf("connect redis: %w", err)
	}
	return client, nil
}

func hasRedisConfig(cfg *config.RedisConfig) bool {
	if cfg == nil {
		return false
	}
	if cfg.UseCluster {
		return len(cfg.ClusterNodes) > 0 || cfg.URI != ""
	}
	if cfg.UseSentinel {
		return len(cfg.SentinelNodes) > 0
	}
	return cfg.URI != ""
}

func closeInfra(db *sql.DB, redisClient redis.UniversalClient) error {
	var closeErr error
	if db != nil {
		if err := db.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close db: %w", err))
		}
	}
	if redisClient != nil {
		if err := redisClient.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close redis: %w", err))
		}
	}
	return closeErr
}

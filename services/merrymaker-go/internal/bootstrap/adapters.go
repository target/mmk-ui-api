package bootstrap

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/adapters/jobrunner"
	"github.com/target/mmk-ui-api/internal/adapters/reaper"
	"github.com/target/mmk-ui-api/internal/adapters/rulesrunner"
	schedrunner "github.com/target/mmk-ui-api/internal/adapters/scheduler"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/observability/statsd"
	"github.com/target/mmk-ui-api/internal/service/failurenotifier"
	"github.com/target/mmk-ui-api/internal/service/rules"
)

// RulesEngineConfig contains configuration for rules engine.
type RulesEngineConfig struct {
	DB              *sql.DB
	RedisClient     redis.UniversalClient
	Logger          *slog.Logger
	Lease           time.Duration
	Concurrency     int
	CacheMetrics    rules.CacheMetrics
	Metrics         statsd.Sink
	FailureNotifier *failurenotifier.Service
}

// RunRulesEngine starts the rules engine service.
func RunRulesEngine(ctx context.Context, cfg RulesEngineConfig) error {
	runner, err := rulesrunner.NewRunner(rulesrunner.RunnerOptions{
		DB:              cfg.DB,
		RedisClient:     cfg.RedisClient,
		Logger:          cfg.Logger,
		Lease:           cfg.Lease,
		Concurrency:     cfg.Concurrency,
		CacheMetrics:    cfg.CacheMetrics,
		Metrics:         cfg.Metrics,
		FailureNotifier: cfg.FailureNotifier,
	})
	if err != nil {
		return fmt.Errorf("create rules runner: %w", err)
	}

	return runner.Run(ctx)
}

//nolint:ireturn // Returning Encryptor interface is required for runner injection.
func resolveEncryptor(enc cryptoutil.Encryptor, logger *slog.Logger) cryptoutil.Encryptor {
	if enc != nil {
		return enc
	}
	if logger != nil {
		logger.Warn("no encryptor provided; using noop encryptor")
	}
	return &cryptoutil.NoopEncryptor{}
}

// runJobRunner centralizes job runner setup so individual runners only pass job-specific options.
// Example usage:
//
//	return runJobRunner(ctx, jobrunner.RunnerOptions{
//		DB:         cfg.DB,
//		Logger:     cfg.Logger,
//		Lease:      cfg.Lease,
//		Concurrency: cfg.Concurrency,
//		JobType:    model.JobTypeAlert,
//	})
func runJobRunner(ctx context.Context, opts jobrunner.RunnerOptions) error {
	opts.Encryptor = resolveEncryptor(opts.Encryptor, opts.Logger)

	label := jobRunnerLabel(opts.JobType)

	runner, err := jobrunner.NewRunner(opts)
	if err != nil {
		return fmt.Errorf("create %s runner: %w", label, err)
	}

	if runErr := runner.Run(ctx); runErr != nil {
		return fmt.Errorf("run %s runner: %w", label, runErr)
	}
	return nil
}

func jobRunnerLabel(jobType model.JobType) string {
	switch jobType {
	case model.JobTypeAlert:
		return "alert"
	case model.JobTypeSecretRefresh:
		return "secret refresh"
	case model.JobTypeBrowser:
		return "browser"
	case model.JobTypeRules:
		return "rules"
	}

	if jobType == "" {
		return "job"
	}
	return strings.ToLower(strings.ReplaceAll(string(jobType), "_", " "))
}

// AlertRunnerConfig contains configuration for alert runner.
type AlertRunnerConfig struct {
	DB              *sql.DB
	Logger          *slog.Logger
	Lease           time.Duration
	Concurrency     int
	Encryptor       cryptoutil.Encryptor
	Metrics         statsd.Sink
	FailureNotifier *failurenotifier.Service
}

// RunAlertRunner starts the alert runner service for HTTP alert dispatch.
func RunAlertRunner(ctx context.Context, cfg AlertRunnerConfig) error {
	return runJobRunner(ctx, jobrunner.RunnerOptions{
		DB:              cfg.DB,
		Logger:          cfg.Logger,
		Lease:           cfg.Lease,
		Concurrency:     cfg.Concurrency,
		JobType:         model.JobTypeAlert,
		Encryptor:       cfg.Encryptor,
		Metrics:         cfg.Metrics,
		FailureNotifier: cfg.FailureNotifier,
	})
}

// SecretRefreshRunnerConfig contains configuration for secret refresh runner.
type SecretRefreshRunnerConfig struct {
	DB              *sql.DB
	Logger          *slog.Logger
	Lease           time.Duration
	Concurrency     int
	DebugMode       bool
	Encryptor       cryptoutil.Encryptor
	Metrics         statsd.Sink
	FailureNotifier *failurenotifier.Service
}

// RunSecretRefreshRunner starts the secret refresh runner service for dynamic secret refresh.
func RunSecretRefreshRunner(ctx context.Context, cfg SecretRefreshRunnerConfig) error {
	return runJobRunner(ctx, jobrunner.RunnerOptions{
		DB:                     cfg.DB,
		Logger:                 cfg.Logger,
		Lease:                  cfg.Lease,
		Concurrency:            cfg.Concurrency,
		JobType:                model.JobTypeSecretRefresh,
		SecretRefreshDebugMode: cfg.DebugMode,
		Encryptor:              cfg.Encryptor,
		Metrics:                cfg.Metrics,
		FailureNotifier:        cfg.FailureNotifier,
	})
}

// SchedulerConfig contains configuration for scheduler.
type SchedulerConfig struct {
	DB                 *sql.DB
	RedisClient        redis.UniversalClient
	Logger             *slog.Logger
	BatchSize          int
	DefaultJobType     model.JobType
	DefaultPriority    int
	MaxRetries         int
	OverrunPolicy      domain.OverrunPolicy
	OverrunStates      domain.OverrunStateMask
	Interval           time.Duration
	SourceCacheEnabled bool
	SourceCacheTTL     time.Duration
	Encryptor          cryptoutil.Encryptor
	Metrics            statsd.Sink
}

// RunScheduler starts the scheduler service.
func RunScheduler(ctx context.Context, cfg SchedulerConfig) error {
	schedulerCfg := core.SchedulerConfig{
		BatchSize:       cfg.BatchSize,
		DefaultJobType:  cfg.DefaultJobType,
		DefaultPriority: cfg.DefaultPriority,
		MaxRetries:      cfg.MaxRetries,
		Strategy: domain.StrategyOptions{
			Overrun:       cfg.OverrunPolicy,
			OverrunStates: cfg.OverrunStates,
		},
	}

	opts := schedrunner.RunnerOptions{
		DB:         cfg.DB,
		Config:     &schedulerCfg,
		Interval:   cfg.Interval,
		Logger:     slog.NewLogLogger(cfg.Logger.Handler(), slog.LevelInfo),
		SlogLogger: cfg.Logger,
		Metrics:    cfg.Metrics,
	}

	// Wire optional source repository when DB is available
	if cfg.DB != nil {
		opts.Sources = data.NewSourceRepo(cfg.DB)
		if cfg.Encryptor != nil {
			opts.Secrets = data.NewSecretRepo(cfg.DB, cfg.Encryptor)
		}
	}

	// Wire optional source cache when Redis client is available
	if cfg.SourceCacheEnabled && cfg.RedisClient != nil {
		opts.Cache = data.NewRedisCacheRepo(cfg.RedisClient)
		if cfg.SourceCacheTTL > 0 {
			opts.CacheConfig = &core.SourceCacheConfig{TTL: cfg.SourceCacheTTL}
		}
	}

	runner, err := schedrunner.NewRunner(opts)
	if err != nil {
		return fmt.Errorf("create scheduler runner: %w", err)
	}

	return runner.Run(ctx)
}

// ReaperConfig contains configuration for reaper.
type ReaperConfig struct {
	DB      *sql.DB
	Logger  *slog.Logger
	Config  config.ReaperConfig
	Metrics statsd.Sink
}

// RunReaper starts the reaper service.
func RunReaper(ctx context.Context, cfg ReaperConfig) error {
	runner, err := reaper.NewRunner(reaper.RunnerOptions{
		DB:      cfg.DB,
		Config:  cfg.Config,
		Logger:  cfg.Logger,
		Metrics: cfg.Metrics,
	})
	if err != nil {
		return fmt.Errorf("create reaper runner: %w", err)
	}

	return runner.Run(ctx)
}

// Package reaper provides adapters for running the job reaper.
package reaper

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/observability/statsd"
	"github.com/target/mmk-ui-api/internal/service"
)

// Runner provides a simple adapter to run the reaper loop.
// It constructs the reaper service and runs the cleanup loop.
type Runner struct {
	reaper  *service.ReaperService
	logger  *slog.Logger
	metrics statsd.Sink
}

// RunnerOptions holds the dependencies for creating a Runner.
type RunnerOptions struct {
	DB     *sql.DB
	Config config.ReaperConfig
	Logger *slog.Logger

	// Optional dependency injection for testing/decoupling
	Repo    core.ReaperRepository
	Metrics statsd.Sink
}

// NewRunner creates a new reaper runner with the given options.
func NewRunner(opts RunnerOptions) (*Runner, error) {
	if err := validateRunnerOptions(&opts); err != nil {
		return nil, err
	}

	reaper, err := wireReaperService(opts)
	if err != nil {
		return nil, fmt.Errorf("wire reaper service: %w", err)
	}

	return &Runner{
		reaper:  reaper,
		logger:  opts.Logger,
		metrics: opts.Metrics,
	}, nil
}

// validateRunnerOptions validates and sets defaults for RunnerOptions.
func validateRunnerOptions(opts *RunnerOptions) error {
	if opts.DB == nil {
		return errors.New("database connection is required")
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return nil
}

// wireReaperService wires up all dependencies for the reaper service.
func wireReaperService(opts RunnerOptions) (*service.ReaperService, error) {
	// Reaper repository
	var repo core.ReaperRepository
	if opts.Repo != nil {
		repo = opts.Repo
	} else {
		// Wire up the repository inline to avoid ireturn linter issue
		jobRepo := data.NewJobRepo(opts.DB, data.RepoConfig{})
		repo = &reaperRepoAdapter{r: jobRepo}
	}

	// Use NewReaperService instead of Must to allow error propagation
	return service.NewReaperService(service.ReaperServiceOptions{
		Repo:    repo,
		Config:  opts.Config,
		Logger:  opts.Logger,
		Metrics: opts.Metrics,
	})
}

// Run starts the reaper loop and runs until the context is cancelled.
func (r *Runner) Run(ctx context.Context) error {
	r.logger.InfoContext(ctx, "starting reaper runner")
	return r.reaper.Run(ctx)
}

// reaperRepoAdapter adapts JobRepo to implement ReaperRepository interface.
type reaperRepoAdapter struct {
	r *data.JobRepo
}

func (a *reaperRepoAdapter) FailStalePendingJobs(
	ctx context.Context,
	maxAge time.Duration,
	batchSize int,
) (int64, error) {
	return a.r.FailStalePendingJobs(ctx, maxAge, batchSize)
}

func (a *reaperRepoAdapter) DeleteOldJobs(ctx context.Context, params core.DeleteOldJobsParams) (int64, error) {
	return a.r.DeleteOldJobs(ctx, params)
}

func (a *reaperRepoAdapter) DeleteOldJobResults(
	ctx context.Context,
	params core.DeleteOldJobResultsParams,
) (int64, error) {
	return a.r.DeleteOldJobResults(ctx, params)
}

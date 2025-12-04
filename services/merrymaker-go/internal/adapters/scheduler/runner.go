// Package scheduler provides adapters for running the job scheduler.
package scheduler

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"log/slog"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
	obserrors "github.com/target/mmk-ui-api/internal/observability/errors"
	"github.com/target/mmk-ui-api/internal/observability/metrics"
	"github.com/target/mmk-ui-api/internal/observability/statsd"
	"github.com/target/mmk-ui-api/internal/service"
)

// Runner provides a simple adapter to run the scheduler loop.
// It constructs the scheduler service and runs a tick loop with configurable interval.
type Runner struct {
	scheduler core.JobScheduler
	interval  time.Duration
	logger    *log.Logger
	metrics   statsd.Sink
}

// RunnerOptions holds the dependencies for creating a Runner.
type RunnerOptions struct {
	DB         *sql.DB
	Config     *core.SchedulerConfig
	Interval   time.Duration
	Logger     *log.Logger
	SlogLogger *slog.Logger
	Metrics    statsd.Sink

	// Optional dependency injections for testing/decoupling
	Jobs            core.JobRepository
	Scheduled       core.ScheduledJobsRepository
	JobIntrospector core.JobIntrospector

	// Optional caching dependencies
	Cache       core.CacheRepository
	Sources     core.SourceRepository
	Secrets     core.SecretRepository
	CacheConfig *core.SourceCacheConfig
}

// NewRunner creates a new scheduler runner with the given options.
func NewRunner(opts RunnerOptions) (*Runner, error) {
	if err := validateRunnerOptions(&opts); err != nil {
		return nil, err
	}

	deps := wireRunnerDependencies(opts)
	scheduler := service.NewSchedulerService(deps)

	return &Runner{
		scheduler: scheduler,
		interval:  opts.Interval,
		logger:    opts.Logger,
		metrics:   opts.Metrics,
	}, nil
}

// validateRunnerOptions validates and sets defaults for RunnerOptions.
func validateRunnerOptions(opts *RunnerOptions) error {
	if opts.DB == nil {
		return errors.New("database connection is required")
	}
	if opts.Interval <= 0 {
		opts.Interval = 1 * time.Second // Default to 1 second
	}
	if opts.Logger == nil {
		opts.Logger = log.Default()
	}
	if opts.SlogLogger == nil {
		opts.SlogLogger = slog.Default()
	}
	return nil
}

// wireRunnerDependencies wires up all dependencies for the scheduler service.
func wireRunnerDependencies(opts RunnerOptions) service.SchedulerServiceOptions {
	// Jobs repository
	var jobs core.JobRepository
	if opts.Jobs != nil {
		jobs = opts.Jobs
	} else {
		jobs = wireJobRepository(opts)
	}

	// Scheduled jobs repository
	var scheduled core.ScheduledJobsRepository
	if opts.Scheduled != nil {
		scheduled = opts.Scheduled
	} else {
		scheduled = wireScheduledJobsRepository(opts)
	}

	// Job introspector
	var ji core.JobIntrospector
	if opts.JobIntrospector != nil {
		ji = opts.JobIntrospector
	} else if x, ok := jobs.(core.JobIntrospector); ok {
		ji = x
	} else {
		ji = wireJobIntrospector(opts, jobs)
	}

	sourceCache := wireSourceCacheService(opts)

	return service.SchedulerServiceOptions{
		Repo:            scheduled,
		Jobs:            jobs,
		JobIntrospector: ji,
		Config:          opts.Config,
		SourceCache:     sourceCache,
		Logger:          opts.SlogLogger,
	}
}

// wireJobRepository wires up the job repository dependency.
// Returns a concrete adapter type to satisfy ireturn linter.
func wireJobRepository(opts RunnerOptions) *jobRepoAdapter {
	return &jobRepoAdapter{r: data.NewJobRepo(opts.DB, data.RepoConfig{})}
}

// wireScheduledJobsRepository wires up the scheduled jobs repository dependency.
// Returns a concrete adapter type to satisfy ireturn linter.
func wireScheduledJobsRepository(opts RunnerOptions) *scheduledJobsRepoAdapter {
	return &scheduledJobsRepoAdapter{r: data.NewScheduledJobsRepo(opts.DB)}
}

// wireJobIntrospector wires up the job introspector dependency.
// Returns a concrete adapter type to satisfy ireturn linter. Caller decides if this is needed.
func wireJobIntrospector(opts RunnerOptions, _ core.JobRepository) *jobIntrospectorAdapter {
	return &jobIntrospectorAdapter{r: data.NewJobRepo(opts.DB, data.RepoConfig{})}
}

// wireSourceCacheService wires up the optional source cache service.
func wireSourceCacheService(opts RunnerOptions) *core.SourceCacheService {
	if opts.Cache == nil || opts.Sources == nil {
		return nil
	}

	cacheConfig := core.DefaultSourceCacheConfig()
	if opts.CacheConfig != nil {
		cacheConfig = *opts.CacheConfig
	}
	return core.NewSourceCacheService(core.SourceCacheServiceOptions{
		Cache:   opts.Cache,
		Sources: opts.Sources,
		Secrets: opts.Secrets,
		Config:  cacheConfig,
	})
}

// Run starts the scheduler loop and runs until the context is cancelled.
// It calls Tick() at the configured interval and logs the results.
func (r *Runner) Run(ctx context.Context) error {
	r.logger.Printf("Starting scheduler runner with interval %v", r.interval)

	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Printf("Scheduler runner stopping: %v", ctx.Err())
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()

		case now := <-ticker.C:
			start := time.Now()
			processed, err := r.scheduler.Tick(ctx, now)
			elapsed := time.Since(start)

			r.emitTickMetrics(processed, elapsed, err)

			if err != nil {
				r.logger.Printf("Scheduler tick error: %v", err)
				// Continue running despite errors
			} else if processed > 0 {
				r.logger.Printf("Scheduler processed %d tasks", processed)
			}
		}
	}
}

func (r *Runner) emitTickMetrics(processed int, elapsed time.Duration, err error) {
	if r.metrics == nil {
		return
	}

	result := metrics.ResultSuccess
	if err != nil {
		result = metrics.ResultError
	} else if processed == 0 {
		result = metrics.ResultNoop
	}

	tags := map[string]string{
		"result": result,
	}

	if err != nil {
		if class := obserrors.Classify(err); class != "" {
			tags["error_class"] = class
		}
	}

	r.metrics.Count("scheduler.tick", 1, tags)

	if processed > 0 {
		r.metrics.Count("scheduler.tasks_enqueued", int64(processed), tags)
	}

	if elapsed > 0 {
		r.metrics.Timing("scheduler.tick_duration", elapsed, metrics.CloneTags(tags))
	}

	if err == nil {
		r.metrics.Gauge("scheduler.last_success_epoch", float64(time.Now().Unix()), nil)
	}
}

// Adapter implementations to bridge data layer to core interfaces

type jobRepoAdapter struct{ r *data.JobRepo }

func (a *jobRepoAdapter) Create(ctx context.Context, req *model.CreateJobRequest) (*model.Job, error) {
	return a.r.Create(ctx, req)
}

func (a *jobRepoAdapter) GetByID(ctx context.Context, id string) (*model.Job, error) {
	return a.r.GetByID(ctx, id)
}

func (a *jobRepoAdapter) ReserveNext(ctx context.Context, jobType model.JobType, leaseSeconds int) (*model.Job, error) {
	return a.r.ReserveNext(ctx, jobType, leaseSeconds)
}

func (a *jobRepoAdapter) WaitForNotification(ctx context.Context, jobType model.JobType) error {
	return a.r.WaitForNotification(ctx, jobType)
}

func (a *jobRepoAdapter) Heartbeat(ctx context.Context, jobID string, leaseSeconds int) (bool, error) {
	return a.r.Heartbeat(ctx, jobID, leaseSeconds)
}

func (a *jobRepoAdapter) Complete(ctx context.Context, id string) (bool, error) {
	return a.r.Complete(ctx, id)
}

func (a *jobRepoAdapter) Fail(ctx context.Context, id, errMsg string) (bool, error) {
	return a.r.Fail(ctx, id, errMsg)
}

func (a *jobRepoAdapter) Stats(ctx context.Context, jobType model.JobType) (*model.JobStats, error) {
	return a.r.Stats(ctx, jobType)
}

func (a *jobRepoAdapter) List(ctx context.Context, opts *model.JobListOptions) ([]*model.JobWithEventCount, error) {
	return a.r.List(ctx, opts)
}

func (a *jobRepoAdapter) Delete(ctx context.Context, id string) error {
	return a.r.Delete(ctx, id)
}

func (a *jobRepoAdapter) DeleteByPayloadField(
	ctx context.Context,
	params core.DeleteByPayloadFieldParams,
) (int, error) {
	return a.r.DeleteByPayloadField(ctx, params)
}

type scheduledJobsRepoAdapter struct{ r *data.ScheduledJobsRepo }

func (a *scheduledJobsRepoAdapter) FindDue(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]domain.ScheduledTask, error) {
	return a.r.FindDue(ctx, now, limit)
}

func (a *scheduledJobsRepoAdapter) FindDueTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.FindDueParams,
) ([]domain.ScheduledTask, error) {
	return a.r.FindDueTx(ctx, tx, p)
}

func (a *scheduledJobsRepoAdapter) MarkQueued(ctx context.Context, id string, now time.Time) (bool, error) {
	return a.r.MarkQueued(ctx, id, now)
}

func (a *scheduledJobsRepoAdapter) MarkQueuedTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.MarkQueuedParams,
) (bool, error) {
	return a.r.MarkQueuedTx(ctx, tx, p)
}

func (a *scheduledJobsRepoAdapter) TryWithTaskLock(
	ctx context.Context,
	taskName string,
	fn func(context.Context, *sql.Tx) error,
) (bool, error) {
	return a.r.TryWithTaskLock(ctx, taskName, fn)
}

func (a *scheduledJobsRepoAdapter) UpdateActiveFireKeyTx(
	ctx context.Context,
	tx *sql.Tx,
	p domain.UpdateActiveFireKeyParams,
) error {
	return a.r.UpdateActiveFireKeyTx(ctx, tx, p)
}

type jobIntrospectorAdapter struct{ r *data.JobRepo }

func (a *jobIntrospectorAdapter) RunningJobExistsByTaskName(
	ctx context.Context,
	taskName string,
	now time.Time,
) (bool, error) {
	return a.r.RunningJobExistsByTaskName(ctx, taskName, now)
}

func (a *jobIntrospectorAdapter) JobStatesByTaskName(
	ctx context.Context,
	taskName string,
	now time.Time,
) (domain.OverrunStateMask, error) {
	return a.r.JobStatesByTaskName(ctx, taskName, now)
}

// Example usage:
//
//	func main() {
//		db, err := sql.Open("postgres", "postgres://...")
//		if err != nil {
//			log.Fatal(err)
//		}
//		defer db.Close()
//
//		cfg := core.DefaultSchedulerConfig()
//		cfg.Strategy.Overrun = domain.OverrunPolicySkip
//		cfg.BatchSize = 50
//
//		runner, err := scheduler.NewRunner(scheduler.RunnerOptions{
//			DB:       db,
//			Config:   &cfg,
//			Interval: 5 * time.Second,
//		})
//		if err != nil {
//			log.Fatal(err)
//		}
//
//		ctx, cancel := context.WithCancel(context.Background())
//		defer cancel()
//
//		// Run scheduler in background
//		go func() {
//			if err := runner.Run(ctx); err != nil && err != context.Canceled {
//				log.Printf("Scheduler error: %v", err)
//			}
//		}()
//
//		// Your application logic here...
//		select {}
//	}

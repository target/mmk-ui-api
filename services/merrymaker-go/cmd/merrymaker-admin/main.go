package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/bootstrap"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/devseed"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/util"
)

type commandFn func(ctx *commandContext, args []string) error

type command struct {
	name        string
	description string
	run         commandFn
}

type commandContext struct {
	Ctx    context.Context
	Logger *slog.Logger
	Config config.AppConfig
}

const defaultMigrationTimeout = 5 * time.Minute

func main() {
	logger := bootstrap.InitLogger()

	if len(os.Args) < 2 {
		if err := printUsage(); err != nil {
			logger.Error("print usage failed", "error", err)
		}
		os.Exit(2) //nolint:forbidigo // CLI must exit with failure status when no command is provided
	}

	cmdName := os.Args[1]
	cmd, ok := commands()[cmdName]
	if !ok {
		if err := writef(os.Stderr, "unknown command %q\n\n", cmdName); err != nil {
			logger.Error("print unknown command message failed", "error", err)
		}
		if err := printUsage(); err != nil {
			logger.Error("print usage failed", "error", err)
		}
		os.Exit(2) //nolint:forbidigo // CLI must exit with failure status when command is unknown
	}

	cfg, err := bootstrap.LoadConfig()
	if err != nil {
		logger.ErrorContext(context.Background(), "load config", "error", err)
		os.Exit(1) //nolint:forbidigo // CLI must signal configuration load failure to shell scripts
	}

	cmdCtx := &commandContext{
		Ctx:    context.Background(),
		Logger: logger,
		Config: cfg,
	}
	if runErr := cmd.run(cmdCtx, os.Args[2:]); runErr != nil {
		logger.ErrorContext(cmdCtx.Ctx, "command failed", "command", cmdName, "error", runErr)
		os.Exit(1) //nolint:forbidigo // CLI must propagate command execution failure to callers
	}
}

func commands() map[string]command {
	return map[string]command{
		"db-reset": {
			name:        "db-reset",
			description: "Drop the database schema, run migrations, and optionally seed data",
			run:         runDBReset,
		},
		"db-seed": {
			name:        "db-seed",
			description: "Run database migrations and seed development data",
			run:         runDBSeed,
		},
		"migrate": {
			name:        "migrate",
			description: "Run database migrations",
			run:         runMigrations,
		},
		"clear-seen-domains": {
			name:        "clear-seen-domains",
			description: "Clear seen domain cache (Postgres + Redis)",
			run:         runClearSeenDomains,
		},
		"list-seen-domains": {
			name:        "list-seen-domains",
			description: "Inspect seen domain records and Redis cache entries",
			run:         runListSeenDomains,
		},
		"list-alertonce-keys": {
			name:        "list-alertonce-keys",
			description: "Inspect alert-once deduplication keys in Redis",
			run:         runListAlertOnceKeys,
		},
		"clear-alertonce-keys": {
			name:        "clear-alertonce-keys",
			description: "Clear alert-once deduplication keys from Redis",
			run:         runClearAlertOnceKeys,
		},
		"fire-http-alert": {
			name:        "fire-http-alert",
			description: "Manually create and dispatch an HTTP alert to configured sinks",
			run:         runFireHTTPAlert,
		},
		"rules-job-results": {
			name:        "rules-job-results",
			description: "Inspect cached rules job results for a specific job",
			run:         runRulesJobResults,
		},
	}
}

func printUsage() error {
	if err := writef(os.Stdout, "Usage: merrymaker-admin <command> [flags]\n\n"); err != nil {
		return err
	}
	if err := writef(os.Stdout, "Available commands:\n"); err != nil {
		return err
	}
	for _, c := range commands() {
		if err := writef(os.Stdout, "  %-24s %s\n", c.name, c.description); err != nil {
			return err
		}
	}
	return nil
}

type clearOptions struct {
	SiteID string
	Scope  string
	Domain string
	All    bool
	DryRun bool
	Yes    bool
}

type listOptions struct {
	SiteID           string
	Scope            string
	Domain           string
	All              bool
	Limit            int
	Offset           int
	DBOnly           bool
	RedisOnly        bool
	IncludeAlertOnce bool
}

type alertOnceClearOptions struct {
	SiteID string
	Scope  string
	Domain string
	All    bool
	DryRun bool
	Yes    bool
}

type rulesResultsOptions struct {
	JobID   string
	RawJSON bool
}

type migrateOptions struct {
	Timeout time.Duration
}

type dbResetOptions struct {
	Timeout     time.Duration
	Yes         bool
	Seed        bool
	AllowRemote bool
}

type dbSeedOptions struct {
	Timeout     time.Duration
	AllowRemote bool
}

func runClearSeenDomains(cmdCtx *commandContext, args []string) error {
	opts, err := parseClearFlags(args)
	if err != nil {
		return err
	}
	if confirmErr := confirmClear(opts); confirmErr != nil {
		return confirmErr
	}

	ctx, cancel := context.WithTimeout(cmdCtx.Ctx, 2*time.Minute)
	defer cancel()

	db, redisClient, err := connectInfra(cmdCtx.Logger, &cmdCtx.Config)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			cmdCtx.Logger.Warn("db close failed", "error", closeErr)
		}
	}()
	if redisClient != nil {
		defer func() {
			if closeErr := redisClient.Close(); closeErr != nil {
				cmdCtx.Logger.Warn("redis close failed", "error", closeErr)
			}
		}()
	}

	rows, err := deleteSeenDomainRows(&deleteSeenDomainRequest{
		Ctx:     ctx,
		DB:      db,
		Logger:  cmdCtx.Logger,
		Options: opts,
	})
	if err != nil {
		return err
	}

	if redisClient != nil {
		if purgeErr := purgeSeenRedis(&purgeSeenRedisRequest{
			Ctx:     ctx,
			Client:  redisClient,
			Logger:  cmdCtx.Logger,
			Options: opts,
		}); purgeErr != nil {
			return purgeErr
		}
	}

	cmdCtx.Logger.Info("clear seen domains complete", "rows_deleted", rows)
	return nil
}

func runListSeenDomains(cmdCtx *commandContext, args []string) error {
	opts, err := parseListFlags(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmdCtx.Ctx, 2*time.Minute)
	defer cancel()

	conns, warnings, err := openListConnections(cmdCtx, &opts)
	if err != nil {
		return err
	}
	defer func() {
		if cerr := conns.Close(); cerr != nil {
			cmdCtx.Logger.Warn("close list connections failed", "error", cerr)
		}
	}()

	fetchReq := &fetchSeenDomainsRequest{
		Ctx:     ctx,
		Logger:  cmdCtx.Logger,
		Options: &opts,
	}
	results, err := gatherListResults(fetchReq, conns)
	if err != nil {
		return err
	}

	if renderErr := renderListResults(&renderListResultsRequest{
		Conns:    conns,
		Warnings: warnings,
		Results:  results,
		Options:  &opts,
	}); renderErr != nil {
		return renderErr
	}

	return nil
}

func runListAlertOnceKeys(cmdCtx *commandContext, _ []string) error {
	ctx, cancel := context.WithTimeout(cmdCtx.Ctx, 2*time.Minute)
	defer cancel()

	_, redisClient, err := connectInfra(cmdCtx.Logger, &cmdCtx.Config)
	if err != nil {
		return err
	}
	if redisClient == nil {
		if writeErr := writeln(os.Stderr, "Redis client is not available"); writeErr != nil {
			return fmt.Errorf("print redis availability: %w", writeErr)
		}
		return nil
	}
	defer func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			cmdCtx.Logger.Warn("redis close failed", "error", closeErr)
		}
	}()

	pattern := "rules:alertonce:*"
	cmdCtx.Logger.Info("scanning redis", "pattern", pattern)

	iter := redisClient.Scan(ctx, 0, pattern, 100).Iterator()
	if headerErr := writef(os.Stdout, "\nAlert-Once Keys in Redis\n"); headerErr != nil {
		return fmt.Errorf("print alert-once header: %w", headerErr)
	}

	total, iterErr := writeAlertOnceKeys(alertOnceScanInput{
		Ctx:    ctx,
		Iter:   iter,
		Client: redisClient,
		Logger: cmdCtx.Logger,
	})
	if iterErr != nil {
		return iterErr
	}

	if total == 0 {
		if nonePrintErr := writeln(os.Stdout, "(no keys found)"); nonePrintErr != nil {
			return fmt.Errorf("print alert-once none: %w", nonePrintErr)
		}
		return nil
	}

	if totalPrintErr := writef(os.Stdout, "\nTotal keys: %d\n", total); totalPrintErr != nil {
		return fmt.Errorf("print alert-once total: %w", totalPrintErr)
	}
	return nil
}

type alertOnceScanInput struct {
	Ctx    context.Context
	Iter   *redis.ScanIterator
	Client redis.UniversalClient
	Logger *slog.Logger
}

func writeAlertOnceKeys(input alertOnceScanInput) (int, error) {
	if input.Iter == nil {
		return 0, errors.New("redis scan: nil iterator")
	}

	printer := alertOnceKeyPrinter{
		ctx:    input.Ctx,
		client: input.Client,
		logger: input.Logger,
	}

	total := 0
	for input.Iter.Next(input.Ctx) {
		key := input.Iter.Val()
		total++

		if err := printer.print(key); err != nil {
			return 0, err
		}
	}

	if err := input.Iter.Err(); err != nil {
		return 0, fmt.Errorf("redis scan: %w", err)
	}

	return total, nil
}

type alertOnceKeyPrinter struct {
	ctx    context.Context
	client redis.UniversalClient
	logger *slog.Logger
}

func (p *alertOnceKeyPrinter) print(key string) error {
	if p == nil {
		return errors.New("alert once printer: nil receiver")
	}

	ttl, ttlErr := p.client.TTL(p.ctx, key).Result()
	if ttlErr != nil {
		if p.logger != nil {
			p.logger.ErrorContext(p.ctx, "failed to fetch TTL", "key", key, "error", ttlErr)
		}
		if ttlPrintErr := writef(os.Stdout, "  %s (TTL: error: %v)\n", key, ttlErr); ttlPrintErr != nil {
			return fmt.Errorf("print alert-once key ttl error: %w", ttlPrintErr)
		}
		return nil
	}

	if ttlPrintErr := writef(os.Stdout, "  %s (TTL: %s)\n", key, renderTTL(ttl)); ttlPrintErr != nil {
		return fmt.Errorf("print alert-once key ttl: %w", ttlPrintErr)
	}
	return nil
}

func runClearAlertOnceKeys(cmdCtx *commandContext, args []string) error {
	opts, err := parseAlertOnceClearFlags(args)
	if err != nil {
		return err
	}
	if confirmErr := confirmClearAlertOnce(opts); confirmErr != nil {
		return confirmErr
	}

	ctx, cancel := context.WithTimeout(cmdCtx.Ctx, 2*time.Minute)
	defer cancel()

	_, redisClient, err := connectInfra(cmdCtx.Logger, &cmdCtx.Config)
	if err != nil {
		return err
	}
	if redisClient == nil {
		if writeErr := writeln(os.Stderr, "Redis client is not available"); writeErr != nil {
			return fmt.Errorf("print redis availability: %w", writeErr)
		}
		return nil
	}
	defer func() {
		if closeErr := redisClient.Close(); closeErr != nil {
			cmdCtx.Logger.Warn("redis close failed", "error", closeErr)
		}
	}()

	req := &alertOnceDeleteRequest{
		Ctx:      ctx,
		Logger:   cmdCtx.Logger,
		Redis:    redisClient,
		Options:  opts,
		BatchCap: 1000,
	}
	stats, err := deleteAlertOnceKeys(req)
	if err != nil {
		return err
	}

	if stats.total == 0 {
		if writeErr := writeln(os.Stdout, "No alert-once keys found in Redis"); writeErr != nil {
			return fmt.Errorf("print alert-once summary: %w", writeErr)
		}
		return nil
	}

	if opts.DryRun {
		return printAlertOnceDryRun(stats)
	}

	return printAlertOnceSummary(stats)
}

func printAlertOnceDryRun(stats alertOnceDeleteStats) error {
	if err := writef(os.Stdout, "Dry-run: would delete %d/%d keys\n", stats.deleted, stats.total); err != nil {
		return fmt.Errorf("print alert-once dry run: %w", err)
	}
	return nil
}

func printAlertOnceSummary(stats alertOnceDeleteStats) error {
	if err := writef(os.Stdout, "Processed %d alert-once keys\n", stats.total); err != nil {
		return fmt.Errorf("print alert-once processed: %w", err)
	}
	if err := writef(os.Stdout, "Deleted %d/%d keys\n", stats.deleted, stats.total); err != nil {
		return fmt.Errorf("print alert-once deleted: %w", err)
	}
	if stats.failures == 0 {
		return nil
	}

	if err := writef(os.Stdout, "Failed batches: %d\n", stats.failures); err != nil {
		return fmt.Errorf("print alert-once failures: %w", err)
	}
	return nil
}

func runRulesJobResults(cmdCtx *commandContext, args []string) error {
	opts, err := parseRulesResultsFlags(args)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(cmdCtx.Ctx, time.Minute)
	defer cancel()

	db, redisClient, err := connectInfraWithOptions(&connectInfraOptions{
		Logger:    cmdCtx.Logger,
		Config:    &cmdCtx.Config,
		WantDB:    true,
		WantRedis: true,
	})
	if err != nil {
		return err
	}
	defer func() {
		if cerr := closeInfra(db, redisClient); cerr != nil {
			cmdCtx.Logger.Warn("close infra failed", "error", cerr)
		}
	}()

	resultPayload, srcInfo, err := fetchJobResults(ctx, jobResultFetchOptions{
		DB:           db,
		Redis:        redisClient,
		Logger:       cmdCtx.Logger,
		JobID:        opts.JobID,
		ShowCacheTTL: true, // Always fetch TTL when Redis is available
	})
	if err != nil {
		return err
	}

	return displayRulesJobResults(opts, resultPayload, srcInfo)
}

func runMigrations(cmdCtx *commandContext, args []string) error {
	opts, err := parseMigrateFlags(args)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(cmdCtx.Ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()

	db, err := bootstrap.ConnectDB(bootstrap.DatabaseConfig{
		DBConfig: cmdCtx.Config.Postgres,
		Logger:   cmdCtx.Logger,
	})
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if closeErr := db.Close(); closeErr != nil {
			cmdCtx.Logger.Warn("db close failed", "error", closeErr)
		}
	}()

	cmdCtx.Logger.Info("running database migrations")

	if migrateErr := bootstrap.RunMigrations(ctx, db, cmdCtx.Logger); migrateErr != nil {
		return fmt.Errorf("run migrations: %w", migrateErr)
	}

	cmdCtx.Logger.Info("migrations completed successfully")
	return nil
}

func runDBReset(cmdCtx *commandContext, args []string) error {
	opts, err := parseDBResetFlags(args)
	if err != nil {
		return err
	}

	target := fmt.Sprintf(
		"database %q on %s:%d",
		cmdCtx.Config.Postgres.Name,
		cmdCtx.Config.Postgres.Host,
		cmdCtx.Config.Postgres.Port,
	)

	remote, err := guardRemoteHost(cmdCtx, opts.AllowRemote, "drop and recreate the public schema")
	if err != nil {
		return err
	}

	confirmOpts := dbResetConfirmOptions{
		yes:    opts.Yes,
		target: target,
	}
	if remote {
		confirmOpts.remoteHost = cmdCtx.Config.Postgres.Host
	}
	if confirmErr := confirmAction(confirmOpts, "reset database schema"); confirmErr != nil {
		return confirmErr
	}

	return withDatabase(cmdCtx, opts.Timeout, func(ctx context.Context, db *sql.DB) error {
		cmdCtx.Logger.Info("dropping public schema", "database", cmdCtx.Config.Postgres.Name)
		if resetErr := cmdCtx.resetDatabase(ctx, db); resetErr != nil {
			return resetErr
		}

		cmdCtx.Logger.Info("re-running database migrations")
		if migrateErr := bootstrap.RunMigrations(ctx, db, cmdCtx.Logger); migrateErr != nil {
			return fmt.Errorf("run migrations: %w", migrateErr)
		}

		if opts.Seed {
			cmdCtx.Logger.Info("seeding development data after reset")
			if seedErr := devseed.Run(ctx, devseed.NewServices(db), cmdCtx.Logger); seedErr != nil {
				return fmt.Errorf("seed data: %w", seedErr)
			}
		}

		cmdCtx.Logger.Info("database reset completed successfully")
		return nil
	})
}

func runDBSeed(cmdCtx *commandContext, args []string) error {
	opts, err := parseDBSeedFlags(args)
	if err != nil {
		return err
	}

	if _, guardErr := guardRemoteHost(cmdCtx, opts.AllowRemote, "seed development data on the configured database"); guardErr != nil {
		return guardErr
	}

	return withDatabase(cmdCtx, opts.Timeout, func(ctx context.Context, db *sql.DB) error {
		cmdCtx.Logger.Info("ensuring database migrations are current")
		if migrateErr := bootstrap.RunMigrations(ctx, db, cmdCtx.Logger); migrateErr != nil {
			return fmt.Errorf("run migrations: %w", migrateErr)
		}

		cmdCtx.Logger.Info("seeding development data")
		if seedErr := devseed.Run(ctx, devseed.NewServices(db), cmdCtx.Logger); seedErr != nil {
			return fmt.Errorf("seed data: %w", seedErr)
		}

		cmdCtx.Logger.Info("database seeding completed successfully")
		return nil
	})
}

func displayRulesJobResults(
	opts rulesResultsOptions,
	payload jobResultPayload,
	src jobResultSource,
) error {
	if payload.raw == nil {
		if err := writef(
			os.Stdout,
			"No cached or persisted results found for job %s (%s)\n",
			opts.JobID,
			src.CacheKey,
		); err != nil {
			return fmt.Errorf("print empty results notice: %w", err)
		}
		return nil
	}

	if opts.RawJSON {
		return printRawRulesJobResults(payload.raw, src)
	}

	return printStructuredRulesJobResults(opts, payload.raw, src)
}

func printRawRulesJobResults(raw json.RawMessage, src jobResultSource) error {
	if err := writef(os.Stdout, "%s\n", raw); err != nil {
		return fmt.Errorf("print raw rules job payload: %w", err)
	}

	if src.TTL != nil {
		if err := writef(os.Stdout, "\nTTL remaining: %s\n", renderTTL(*src.TTL)); err != nil {
			return fmt.Errorf("print raw payload ttl: %w", err)
		}
	}

	if err := writef(os.Stdout, "\nSource: %s\n", src.Source); err != nil {
		return fmt.Errorf("print raw payload source: %w", err)
	}
	return nil
}

func printStructuredRulesJobResults(opts rulesResultsOptions, raw json.RawMessage, src jobResultSource) error {
	results := &service.RulesProcessingResults{}
	if err := json.Unmarshal(raw, results); err != nil {
		return fmt.Errorf("decode cached results: %w", err)
	}

	req := &printRulesJobResultsRequest{
		JobID:   opts.JobID,
		Key:     src.CacheKey,
		Results: results,
		TTL:     src.TTL,
		Source:  src.Source,
	}
	if err := printRulesJobResults(req); err != nil {
		return fmt.Errorf("print rules job summary: %w", err)
	}
	return nil
}

func renderTTL(d time.Duration) string {
	switch {
	case d == -1*time.Second:
		return "no expiry"
	case d == -2*time.Second:
		return "key missing"
	case d < 0:
		return d.String()
	default:
		return d.String()
	}
}

type jobResultFetchOptions struct {
	DB           *sql.DB
	Redis        redis.UniversalClient
	Logger       *slog.Logger
	JobID        string
	ShowCacheTTL bool
}

type jobResultPayload struct {
	raw []byte
}

type jobResultSource struct {
	CacheKey string
	Source   string
	TTL      *time.Duration
}

func fetchJobResults(
	ctx context.Context,
	opts jobResultFetchOptions,
) (jobResultPayload, jobResultSource, error) {
	cacheKey := "rules:results:" + opts.JobID
	src := jobResultSource{CacheKey: cacheKey}

	// Try cache first
	payload, found, err := fetchFromCache(ctx, opts, cacheKey)
	if err != nil {
		return payload, src, err
	}
	if found {
		src.Source = "cache"
		src.setTTL(ctx, opts, cacheKey)
		return payload, src, nil
	}

	// Fall back to database
	if opts.DB == nil {
		return jobResultPayload{}, src, nil
	}
	return fetchFromDB(ctx, opts, src)
}

func (src *jobResultSource) setTTL(ctx context.Context, opts jobResultFetchOptions, cacheKey string) {
	if src == nil {
		return
	}
	if cacheKey == "" {
		cacheKey = src.CacheKey
	} else if src.CacheKey == "" {
		src.CacheKey = cacheKey
	}
	if cacheKey == "" {
		return
	}
	if !opts.ShowCacheTTL || opts.Redis == nil {
		return
	}

	ttlVal, err := opts.Redis.TTL(ctx, cacheKey).Result()
	if err != nil {
		return
	}

	src.TTL = &ttlVal
}

func fetchFromCache(
	ctx context.Context,
	opts jobResultFetchOptions,
	cacheKey string,
) (jobResultPayload, bool, error) {
	if opts.Redis == nil {
		return jobResultPayload{}, false, nil
	}
	bytesVal, err := opts.Redis.Get(ctx, cacheKey).Bytes()
	if errors.Is(err, redis.Nil) {
		return jobResultPayload{}, false, nil
	}
	if err != nil {
		return jobResultPayload{}, false, fmt.Errorf("redis get %s: %w", cacheKey, err)
	}
	return jobResultPayload{raw: bytesVal}, true, nil
}

func fetchFromDB(
	ctx context.Context,
	opts jobResultFetchOptions,
	src jobResultSource,
) (jobResultPayload, jobResultSource, error) {
	repo := data.NewJobResultRepo(opts.DB)
	stored, err := repo.GetByJobID(ctx, opts.JobID)
	if err != nil {
		if errors.Is(err, data.ErrJobResultsNotFound) {
			return jobResultPayload{}, src, nil
		}
		return jobResultPayload{}, src, err
	}
	if stored == nil {
		return jobResultPayload{}, src, nil
	}
	src.Source = "database"
	return jobResultPayload{raw: stored.Result}, src, nil
}

type printRulesJobResultsRequest struct {
	JobID   string
	Key     string
	Results *service.RulesProcessingResults
	TTL     *time.Duration
	Source  string
}

type metricRow struct {
	Label   string
	Count   int
	Samples []string
}

func printRulesJobResults(req *printRulesJobResultsRequest) error {
	if err := printRulesJobHeader(req); err != nil {
		return err
	}
	if err := printRulesJobStatus(req.Results); err != nil {
		return err
	}
	if err := printRulesJobSummary(req.Results); err != nil {
		return err
	}
	if err := printUnknownDomainSections(req.Results); err != nil {
		return err
	}
	if err := printIOCSections(req.Results); err != nil {
		return err
	}
	return nil
}

func printRulesJobHeader(req *printRulesJobResultsRequest) error {
	if err := writef(os.Stdout, "\nRules Job Results\n"); err != nil {
		return fmt.Errorf("write header title: %w", err)
	}
	if err := writef(os.Stdout, "Job ID:    %s\n", req.JobID); err != nil {
		return fmt.Errorf("write header job id: %w", err)
	}
	if err := writef(os.Stdout, "Cache Key: %s\n", req.Key); err != nil {
		return fmt.Errorf("write header cache key: %w", err)
	}
	if req.Source != "" {
		if err := writef(os.Stdout, "Source:    %s\n", req.Source); err != nil {
			return fmt.Errorf("write header source: %w", err)
		}
	}
	if req.TTL != nil {
		if err := writef(os.Stdout, "TTL:       %s\n", renderTTL(*req.TTL)); err != nil {
			return fmt.Errorf("write header ttl: %w", err)
		}
	}
	if err := writeln(os.Stdout); err != nil {
		return fmt.Errorf("write header newline: %w", err)
	}
	return nil
}

func printRulesJobStatus(results *service.RulesProcessingResults) error {
	if results == nil {
		return nil
	}
	if results.ErrorsEncountered == 0 {
		return nil
	}
	if err := writef(
		os.Stdout,
		"Status: failed (rule evaluation errors: %d)\n",
		results.ErrorsEncountered,
	); err != nil {
		return fmt.Errorf("write job status summary: %w", err)
	}
	if err := writeln(
		os.Stdout,
		"The job reported rule evaluator failures; results may be incomplete.",
	); err != nil {
		return fmt.Errorf("write job status warning: %w", err)
	}
	if err := writeln(os.Stdout); err != nil {
		return fmt.Errorf("write job status spacer: %w", err)
	}
	return nil
}

func printRulesJobSummary(results *service.RulesProcessingResults) error {
	if results == nil {
		return nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if err := writeln(w, "Metric\tValue"); err != nil {
		return fmt.Errorf("write summary header: %w", err)
	}
	if err := writef(w, "Alerts Created\t%d\n", results.AlertsCreated); err != nil {
		return fmt.Errorf("write alerts created: %w", err)
	}
	if err := writef(w, "Domains Processed\t%d\n", results.DomainsProcessed); err != nil {
		return fmt.Errorf("write domains processed: %w", err)
	}
	if err := writef(w, "Events Skipped\t%d\n", results.EventsSkipped); err != nil {
		return fmt.Errorf("write events skipped: %w", err)
	}
	if err := writef(w, "Unknown Domains\t%d\n", results.UnknownDomains); err != nil {
		return fmt.Errorf("write unknown domains: %w", err)
	}
	if err := writef(w, "IOC Matches\t%d\n", results.IOCHostMatches); err != nil {
		return fmt.Errorf("write ioc matches: %w", err)
	}
	if err := writef(
		w,
		"Processing Time\t%s\n",
		util.FormatProcessingDuration(results.ProcessingTime),
	); err != nil {
		return fmt.Errorf("write processing time: %w", err)
	}
	if err := writef(w, "Errors Encountered\t%d\n", results.ErrorsEncountered); err != nil {
		return fmt.Errorf("write errors encountered: %w", err)
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush summary: %w", err)
	}
	return nil
}

func printUnknownDomainSections(results *service.RulesProcessingResults) error {
	if results == nil {
		return nil
	}
	if err := printMetricSection("Unknown Domain Breakdown", []metricRow{
		{
			"Alerts Created",
			results.UnknownDomain.Alerted.Count,
			results.UnknownDomain.Alerted.Samples,
		},
		{
			"Would Alert (Dry-Run)",
			results.UnknownDomain.AlertedDryRun.Count,
			results.UnknownDomain.AlertedDryRun.Samples,
		},
		{
			"Allowlisted",
			results.UnknownDomain.SuppressedAllowlist.Count,
			results.UnknownDomain.SuppressedAllowlist.Samples,
		},
		{
			"Already Seen",
			results.UnknownDomain.SuppressedSeen.Count,
			results.UnknownDomain.SuppressedSeen.Samples,
		},
		{
			"Alert Once (TTL)",
			results.UnknownDomain.SuppressedDedupe.Count,
			results.UnknownDomain.SuppressedDedupe.Samples,
		},
		{
			"Normalization Issues",
			results.UnknownDomain.NormalizationFailed.Count,
			results.UnknownDomain.NormalizationFailed.Samples,
		},
		{
			"Evaluation Errors",
			results.UnknownDomain.Errors.Count,
			results.UnknownDomain.Errors.Samples,
		},
	}); err != nil {
		return fmt.Errorf("print unknown domain metrics: %w", err)
	}

	if len(results.WouldAlertUnknown) > 0 {
		if err := writef(
			os.Stdout,
			"\nUnknown Domains (Dry-Run Samples):\n  %s\n",
			strings.Join(results.WouldAlertUnknown, ", "),
		); err != nil {
			return fmt.Errorf("print unknown domain samples: %w", err)
		}
	}
	return nil
}

func printIOCSections(results *service.RulesProcessingResults) error {
	if results == nil {
		return nil
	}
	if err := printMetricSection("IOC Domain Breakdown", []metricRow{
		{"Alerts Created", results.IOC.Alerts.Count, results.IOC.Alerts.Samples},
		{
			"Would Alert (Dry-Run)",
			results.IOC.MatchesDryRun.Count,
			results.IOC.MatchesDryRun.Samples,
		},
		{"Matches Observed", results.IOC.Matches.Count, results.IOC.Matches.Samples},
	}); err != nil {
		return fmt.Errorf("print ioc metrics: %w", err)
	}

	if len(results.WouldAlertIOC) > 0 {
		if err := writef(
			os.Stdout,
			"\nIOC Domains (Dry-Run Samples):\n  %s\n",
			strings.Join(results.WouldAlertIOC, ", "),
		); err != nil {
			return fmt.Errorf("print ioc samples: %w", err)
		}
	}
	return nil
}

func printMetricSection(title string, rows []metricRow) error {
	if err := writef(os.Stdout, "\n%s\n", title); err != nil {
		return fmt.Errorf("write metric section title: %w", err)
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if err := writeln(w, "Detail\tCount"); err != nil {
		return fmt.Errorf("write metric section header: %w", err)
	}
	for _, row := range rows {
		if err := writef(w, "%s\t%d\n", row.Label, row.Count); err != nil {
			return fmt.Errorf("write metric row %q: %w", row.Label, err)
		}
		if len(row.Samples) > 0 {
			if err := writef(w, "  Samples\t%s\n", strings.Join(row.Samples, ", ")); err != nil {
				return fmt.Errorf("write metric samples for %q: %w", row.Label, err)
			}
		}
	}
	if err := w.Flush(); err != nil {
		return fmt.Errorf("flush metric section: %w", err)
	}
	return nil
}

type alertOnceDeleteRequest struct {
	Ctx      context.Context
	Logger   *slog.Logger
	Redis    redis.UniversalClient
	Options  alertOnceClearOptions
	BatchCap int
}

type alertOnceDeleteStats struct {
	total    int
	deleted  int64
	failures int
}

func flushAlertOnceBatch(req *alertOnceDeleteRequest, batch []string, stats *alertOnceDeleteStats) {
	if len(batch) == 0 {
		return
	}
	if req.Options.DryRun {
		stats.deleted += int64(len(batch))
		if req.Logger != nil {
			req.Logger.Info("dry-run skipping alert-once delete", "count", len(batch))
		}
		return
	}
	n, delErr := req.Redis.Del(req.Ctx, batch...).Result()
	if delErr != nil {
		stats.failures++
		if req.Logger != nil {
			req.Logger.Error(
				"failed to delete alert-once keys",
				"count",
				len(batch),
				"error",
				delErr,
			)
		}
		if err := writef(os.Stdout, "Failed to delete %d keys: %v\n", len(batch), delErr); err != nil &&
			req.Logger != nil {
			req.Logger.Error("stdout write for alert-once delete failure failed", "error", err)
		}
		return
	}
	stats.deleted += n
}

func shouldIncludeAlertOnceKey(req *alertOnceDeleteRequest, key string) bool {
	if req.Options.Domain == "" {
		return true
	}
	_, dedupe, err := parseAlertOnceRedisKey(key)
	if err != nil {
		if req.Logger != nil {
			req.Logger.Warn("skipping alert-once key", "key", key, "error", err)
		}
		return false
	}
	_, subject := splitAlertOnceDedupeKey(dedupe)
	return alertOnceSubjectMatchesFilter(subject, req.Options.Domain)
}

func deleteAlertOnceKeys(req *alertOnceDeleteRequest) (alertOnceDeleteStats, error) {
	patternOpts := &listOptions{
		SiteID: req.Options.SiteID,
		Scope:  req.Options.Scope,
		Domain: req.Options.Domain,
		All:    req.Options.All,
	}
	patterns := buildAlertOncePatterns(patternOpts)
	if len(patterns) == 0 {
		return alertOnceDeleteStats{}, nil
	}

	batchCap := req.BatchCap
	if batchCap <= 0 {
		batchCap = 1000
	}

	stats := alertOnceDeleteStats{}
	for _, pattern := range patterns {
		if err := req.deleteAlertOnceKeysForPattern(pattern, &stats, batchCap); err != nil {
			return stats, err
		}
	}
	return stats, nil
}

func (req *alertOnceDeleteRequest) deleteAlertOnceKeysForPattern(
	pattern string,
	stats *alertOnceDeleteStats,
	batchCap int,
) error {
	if req.Logger != nil {
		req.Logger.Info("scanning redis", "pattern", pattern, "dry_run", req.Options.DryRun)
	}

	iter := req.Redis.Scan(req.Ctx, 0, pattern, 100).Iterator()
	batch := make([]string, 0, batchCap)

	for iter.Next(req.Ctx) {
		key := iter.Val()
		if !shouldIncludeAlertOnceKey(req, key) {
			continue
		}

		stats.total++
		batch = append(batch, key)

		if len(batch) == batchCap {
			flushAlertOnceBatch(req, batch, stats)
			batch = batch[:0]
		}
	}
	if err := iter.Err(); err != nil {
		return fmt.Errorf("redis scan: %w", err)
	}

	flushAlertOnceBatch(req, batch, stats)
	return nil
}

func parseClearFlags(args []string) (clearOptions, error) {
	fs := flag.NewFlagSet("clear-seen-domains", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts clearOptions
	fs.StringVar(&opts.SiteID, "site-id", "", "Site ID to clear (required unless --all)")
	fs.StringVar(&opts.Scope, "scope", "", "Optional scope filter (requires --site-id)")
	fs.StringVar(&opts.Domain, "domain", "", "Optional domain filter (requires --site-id)")
	fs.BoolVar(&opts.All, "all", false, "Clear seen domains for all sites")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "Print actions without executing")
	fs.BoolVar(&opts.Yes, "yes", false, "Skip confirmation prompt")

	if err := fs.Parse(args); err != nil {
		return clearOptions{}, err
	}

	normalizeClearOptions(&opts)
	if err := validateClearOptions(opts); err != nil {
		return clearOptions{}, err
	}

	return opts, nil
}

func parseAlertOnceClearFlags(args []string) (alertOnceClearOptions, error) {
	fs := flag.NewFlagSet("clear-alertonce-keys", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts alertOnceClearOptions
	fs.StringVar(&opts.SiteID, "site-id", "", "[Deprecated: Redis keys no longer include site_id]")
	fs.StringVar(
		&opts.Scope,
		"scope",
		"",
		"Scope to clear (required unless --all)",
	)
	fs.StringVar(
		&opts.Domain,
		"domain",
		"",
		"Optional domain/key filter",
	)
	fs.BoolVar(&opts.All, "all", false, "Clear alert-once keys for all scopes")
	fs.BoolVar(&opts.DryRun, "dry-run", false, "Print actions without executing")
	fs.BoolVar(&opts.Yes, "yes", false, "Skip confirmation prompt")

	if err := fs.Parse(args); err != nil {
		return alertOnceClearOptions{}, err
	}

	normalizeAlertOnceClearOptions(&opts)
	if err := validateAlertOnceClearOptions(opts); err != nil {
		return alertOnceClearOptions{}, err
	}

	return opts, nil
}

func parseListFlags(args []string) (listOptions, error) {
	fs := flag.NewFlagSet("list-seen-domains", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts listOptions
	fs.StringVar(
		&opts.SiteID,
		"site-id",
		"",
		"Filter by site ID (for DB queries; required unless --all or --redis-only)",
	)
	fs.StringVar(&opts.Scope, "scope", "", "Filter by scope (required for Redis queries unless --all)")
	fs.StringVar(&opts.Domain, "domain", "", "Filter by domain substring (case-insensitive)")
	fs.BoolVar(
		&opts.All,
		"all",
		false,
		"Include all entries (can be combined with --scope or --domain)",
	)
	fs.IntVar(&opts.Limit, "limit", 20, "Maximum rows/keys to display (0 for unlimited)")
	fs.IntVar(&opts.Offset, "offset", 0, "Offset for database query results")
	fs.BoolVar(&opts.DBOnly, "db-only", false, "Only query Postgres (skip Redis)")
	fs.BoolVar(&opts.RedisOnly, "redis-only", false, "Only query Redis (skip Postgres)")
	fs.BoolVar(
		&opts.IncludeAlertOnce,
		"include-alertonce",
		false,
		"Include alert-once dedupe keys for matching filters",
	)

	if err := fs.Parse(args); err != nil {
		return listOptions{}, err
	}

	normalizeListOptions(&opts)
	if err := validateListOptions(&opts); err != nil {
		return listOptions{}, err
	}

	return opts, nil
}

func parseRulesResultsFlags(args []string) (rulesResultsOptions, error) {
	fs := flag.NewFlagSet("rules-job-results", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	var opts rulesResultsOptions
	fs.StringVar(&opts.JobID, "job-id", "", "Job ID to inspect (required)")
	fs.BoolVar(&opts.RawJSON, "json", false, "Print raw cached JSON payload")

	if err := fs.Parse(args); err != nil {
		return rulesResultsOptions{}, err
	}

	opts.JobID = strings.TrimSpace(opts.JobID)
	if opts.JobID == "" {
		return rulesResultsOptions{}, errors.New("--job-id is required")
	}

	return opts, nil
}

func parseMigrateFlags(args []string) (migrateOptions, error) {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	opts := migrateOptions{
		Timeout: defaultMigrationTimeout,
	}

	fs.DurationVar(
		&opts.Timeout,
		"timeout",
		defaultMigrationTimeout,
		"Maximum duration to wait for migrations to complete",
	)

	if err := fs.Parse(args); err != nil {
		return migrateOptions{}, err
	}

	if opts.Timeout <= 0 {
		return migrateOptions{}, errors.New("--timeout must be greater than zero")
	}

	return opts, nil
}

func parseDBResetFlags(args []string) (dbResetOptions, error) {
	fs := flag.NewFlagSet("db-reset", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	opts := dbResetOptions{
		Timeout: defaultMigrationTimeout,
	}

	fs.DurationVar(
		&opts.Timeout,
		"timeout",
		defaultMigrationTimeout,
		"Maximum duration to wait for reset operations to complete",
	)
	fs.BoolVar(
		&opts.Yes,
		"yes",
		false,
		"Skip confirmation prompt",
	)
	fs.BoolVar(
		&opts.Seed,
		"seed",
		false,
		"Run database seeding after reset completes",
	)
	fs.BoolVar(
		&opts.AllowRemote,
		"allow-remote",
		false,
		"Permit running against database hosts that do not look local",
	)

	if err := fs.Parse(args); err != nil {
		return dbResetOptions{}, err
	}

	if opts.Timeout <= 0 {
		return dbResetOptions{}, errors.New("--timeout must be greater than zero")
	}

	return opts, nil
}

func parseDBSeedFlags(args []string) (dbSeedOptions, error) {
	fs := flag.NewFlagSet("db-seed", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	opts := dbSeedOptions{
		Timeout: defaultMigrationTimeout,
	}

	fs.DurationVar(
		&opts.Timeout,
		"timeout",
		defaultMigrationTimeout,
		"Maximum duration to wait for seeding to complete",
	)
	fs.BoolVar(
		&opts.AllowRemote,
		"allow-remote",
		false,
		"Permit running against database hosts that do not look local",
	)

	if err := fs.Parse(args); err != nil {
		return dbSeedOptions{}, err
	}

	if opts.Timeout <= 0 {
		return dbSeedOptions{}, errors.New("--timeout must be greater than zero")
	}

	return opts, nil
}

func withDatabase(
	cmdCtx *commandContext,
	timeout time.Duration,
	f func(context.Context, *sql.DB) error,
) error {
	ctx, stop := signal.NotifyContext(cmdCtx.Ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	db, err := bootstrap.ConnectDB(bootstrap.DatabaseConfig{
		DBConfig: cmdCtx.Config.Postgres,
		Logger:   cmdCtx.Logger,
	})
	if err != nil {
		return fmt.Errorf("connect db: %w", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			cmdCtx.Logger.Warn("db close failed", "error", cerr)
		}
	}()

	return f(ctx, db)
}

func guardRemoteHost(cmdCtx *commandContext, allow bool, action string) (bool, error) {
	remote := isLikelyRemoteHost(cmdCtx.Config.Postgres.Host)
	if !remote {
		return false, nil
	}
	if !allow {
		return true, fmt.Errorf(
			"refusing to run against potentially remote database host %q; re-run with --allow-remote if this is intentional",
			cmdCtx.Config.Postgres.Host,
		)
	}
	if err := requireRemoteHostConfirmation(action, cmdCtx.Config.Postgres.Host); err != nil {
		return true, err
	}
	return true, nil
}

func (cmdCtx *commandContext) resetDatabase(ctx context.Context, db *sql.DB) error {
	if cmdCtx == nil {
		return errors.New("command context is required")
	}

	cfg := &cmdCtx.Config.Postgres
	statements := []string{
		"DROP SCHEMA public CASCADE",
		"CREATE SCHEMA public",
		"GRANT ALL ON SCHEMA public TO public",
	}
	if user := strings.TrimSpace(cfg.User); user != "" && !strings.EqualFold(user, "public") {
		statements = append(statements, "GRANT ALL ON SCHEMA public TO "+quoteIdentifier(user))
	}

	for _, stmt := range statements {
		if cmdCtx.Logger != nil {
			cmdCtx.Logger.DebugContext(ctx, "executing reset statement", "sql", stmt)
		}
		if _, err := db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("exec %q: %w", stmt, err)
		}
	}
	return nil
}

func quoteIdentifier(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

func isLikelyRemoteHost(host string) bool {
	h := strings.ToLower(strings.TrimSpace(host))
	if h == "" {
		return false
	}
	if h == "localhost" || h == "127.0.0.1" || h == "::1" {
		return false
	}
	if strings.HasSuffix(h, ".local") {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return !ip.IsLoopback()
	}
	return true
}

func requireRemoteHostConfirmation(action, host string) error {
	if err := writef(
		os.Stderr,
		"\nWARNING: database host %q does not look like a local address.\n"+
			"This operation will %s.\n",
		host,
		action,
	); err != nil {
		return fmt.Errorf("print remote host warning: %w", err)
	}
	if err := writef(os.Stderr, "Type %q to continue or press enter to abort: ", host); err != nil {
		return fmt.Errorf("print remote host prompt: %w", err)
	}
	reader := bufio.NewReader(os.Stdin)
	resp, err := reader.ReadString('\n')
	if err != nil {
		if writeErr := writef(os.Stderr, "\nFailed to read confirmation input: %v\n", err); writeErr != nil {
			return fmt.Errorf("aborted by user: report write failed: %w", writeErr)
		}
		return errors.New("aborted by user")
	}
	if strings.TrimSpace(resp) != host {
		if writeErr := writeln(os.Stderr, "\nRemote safeguard check failed; aborting."); writeErr != nil {
			return fmt.Errorf("print remote safeguard failure: %w", writeErr)
		}
		return errors.New("aborted by user")
	}
	return nil
}

type listConnections struct {
	DB        *sql.DB
	Redis     redis.UniversalClient
	wantDB    bool
	wantRedis bool
}

type listWarnings struct {
	missingRedis bool
}

type listResults struct {
	db        querySeenDomainResponse
	redis     inspectSeenRedisResponse
	alertOnce inspectAlertOnceResponse
}

func openListConnections(
	cmdCtx *commandContext,
	opts *listOptions,
) (*listConnections, listWarnings, error) {
	if opts == nil {
		return nil, listWarnings{}, errors.New("list options are required")
	}
	wantDB := !opts.RedisOnly
	wantRedis := !opts.DBOnly || opts.IncludeAlertOnce

	db, redisClient, err := connectInfraWithOptions(&connectInfraOptions{
		Logger:    cmdCtx.Logger,
		Config:    &cmdCtx.Config,
		WantDB:    wantDB,
		WantRedis: wantRedis,
	})
	if err != nil {
		return nil, listWarnings{}, err
	}

	warnings := listWarnings{}
	if wantRedis && redisClient == nil && !hasRedisConfig(&cmdCtx.Config.Redis) {
		warnings.missingRedis = true
	}

	return &listConnections{
		DB:        db,
		Redis:     redisClient,
		wantDB:    wantDB,
		wantRedis: wantRedis,
	}, warnings, nil
}

func (c *listConnections) Close() error {
	var closeErr error
	if c.DB != nil {
		if err := c.DB.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close db: %w", err))
		}
	}
	if c.Redis != nil {
		if err := c.Redis.Close(); err != nil {
			closeErr = errors.Join(closeErr, fmt.Errorf("close redis: %w", err))
		}
	}
	return closeErr
}

type fetchSeenDomainsRequest struct {
	Ctx     context.Context
	Logger  *slog.Logger
	DB      *sql.DB
	Client  redis.UniversalClient
	Options *listOptions
}

func fetchSeenDomainsFromDB(req *fetchSeenDomainsRequest) (querySeenDomainResponse, error) {
	if req == nil || req.DB == nil || req.Options == nil || req.Options.RedisOnly {
		return querySeenDomainResponse{}, nil
	}
	return querySeenDomainRows(&querySeenDomainRequest{
		Ctx:     req.Ctx,
		DB:      req.DB,
		Logger:  req.Logger,
		Options: req.Options,
	})
}

func fetchSeenDomainsFromRedis(req *fetchSeenDomainsRequest) (inspectSeenRedisResponse, error) {
	if req == nil || req.Client == nil || req.Options == nil || req.Options.DBOnly {
		return inspectSeenRedisResponse{}, nil
	}
	return inspectSeenRedis(&inspectSeenRedisRequest{
		Ctx:     req.Ctx,
		Client:  req.Client,
		Logger:  req.Logger,
		Options: req.Options,
	})
}

func fetchAlertOnceKeys(req *fetchSeenDomainsRequest) (inspectAlertOnceResponse, error) {
	if req == nil || req.Client == nil || req.Options == nil {
		return inspectAlertOnceResponse{}, nil
	}
	return inspectAlertOnce(&inspectAlertOnceRequest{
		Ctx:     req.Ctx,
		Client:  req.Client,
		Logger:  req.Logger,
		Options: req.Options,
	})
}

func gatherListResults(req *fetchSeenDomainsRequest, conns *listConnections) (listResults, error) {
	if req == nil {
		return listResults{}, errors.New("fetch request is required")
	}
	req.DB = conns.DB
	req.Client = conns.Redis
	dbResp, err := fetchSeenDomainsFromDB(req)
	if err != nil {
		return listResults{}, err
	}

	redisResp, err := fetchSeenDomainsFromRedis(req)
	if err != nil {
		return listResults{}, err
	}

	var alertOnceResp inspectAlertOnceResponse
	if req.Options != nil && req.Options.IncludeAlertOnce {
		alertOnceResp, err = fetchAlertOnceKeys(req)
		if err != nil {
			return listResults{}, err
		}
	}

	return listResults{db: dbResp, redis: redisResp, alertOnce: alertOnceResp}, nil
}

type renderListResultsRequest struct {
	Conns    *listConnections
	Warnings listWarnings
	Results  listResults
	Options  *listOptions
}

func renderListResults(req *renderListResultsRequest) error {
	if req == nil {
		return errors.New("render request is required")
	}
	if err := printRedisWarningIfNeeded(req.Warnings); err != nil {
		return err
	}

	if err := renderDBResults(req); err != nil {
		return err
	}

	if err := renderRedisResults(req); err != nil {
		return err
	}

	return renderAlertOnceResults(req)
}

func printRedisWarningIfNeeded(warnings listWarnings) error {
	if !warnings.missingRedis {
		return nil
	}

	if err := writeln(
		os.Stdout,
		"Redis not configured or connection disabled; skipping cache inspection.",
	); err != nil {
		return fmt.Errorf("print redis warning: %w", err)
	}
	return nil
}

func renderDBResults(req *renderListResultsRequest) error {
	if req == nil || !req.Conns.wantDB {
		return nil
	}

	if err := printSeenDomainRows(req.Results.db, req.Options); err != nil {
		return fmt.Errorf("render seen domain rows: %w", err)
	}
	return nil
}

func renderRedisResults(req *renderListResultsRequest) error {
	if req == nil || !req.Conns.wantRedis || req.Conns.Redis == nil {
		return nil
	}

	if err := printRedisEntries(req.Results.redis, req.Options); err != nil {
		return fmt.Errorf("render redis entries: %w", err)
	}
	return nil
}

func renderAlertOnceResults(req *renderListResultsRequest) error {
	if req == nil || req.Options == nil || !req.Options.IncludeAlertOnce {
		return nil
	}

	if req.Conns.Redis == nil {
		if err := writeln(
			os.Stdout,
			"\nAlert-once inspection skipped: Redis connection unavailable.",
		); err != nil {
			return fmt.Errorf("print alert-once skip message: %w", err)
		}
		return nil
	}

	if err := printAlertOnceEntries(req.Results.alertOnce, req.Options); err != nil {
		return fmt.Errorf("print alert-once entries: %w", err)
	}
	return nil
}

func normalizeClearOptions(opts *clearOptions) {
	opts.SiteID = strings.TrimSpace(opts.SiteID)
	opts.Scope = strings.TrimSpace(opts.Scope)
	opts.Domain = strings.ToLower(strings.TrimSpace(opts.Domain))
}

func normalizeAlertOnceClearOptions(opts *alertOnceClearOptions) {
	opts.SiteID = strings.TrimSpace(opts.SiteID)
	opts.Scope = strings.TrimSpace(opts.Scope)
	opts.Domain = strings.ToLower(strings.TrimSpace(opts.Domain))
}

func normalizeListOptions(opts *listOptions) {
	opts.SiteID = strings.TrimSpace(opts.SiteID)
	opts.Scope = strings.TrimSpace(opts.Scope)
	opts.Domain = strings.ToLower(strings.TrimSpace(opts.Domain))
}

func validateClearOptions(opts clearOptions) error {
	if opts.All {
		if opts.SiteID != "" || opts.Scope != "" || opts.Domain != "" {
			return errors.New("--all cannot be combined with site, scope, or domain filters")
		}
		return nil
	}
	if opts.SiteID == "" {
		return errors.New("--site-id is required unless --all is provided")
	}
	if opts.Scope == "" && opts.Domain != "" {
		return errors.New("--domain requires --scope to avoid clearing other scopes accidentally")
	}
	return nil
}

func validateAlertOnceClearOptions(opts alertOnceClearOptions) error {
	if opts.All {
		if opts.Scope != "" {
			return errors.New("--all cannot be combined with --scope")
		}
		return nil
	}
	if opts.Scope == "" {
		return errors.New("--scope is required unless --all is provided")
	}
	return nil
}

func validateListOptions(opts *listOptions) error {
	if opts == nil {
		return errors.New("list options are required")
	}
	if opts.Limit < 0 {
		return errors.New("--limit must be >= 0")
	}
	if opts.Offset < 0 {
		return errors.New("--offset must be >= 0")
	}
	if opts.DBOnly && opts.RedisOnly {
		return errors.New("--db-only and --redis-only cannot both be set")
	}

	// For --all queries, no site/scope filter required
	if opts.All {
		if opts.SiteID != "" {
			return errors.New("--all cannot be combined with --site-id")
		}
		return nil
	}

	// For Redis-only queries, require --scope (Redis keys don't include site_id)
	if opts.RedisOnly {
		if opts.Scope == "" {
			return errors.New("--scope is required for Redis queries (or use --all)")
		}
		return nil
	}

	// For DB queries (including combined DB+Redis), require --site-id
	if opts.SiteID == "" {
		return errors.New("--site-id is required for DB queries (or use --all or --redis-only with --scope)")
	}
	return nil
}

type confirmOptions interface {
	IsDryRun() bool
	IsYes() bool
	GetTarget() string
	GetWarning() string
}

type dbResetConfirmOptions struct {
	yes        bool
	target     string
	remoteHost string
}

func (d dbResetConfirmOptions) IsDryRun() bool { return false }
func (d dbResetConfirmOptions) IsYes() bool {
	if d.remoteHost != "" {
		return false
	}
	return d.yes
}

func (d dbResetConfirmOptions) GetWarning() string {
	warning := "WARNING: this will drop and recreate the public schema for the configured database."
	if d.remoteHost != "" {
		warning += fmt.Sprintf(" Host %q appears to be remote; double-check before proceeding.", d.remoteHost)
	}
	return warning
}
func (d dbResetConfirmOptions) GetTarget() string { return d.target }

type clearConfirmOptions struct {
	opts clearOptions
}

func (c clearConfirmOptions) IsDryRun() bool { return c.opts.DryRun }
func (c clearConfirmOptions) IsYes() bool    { return c.opts.Yes }
func (c clearConfirmOptions) GetWarning() string {
	return "WARNING: this will remove all seen domain records and cache entries for every site."
}

func (c clearConfirmOptions) GetTarget() string {
	target := fmt.Sprintf("site %q", c.opts.SiteID)
	if c.opts.Scope != "" {
		target += fmt.Sprintf(", scope %q", c.opts.Scope)
	}
	if c.opts.Domain != "" {
		target += fmt.Sprintf(", domain %q", c.opts.Domain)
	}
	return target
}

type alertOnceConfirmOptions struct {
	opts alertOnceClearOptions
}

func (a alertOnceConfirmOptions) IsDryRun() bool { return a.opts.DryRun }
func (a alertOnceConfirmOptions) IsYes() bool    { return a.opts.Yes }
func (a alertOnceConfirmOptions) GetWarning() string {
	return "WARNING: this will remove all alert-once dedupe keys for every site."
}

func (a alertOnceConfirmOptions) GetTarget() string {
	target := fmt.Sprintf("site %q", a.opts.SiteID)
	if a.opts.Scope != "" {
		target += fmt.Sprintf(", scope %q", a.opts.Scope)
	}
	if a.opts.Domain != "" {
		target += fmt.Sprintf(", domain %q", a.opts.Domain)
	}
	return target
}

func confirmAction(opts confirmOptions, actionType string) error {
	if opts.IsDryRun() || opts.IsYes() {
		return nil
	}

	if err := printConfirmationIntro(opts, actionType); err != nil {
		return err
	}

	if err := write(os.Stdout, "Continue? [y/N]: "); err != nil {
		return fmt.Errorf("print confirmation prompt: %w", err)
	}
	reader := bufio.NewReader(os.Stdin)
	resp, err := reader.ReadString('\n')
	if err != nil {
		if writeErr := writef(os.Stdout, "\nFailed to read confirmation input: %v\n", err); writeErr != nil {
			return fmt.Errorf("aborted by user: report write failed: %w", writeErr)
		}
		return errors.New("aborted by user")
	}
	resp = strings.ToLower(strings.TrimSpace(resp))
	if resp == "y" || resp == "yes" {
		return nil
	}
	return errors.New("aborted by user")
}

func confirmClear(opts clearOptions) error {
	return confirmAction(clearConfirmOptions{opts}, "clear seen domain data")
}

func confirmClearAlertOnce(opts alertOnceClearOptions) error {
	return confirmAction(alertOnceConfirmOptions{opts}, "clear alert-once keys")
}

func printConfirmationIntro(opts confirmOptions, actionType string) error {
	target := opts.GetTarget()
	if target == "" {
		if err := writeln(os.Stdout, opts.GetWarning()); err != nil {
			return fmt.Errorf("print confirmation warning: %w", err)
		}
		return nil
	}

	if err := writef(os.Stdout, "About to %s for %s.\n", actionType, target); err != nil {
		return fmt.Errorf("print confirmation message: %w", err)
	}
	return nil
}

type inspectAlertOnceRequest struct {
	Ctx     context.Context
	Client  redis.UniversalClient
	Logger  *slog.Logger
	Options *listOptions
}

type alertOnceEntry struct {
	Key       string
	Scope     string
	DedupeKey string
	TTL       time.Duration
}

type inspectAlertOnceResponse struct {
	Entries []alertOnceEntry
	Total   int
}

func inspectAlertOnce(req *inspectAlertOnceRequest) (inspectAlertOnceResponse, error) {
	if req == nil || req.Options == nil {
		return inspectAlertOnceResponse{}, nil
	}
	patterns := buildAlertOncePatterns(req.Options)
	if len(patterns) == 0 {
		return inspectAlertOnceResponse{}, nil
	}

	collector := alertOnceCollector{limit: req.Options.Limit}
	for _, pattern := range patterns {
		if req.Logger != nil {
			req.Logger.Info("scanning redis", "pattern", pattern)
		}
		if err := collector.scanPattern(req, pattern); err != nil {
			return inspectAlertOnceResponse{}, err
		}
		if collector.truncated {
			break
		}
	}
	return collector.result(), nil
}

type alertOnceCollector struct {
	entries   []alertOnceEntry
	total     int
	limit     int
	truncated bool
}

func (c *alertOnceCollector) scanPattern(req *inspectAlertOnceRequest, pattern string) error {
	if req == nil {
		return errors.New("alert-once request is required")
	}
	iter := req.Client.Scan(req.Ctx, 0, pattern, 1000).Iterator()
	for iter.Next(req.Ctx) {
		if err := c.addKey(req, iter.Val()); err != nil {
			return err
		}
		if c.truncated {
			break
		}
	}
	return iter.Err()
}

func (c *alertOnceCollector) addKey(req *inspectAlertOnceRequest, key string) error {
	if req == nil || req.Options == nil {
		return nil
	}
	scope, dedupe, err := parseAlertOnceRedisKey(key)
	if err != nil {
		if req.Logger != nil {
			req.Logger.Warn("skipping alert-once key", "key", key, "error", err)
		}
		return nil
	}

	_, subject := splitAlertOnceDedupeKey(dedupe)
	if !alertOnceSubjectMatchesFilter(subject, req.Options.Domain) {
		return nil
	}

	c.total++
	if c.limit > 0 && len(c.entries) >= c.limit {
		c.truncated = true
		return nil
	}

	ttl, err := req.Client.TTL(req.Ctx, key).Result()
	if err != nil {
		return fmt.Errorf("query redis ttl for key %q: %w", key, err)
	}

	c.entries = append(c.entries, alertOnceEntry{
		Key:       key,
		Scope:     scope,
		DedupeKey: dedupe,
		TTL:       ttl,
	})
	return nil
}

func (c *alertOnceCollector) result() inspectAlertOnceResponse {
	sort.Slice(c.entries, func(i, j int) bool {
		if c.entries[i].Scope == c.entries[j].Scope {
			return c.entries[i].DedupeKey < c.entries[j].DedupeKey
		}
		return c.entries[i].Scope < c.entries[j].Scope
	})
	return inspectAlertOnceResponse{
		Entries: c.entries,
		Total:   c.total,
	}
}

const alertOnceScopeKeyPrefix = "rules:alertonce:scope:"

func buildAlertOncePatterns(opts *listOptions) []string {
	if opts == nil {
		return nil
	}
	if !opts.All && opts.Scope == "" {
		return nil
	}

	scopePart := "*"
	if opts.Scope != "" {
		scopePart = opts.Scope
	}
	keyPart := "*"
	if opts.Domain != "" {
		keyPart = "*" + opts.Domain + "*"
	}

	return []string{alertOnceScopeKeyPrefix + scopePart + ":key:" + keyPart}
}

var errUnexpectedAlertOnceRedisKeyFormat = errors.New("unexpected alert-once redis key format")

// parseAlertOnceRedisKey parses "rules:alertonce:scope:<scope>:key:<dedupe>" into (scope, dedupe).
// Uses SplitN to minimize allocations when the dedupe key contains colons.
func parseAlertOnceRedisKey(key string) (string, string, error) {
	// Expected format: rules:alertonce:scope:<scope>:key:<dedupe>
	// Split into at most 6 parts to preserve colons in dedupe key
	parts := strings.SplitN(key, ":", 6)
	if len(parts) < 6 {
		return "", "", errUnexpectedAlertOnceRedisKeyFormat
	}
	if parts[0] != "rules" || parts[1] != "alertonce" || parts[2] != "scope" ||
		parts[4] != "key" {
		return "", "", errUnexpectedAlertOnceRedisKeyFormat
	}
	return parts[3], parts[5], nil
}

func splitAlertOnceDedupeKey(dedupe string) (string, string) {
	dedupe = strings.TrimSpace(dedupe)
	if dedupe == "" {
		return "", ""
	}
	parts := strings.SplitN(dedupe, ":", 2)
	rule := strings.TrimSpace(parts[0])
	value := ""
	if len(parts) > 1 {
		value = strings.TrimSpace(parts[1])
	}
	return rule, value
}

func alertOnceSubjectMatchesFilter(subject, filter string) bool {
	if filter == "" {
		return true
	}
	return strings.Contains(subject, filter)
}

func prettyAlertOnceRule(rule string) string {
	switch strings.ToLower(rule) {
	case "unknown":
		return "unknown_domain"
	case "ioc":
		return "ioc_domain"
	default:
		return rule
	}
}

func printAlertOnceEntries(resp inspectAlertOnceResponse, opts *listOptions) error {
	if opts == nil {
		return errors.New("list options are required")
	}
	displayLimit := max(opts.Limit, 0)
	if err := writef(os.Stdout, "\nAlert-once dedupe entries"); err != nil {
		return fmt.Errorf("write alert-once header: %w", err)
	}
	if displayLimit > 0 {
		if err := writef(os.Stdout, " (showing up to %d)", displayLimit); err != nil {
			return fmt.Errorf("write alert-once limit: %w", err)
		}
	}
	if err := writeln(os.Stdout); err != nil {
		return fmt.Errorf("write alert-once header newline: %w", err)
	}

	if len(resp.Entries) == 0 {
		return printNoAlertOnceEntries()
	}

	if err := renderAlertOnceTable(resp.Entries); err != nil {
		return err
	}

	if err := writef(os.Stdout, "Total keys matched: %d\n", resp.Total); err != nil {
		return fmt.Errorf("write alert-once total: %w", err)
	}
	if opts.Limit > 0 && resp.Total > len(resp.Entries) {
		if err := writeln(os.Stdout, "More keys available; increase --limit to view additional entries."); err != nil {
			return fmt.Errorf("write alert-once more-keys message: %w", err)
		}
	}
	return nil
}

func printNoAlertOnceEntries() error {
	if err := writeln(os.Stdout, "  (no keys matched)"); err != nil {
		return fmt.Errorf("write alert-once empty message: %w", err)
	}
	return nil
}

func renderAlertOnceTable(entries []alertOnceEntry) error {
	tw := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	if err := writeln(tw, "SCOPE\tRULE\tSUBJECT\tTTL\tKEY"); err != nil {
		return fmt.Errorf("write alert-once header row: %w", err)
	}

	for _, entry := range entries {
		rule, subject := splitAlertOnceDedupeKey(entry.DedupeKey)
		if subject == "" {
			subject = "-"
		}
		if err := writef(
			tw,
			"%s\t%s\t%s\t%s\t%s\n",
			entry.Scope,
			prettyAlertOnceRule(rule),
			subject,
			formatRedisTTL(entry.TTL),
			entry.Key,
		); err != nil {
			return fmt.Errorf("write alert-once entry: %w", err)
		}
	}

	if err := tw.Flush(); err != nil {
		return fmt.Errorf("flush alert-once table: %w", err)
	}
	return nil
}

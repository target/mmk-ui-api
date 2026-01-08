package bootstrap

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/target/mmk-ui-api/config"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/observability/notify/pagerduty"
	"github.com/target/mmk-ui-api/internal/observability/notify/slack"
	"github.com/target/mmk-ui-api/internal/observability/statsd"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/service/failurenotifier"
	"github.com/target/mmk-ui-api/internal/service/rules"
)

// ServiceContainer holds all application services.
type ServiceContainer struct {
	Jobs            *service.JobService
	Events          *service.EventService
	Secrets         *service.SecretService
	HTTPAlertSinks  *service.HTTPAlertSinkService
	AlertSinkSvc    *service.AlertSinkService // For test fire and dispatch logic
	Alerts          *service.AlertService
	Sources         *service.SourceService
	Sites           *service.SiteService
	Filter          *service.EventFilterService
	Auth            *service.AuthService
	DomainAllowlist *service.DomainAllowlistService
	IOC             *service.IOCService
	Orchestrator    *service.RulesOrchestrationService
	JobResults      core.JobResultRepository
	Observability   ObservabilityContainer
}

// ObservabilityContainer groups shared observability dependencies.
type ObservabilityContainer struct {
	RulesMetrics    *service.RulesEngineMetricsService
	MetricsSink     *statsd.Client
	MetricsConfig   config.ObservabilityMetricsConfig
	FailureNotifier *failurenotifier.Service
	NotifierConfig  config.ObservabilityNotificationsConfig
}

// ServiceDeps groups dependencies for service initialization.
type ServiceDeps struct {
	Config      *config.AppConfig
	DB          *sql.DB
	RedisClient redis.UniversalClient
	Logger      *slog.Logger
}

// serviceRepositories groups data adapters backing service ports.
type serviceRepositories struct {
	DB                     *sql.DB
	Redis                  redis.UniversalClient
	JobRepo                *data.JobRepo
	EventRepo              *data.EventRepo
	SecretRepo             *data.SecretRepo
	HTTPAlertSinkRepo      *data.HTTPAlertSinkRepo
	AlertRepo              *data.AlertRepo
	SourceRepo             *data.SourceRepo
	DomainAllowlistRepo    *data.DomainAllowlistRepo
	IOCRepo                *data.IOCRepo
	SeenDomainRepo         *data.SeenDomainRepo
	SiteRepo               *data.SiteRepo
	ScheduledJobsAdminRepo *data.ScheduledJobsAdminRepo
	CacheRepo              *data.RedisCacheRepo
	JobResultRepo          *data.JobResultRepo
}

// buildObservability configures metrics and notification adapters.
func buildObservability(logger *slog.Logger, cfg config.ObservabilityConfig) ObservabilityContainer {
	obsLogger := logger
	if obsLogger == nil {
		obsLogger = slog.Default()
	}

	rulesMetrics := service.NewRulesEngineMetricsService()

	var metricsSink *statsd.Client
	if cfg.Metrics.IsEnabled() {
		client, err := statsd.NewClient(statsd.Config{
			Enabled: true,
			Address: cfg.Metrics.StatsdAddress,
			Prefix:  "merrymaker",
			Logger:  obsLogger,
		})
		if err != nil {
			obsLogger.Error("failed to initialise statsd client", "error", err)
		} else {
			metricsSink = client
			rulesMetrics.SetSink(metricsSink)
		}
	}

	failureNotifier := buildFailureNotifier(obsLogger, cfg.Notifications)

	return ObservabilityContainer{
		RulesMetrics:    rulesMetrics,
		MetricsSink:     metricsSink,
		MetricsConfig:   cfg.Metrics,
		FailureNotifier: failureNotifier,
		NotifierConfig:  cfg.Notifications,
	}
}

// buildRepositories builds repositories backing service ports; no business rules here.
func buildRepositories(db *sql.DB, redis redis.UniversalClient) *serviceRepositories {
	return &serviceRepositories{
		DB:                     db,
		Redis:                  redis,
		JobRepo:                data.NewJobRepo(db, data.RepoConfig{}),
		EventRepo:              &data.EventRepo{DB: db},
		HTTPAlertSinkRepo:      data.NewHTTPAlertSinkRepo(db),
		AlertRepo:              data.NewAlertRepo(db),
		SourceRepo:             data.NewSourceRepo(db),
		DomainAllowlistRepo:    data.NewDomainAllowlistRepo(db),
		IOCRepo:                data.NewIOCRepo(db),
		SeenDomainRepo:         data.NewSeenDomainRepo(db),
		SiteRepo:               data.NewSiteRepo(db),
		ScheduledJobsAdminRepo: data.NewScheduledJobsAdminRepo(db),
		CacheRepo:              data.NewRedisCacheRepo(redis),
		JobResultRepo:          data.NewJobResultRepo(db),
	}
}

// ensureSecretRepo attaches the encrypted secret repo once config is available.
func ensureSecretRepo(repos *serviceRepositories, cfg *config.AppConfig, logger *slog.Logger) {
	if cfg == nil || cfg.SecretsEncryptionKey == "" {
		log := logger
		if log == nil {
			log = slog.Default()
		}
		log.Warn("secrets encryption key is empty; secret repo will use a default encryptor")
	}
	key := ""
	if cfg != nil {
		key = cfg.SecretsEncryptionKey
	}
	repos.SecretRepo = data.NewSecretRepo(repos.DB, CreateEncryptor(key, logger))
}

func newSourceCacheService(repos *serviceRepositories, cfg config.CacheConfig) *core.SourceCacheService {
	if repos.CacheRepo == nil {
		return nil
	}
	cacheCfg := core.DefaultSourceCacheConfig()
	if cfg.SourceTTL > 0 {
		cacheCfg.TTL = cfg.SourceTTL
	}
	return core.NewSourceCacheService(core.SourceCacheServiceOptions{
		Cache:   repos.CacheRepo,
		Sources: repos.SourceRepo,
		Secrets: repos.SecretRepo,
		Config:  cacheCfg,
	})
}

func newJobService(repos *serviceRepositories, observability ObservabilityContainer) *service.JobService {
	return service.MustNewJobService(service.JobServiceOptions{
		Repo:            repos.JobRepo,
		DefaultLease:    30 * time.Second,
		FailureNotifier: observability.FailureNotifier,
		Sites:           repos.SiteRepo,
	})
}

func newEventService(eventRepo *data.EventRepo, logger *slog.Logger) *service.EventService {
	return service.MustNewEventService(service.EventServiceOptions{
		Repo: eventRepo,
		Config: service.EventServiceConfig{
			MaxBatch:                 1000,
			ThreatScoreProcessCutoff: 0.7,
		},
		Logger: logger,
	})
}

func newSecretService(repos *serviceRepositories, cfg *config.AppConfig, logger *slog.Logger) *service.SecretService {
	debugMode := false
	if cfg != nil {
		debugMode = cfg.SecretRefreshRunner.DebugMode
	}
	refreshSvc := service.MustNewSecretRefreshService(service.SecretRefreshServiceOptions{
		SecretRepo: repos.SecretRepo,
		AdminRepo:  repos.ScheduledJobsAdminRepo,
		JobRepo:    repos.JobRepo,
		Logger:     logger,
		DebugMode:  debugMode,
	})

	return service.MustNewSecretService(service.SecretServiceOptions{
		Repo:       repos.SecretRepo,
		RefreshSvc: refreshSvc,
		Logger:     logger,
	})
}

type alertingBundle struct {
	httpSink     *service.HTTPAlertSinkService
	alertSinkSvc *service.AlertSinkService
	alert        *service.AlertService
}

func newAlertingServices(repos *serviceRepositories, baseURL string, logger *slog.Logger) alertingBundle {
	httpSink := service.MustNewHTTPAlertSinkService(service.HTTPAlertSinkServiceOptions{
		Repo:   repos.HTTPAlertSinkRepo,
		Logger: logger,
	})

	alertSinkSvc := service.NewAlertSinkService(service.AlertSinkServiceOptions{
		JobRepo:    repos.JobRepo,
		SecretRepo: repos.SecretRepo,
		Evaluator:  nil,
	})

	dispatcher := service.NewAlertDispatchService(service.AlertDispatchServiceOptions{
		Sites:     repos.SiteRepo,
		Sinks:     repos.HTTPAlertSinkRepo,
		AlertSink: alertSinkSvc,
		BaseURL:   baseURL,
		Logger:    logger,
	})

	alert := service.MustNewAlertService(service.AlertServiceOptions{
		Repo:       repos.AlertRepo,
		Sites:      repos.SiteRepo,
		Dispatcher: dispatcher,
		Logger:     logger,
	})

	return alertingBundle{
		httpSink:     httpSink,
		alertSinkSvc: alertSinkSvc,
		alert:        alert,
	}
}

func newDomainAllowlistService(repo *data.DomainAllowlistRepo, logger *slog.Logger) *service.DomainAllowlistService {
	return service.NewDomainAllowlistService(service.DomainAllowlistServiceOptions{
		Repo:   repo,
		Logger: logger,
	})
}

type sourceServiceDeps struct {
	Repo       *data.SourceRepo
	JobRepo    *data.JobRepo
	SecretRepo core.SecretRepository
	Cache      *core.SourceCacheService
}

func newSourceService(deps sourceServiceDeps) *service.SourceService {
	opts := service.SourceServiceOptions{
		SourceRepo: deps.Repo,
		Jobs:       deps.JobRepo,
		SecretRepo: deps.SecretRepo,
	}
	if deps.Cache != nil {
		opts.Cache = deps.Cache
	}
	return service.NewSourceService(opts)
}

type rulesRuntime struct {
	orchestrator *service.RulesOrchestrationService
	iocService   *service.IOCService
}

type RulesRuntimeOptions struct {
	Repos           *serviceRepositories
	AlertSvc        *service.AlertService
	DomainAllowlist *service.DomainAllowlistService
	Observability   ObservabilityContainer
	Logger          *slog.Logger
}

func newRulesRuntime(opts RulesRuntimeOptions) rulesRuntime {
	iocVersioner := rules.NewIOCCacheVersioner(opts.Repos.CacheRepo, "", 0)

	var cacheMetrics rules.CacheMetrics
	if opts.Observability.RulesMetrics != nil {
		cacheMetrics = opts.Observability.RulesMetrics.GetMetrics()
	}

	orchestrator := buildRulesOrchestrator(OrchestratorDeps{
		Events:       opts.Repos.EventRepo,
		Jobs:         opts.Repos.JobRepo,
		Sites:        opts.Repos.SiteRepo,
		Cache:        opts.Repos.CacheRepo,
		JobResults:   opts.Repos.JobResultRepo,
		Seen:         opts.Repos.SeenDomainRepo,
		IOCs:         opts.Repos.IOCRepo,
		Alert:        opts.AlertSvc,
		Allowlist:    opts.DomainAllowlist,
		Log:          opts.Logger,
		CacheMetrics: cacheMetrics,
		IOCVersioner: iocVersioner,
	})

	iocService := service.MustNewIOCService(service.IOCServiceOptions{
		Repo:           opts.Repos.IOCRepo,
		Logger:         opts.Logger,
		CacheVersioner: iocVersioner,
	})

	return rulesRuntime{
		orchestrator: orchestrator,
		iocService:   iocService,
	}
}

func newAuthService(cfg config.AuthConfig, redis redis.UniversalClient, logger *slog.Logger) *service.AuthService {
	return BuildAuthService(AuthConfig{
		Auth:        cfg,
		RedisClient: redis,
		Logger:      logger,
	})
}

func newSiteService(repo *data.SiteRepo, admin *data.ScheduledJobsAdminRepo) *service.SiteService {
	return service.NewSiteService(service.SiteServiceOptions{
		SiteRepo: repo,
		Admin:    admin,
	})
}

type DomainServicesOptions struct {
	Repos         *serviceRepositories
	Observability ObservabilityContainer
	Config        *config.AppConfig
	Logger        *slog.Logger
}

// buildDomainServices wires business services using repositories and observability adapters.
func buildDomainServices(opts *DomainServicesOptions) ServiceContainer {
	if opts == nil {
		return ServiceContainer{}
	}
	svcLogger := opts.Logger
	if svcLogger == nil {
		svcLogger = slog.Default()
	}

	appCfg := opts.Config
	if appCfg == nil {
		appCfg = &config.AppConfig{}
	}

	ensureSecretRepo(opts.Repos, appCfg, svcLogger)
	jobService := newJobService(opts.Repos, opts.Observability)
	eventService := newEventService(opts.Repos.EventRepo, svcLogger)
	secretService := newSecretService(opts.Repos, appCfg, svcLogger)
	alerting := newAlertingServices(opts.Repos, appCfg.HTTP.BaseURL, svcLogger)
	domainAllowlistService := newDomainAllowlistService(opts.Repos.DomainAllowlistRepo, svcLogger)
	sourceCache := newSourceCacheService(opts.Repos, appCfg.Cache)
	sourceService := newSourceService(sourceServiceDeps{
		Repo:       opts.Repos.SourceRepo,
		JobRepo:    opts.Repos.JobRepo,
		SecretRepo: opts.Repos.SecretRepo,
		Cache:      sourceCache,
	})
	rules := newRulesRuntime(RulesRuntimeOptions{
		Repos:           opts.Repos,
		AlertSvc:        alerting.alert,
		DomainAllowlist: domainAllowlistService,
		Observability:   opts.Observability,
		Logger:          svcLogger,
	})
	authService := newAuthService(appCfg.Auth, opts.Repos.Redis, svcLogger)
	siteService := newSiteService(opts.Repos.SiteRepo, opts.Repos.ScheduledJobsAdminRepo)

	return ServiceContainer{
		Jobs:            jobService,
		Events:          eventService,
		Secrets:         secretService,
		HTTPAlertSinks:  alerting.httpSink,
		AlertSinkSvc:    alerting.alertSinkSvc,
		Alerts:          alerting.alert,
		Sources:         sourceService,
		Sites:           siteService,
		Filter:          service.NewEventFilterService(),
		Auth:            authService,
		DomainAllowlist: domainAllowlistService,
		IOC:             rules.iocService,
		Orchestrator:    rules.orchestrator,
		JobResults:      opts.Repos.JobResultRepo,
		Observability:   opts.Observability,
	}
}

func NewServices(deps *ServiceDeps) ServiceContainer {
	if deps == nil {
		return ServiceContainer{}
	}
	logger := deps.Logger
	if logger == nil {
		logger = slog.Default()
	}

	var obsCfg config.ObservabilityConfig
	if deps.Config != nil {
		obsCfg = deps.Config.Observability
	}
	observability := buildObservability(logger, obsCfg)
	repos := buildRepositories(deps.DB, deps.RedisClient)
	return buildDomainServices(&DomainServicesOptions{
		Repos:         repos,
		Observability: observability,
		Config:        deps.Config,
		Logger:        logger,
	})
}

func buildFailureNotifier(logger *slog.Logger, cfg config.ObservabilityNotificationsConfig) *failurenotifier.Service {
	baseLogger := logger
	if baseLogger == nil {
		baseLogger = slog.Default()
	}

	if !cfg.Enabled {
		return failurenotifier.NewService(failurenotifier.Options{
			Logger: baseLogger.With("component", "failure_notifier"),
		})
	}

	sinks := make([]failurenotifier.SinkRegistration, 0, 2)

	if cfg.Slack.Enabled {
		client, err := slack.NewClient(slack.Config{
			WebhookURL:    cfg.Slack.WebhookURL,
			Channel:       cfg.Slack.Channel,
			Username:      cfg.Slack.Username,
			Timeout:       cfg.Timeout,
			RetryLimit:    cfg.RetryLimit,
			SiteURLPrefix: cfg.Slack.SiteURLPrefix,
		})
		if err != nil {
			baseLogger.Error("failed to initialise slack notifier", "error", err)
		} else {
			sinks = append(sinks, failurenotifier.SinkRegistration{
				Name: "slack",
				Sink: client,
			})
		}
	}

	if cfg.PagerDuty.Enabled {
		client, err := pagerduty.NewClient(pagerduty.Config{
			RoutingKey: cfg.PagerDuty.RoutingKey,
			Source:     cfg.PagerDuty.Source,
			Component:  cfg.PagerDuty.Component,
			Timeout:    cfg.Timeout,
			RetryLimit: cfg.RetryLimit,
		})
		if err != nil {
			baseLogger.Error("failed to initialise pagerduty notifier", "error", err)
		} else {
			sinks = append(sinks, failurenotifier.SinkRegistration{
				Name: "pagerduty",
				Sink: client,
			})
		}
	}

	return failurenotifier.NewService(failurenotifier.Options{
		Logger: baseLogger.With("component", "failure_notifier"),
		Sinks:  sinks,
	})
}

// OrchestratorDeps groups dependencies for rules orchestrator.
type OrchestratorDeps struct {
	Events       core.EventRepository
	Jobs         core.JobRepository
	Sites        core.SiteRepository
	Cache        core.CacheRepository
	JobResults   core.JobResultRepository
	Seen         core.SeenDomainRepository
	IOCs         core.IOCRepository
	Files        core.ProcessedFileRepository
	Alert        rules.AlertCreator
	Allowlist    rules.DomainAllowlistService
	Log          *slog.Logger
	CacheMetrics rules.CacheMetrics
	IOCVersioner rules.IOCVersioner
}

func buildRulesOrchestrator(d OrchestratorDeps) *service.RulesOrchestrationService {
	if d.Log == nil {
		d.Log = slog.Default()
	}

	cacheOpts := rules.DefaultCachesOptions()
	cacheOpts.Redis = d.Cache
	cacheOpts.SeenRepo = d.Seen
	cacheOpts.IOCsRepo = d.IOCs
	cacheOpts.FilesRepo = d.Files
	cacheOpts.Metrics = d.CacheMetrics
	cacheOpts.IOCVersioner = d.IOCVersioner

	caches := rules.BuildCaches(cacheOpts)
	allowlistChecker := newDomainAllowlistChecker(d.Allowlist)
	unknownDomainEvaluator := newUnknownDomainEvaluator(unknownDomainEvaluatorOptions{
		Alert:     d.Alert,
		Caches:    caches,
		Allowlist: allowlistChecker,
		Logger:    d.Log,
	})
	globalIOCEvaluator := newIOCEvaluator(d.Alert, caches)

	return service.NewRulesOrchestrationService(service.RulesOrchestrationOptions{
		Events:                 d.Events,
		Jobs:                   d.Jobs,
		Sites:                  d.Sites,
		Caches:                 caches,
		Logger:                 d.Log,
		DedupeCache:            d.Cache,
		DedupeTTL:              2 * time.Minute,
		JobResults:             d.JobResults,
		UnknownDomainEvaluator: unknownDomainEvaluator,
		IOCEvaluator:           globalIOCEvaluator,
	})
}

func newDomainAllowlistChecker(svc rules.DomainAllowlistService) *rules.DomainAllowlistChecker {
	if svc == nil {
		return nil
	}
	return rules.NewDomainAllowlistChecker(rules.DomainAllowlistCheckerOptions{
		Service:   svc,
		CacheTTL:  5 * time.Minute,
		CacheSize: 1000,
	})
}

type unknownDomainEvaluatorOptions struct {
	Alert     rules.AlertCreator
	Caches    rules.Caches
	Allowlist rules.AllowlistChecker
	Logger    *slog.Logger
}

func newUnknownDomainEvaluator(opts unknownDomainEvaluatorOptions) *rules.UnknownDomainEvaluator {
	if opts.Alert == nil {
		return nil
	}
	return &rules.UnknownDomainEvaluator{
		Caches:    opts.Caches,
		Alerter:   opts.Alert,
		Allowlist: opts.Allowlist,
		AlertTTL:  24 * time.Hour,
		Logger:    opts.Logger,
	}
}

func newIOCEvaluator(alert rules.AlertCreator, caches rules.Caches) *rules.IOCEvaluator {
	if alert == nil {
		return nil
	}
	return &rules.IOCEvaluator{
		Caches:   caches,
		Alerter:  alert,
		AlertTTL: 24 * time.Hour,
	}
}

// ServiceOrchestrationConfig contains configuration for service orchestration.
type ServiceOrchestrationConfig struct {
	Config      *config.AppConfig
	Services    ServiceContainer
	DB          *sql.DB
	RedisClient redis.UniversalClient
	Logger      *slog.Logger
}

const (
	// shutdownWaitTimeout is the maximum time to wait for services to stop gracefully.
	shutdownWaitTimeout = 15 * time.Second
)

// serviceStartupDeps groups dependencies for service startup.
type serviceStartupDeps struct {
	ctx             context.Context
	cfg             *ServiceOrchestrationConfig
	logger          *slog.Logger
	encryptor       cryptoutil.Encryptor
	enabledServices map[config.ServiceMode]bool
	errCh           chan error
}

// backgroundService describes a startable background component.
type backgroundService struct {
	mode  config.ServiceMode
	name  string
	start func(context.Context) error
}

// backgroundServiceHandle tracks a running background service.
type backgroundServiceHandle struct {
	mode config.ServiceMode
	name string
	done <-chan struct{}
}

// startHTTPServerIfEnabled starts the HTTP server if enabled.
func startHTTPServerIfEnabled(deps *serviceStartupDeps) *http.Server {
	if deps == nil || deps.cfg == nil || !deps.enabledServices[config.ServiceModeHTTP] {
		return nil
	}
	return StartHTTPServer(&HTTPServerConfig{
		Config:   deps.cfg.Config,
		Services: deps.cfg.Services,
		Logger:   deps.logger,
	})
}

func launchBackground(ctx context.Context, deps *serviceStartupDeps, descriptor backgroundService) <-chan struct{} {
	if deps == nil || !deps.enabledServices[descriptor.mode] {
		return nil
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		if err := descriptor.start(ctx); err != nil {
			errMsg := fmt.Errorf("%s failed: %w", descriptor.name, err)
			select {
			case deps.errCh <- errMsg:
			case <-ctx.Done():
			default:
				if deps.logger != nil {
					deps.logger.WarnContext(
						ctx,
						"dropping background service error",
						"service",
						descriptor.name,
						"error",
						errMsg,
					)
				} else {
					slog.Default().WarnContext(ctx, "dropping background service error", "service", descriptor.name, "error", errMsg)
				}
			}
		}
	}()

	if deps.logger != nil {
		deps.logger.InfoContext(ctx, "background service started", "service", descriptor.name, "mode", descriptor.mode)
	} else {
		slog.Default().InfoContext(ctx, "background service started", "service", descriptor.name, "mode", descriptor.mode)
	}

	return done
}

func startBackgroundServices(deps *serviceStartupDeps, services []backgroundService) []backgroundServiceHandle {
	if deps == nil {
		return nil
	}
	handles := make([]backgroundServiceHandle, 0, len(services))

	for _, svc := range services {
		done := launchBackground(deps.ctx, deps, svc)
		if done == nil {
			continue
		}

		handles = append(handles, backgroundServiceHandle{
			mode: svc.mode,
			name: svc.name,
			done: done,
		})
	}

	return handles
}

func newRulesEngineBackgroundService(deps *serviceStartupDeps) backgroundService {
	return backgroundService{
		mode: config.ServiceModeRulesEngine,
		name: "rules engine",
		start: func(ctx context.Context) error {
			if deps == nil {
				return nil
			}
			var cacheMetrics rules.CacheMetrics
			if deps.cfg.Services.Observability.RulesMetrics != nil {
				cacheMetrics = deps.cfg.Services.Observability.RulesMetrics.GetMetrics()
			}
			lease := time.Duration(0)
			concurrency := 0
			if deps.cfg.Config != nil {
				lease = deps.cfg.Config.RulesEngine.JobLease
				concurrency = deps.cfg.Config.RulesEngine.Concurrency
			}
			return RunRulesEngine(ctx, RulesEngineConfig{
				DB:              deps.cfg.DB,
				RedisClient:     deps.cfg.RedisClient,
				Logger:          deps.logger,
				Lease:           lease,
				Concurrency:     concurrency,
				CacheMetrics:    cacheMetrics,
				Metrics:         deps.cfg.Services.Observability.MetricsSink,
				FailureNotifier: deps.cfg.Services.Observability.FailureNotifier,
			})
		},
	}
}

func newSchedulerBackgroundService(deps *serviceStartupDeps) backgroundService {
	return backgroundService{
		mode: config.ServiceModeScheduler,
		name: "scheduler",
		start: func(ctx context.Context) error {
			if deps == nil || deps.cfg == nil {
				return nil
			}
			schedulerCfg := config.SchedulerConfig{}
			cacheCfg := config.CacheConfig{}
			if deps.cfg.Config != nil {
				schedulerCfg = deps.cfg.Config.Scheduler
				cacheCfg = deps.cfg.Config.Cache
			}
			return RunScheduler(ctx, SchedulerConfig{
				DB:                 deps.cfg.DB,
				RedisClient:        deps.cfg.RedisClient,
				Logger:             deps.logger,
				BatchSize:          schedulerCfg.BatchSize,
				DefaultJobType:     schedulerCfg.DefaultJobType,
				DefaultPriority:    schedulerCfg.DefaultPriority,
				MaxRetries:         schedulerCfg.MaxRetries,
				OverrunPolicy:      schedulerCfg.OverrunPolicy,
				OverrunStates:      schedulerCfg.OverrunStates,
				Interval:           schedulerCfg.Interval,
				SourceCacheEnabled: deps.cfg.RedisClient != nil,
				SourceCacheTTL:     cacheCfg.SourceTTL,
				Encryptor:          deps.encryptor,
				Metrics:            deps.cfg.Services.Observability.MetricsSink,
			})
		},
	}
}

func newAlertRunnerBackgroundService(deps *serviceStartupDeps) backgroundService {
	return backgroundService{
		mode: config.ServiceModeAlertRunner,
		name: "alert runner",
		start: func(ctx context.Context) error {
			if deps == nil || deps.cfg == nil {
				return nil
			}
			var lease time.Duration
			concurrency := 0
			if deps.cfg.Config != nil {
				lease = deps.cfg.Config.AlertRunner.JobLease
				concurrency = deps.cfg.Config.AlertRunner.Concurrency
			}
			return RunAlertRunner(ctx, AlertRunnerConfig{
				DB:              deps.cfg.DB,
				Logger:          deps.logger,
				Lease:           lease,
				Concurrency:     concurrency,
				Encryptor:       deps.encryptor,
				Metrics:         deps.cfg.Services.Observability.MetricsSink,
				FailureNotifier: deps.cfg.Services.Observability.FailureNotifier,
			})
		},
	}
}

func newSecretRefreshBackgroundService(deps *serviceStartupDeps) backgroundService {
	return backgroundService{
		mode: config.ServiceModeSecretRefreshRunner,
		name: "secret refresh runner",
		start: func(ctx context.Context) error {
			if deps == nil || deps.cfg == nil {
				return nil
			}
			var lease time.Duration
			concurrency := 0
			debugMode := false
			if deps.cfg.Config != nil {
				lease = deps.cfg.Config.SecretRefreshRunner.JobLease
				concurrency = deps.cfg.Config.SecretRefreshRunner.Concurrency
				debugMode = deps.cfg.Config.SecretRefreshRunner.DebugMode
			}
			return RunSecretRefreshRunner(ctx, SecretRefreshRunnerConfig{
				DB:              deps.cfg.DB,
				Logger:          deps.logger,
				Lease:           lease,
				Concurrency:     concurrency,
				DebugMode:       debugMode,
				Encryptor:       deps.encryptor,
				Metrics:         deps.cfg.Services.Observability.MetricsSink,
				FailureNotifier: deps.cfg.Services.Observability.FailureNotifier,
			})
		},
	}
}

func newReaperBackgroundService(deps *serviceStartupDeps) backgroundService {
	return backgroundService{
		mode: config.ServiceModeReaper,
		name: "reaper",
		start: func(ctx context.Context) error {
			if deps == nil || deps.cfg == nil {
				return nil
			}
			var reaperCfg config.ReaperConfig
			if deps.cfg.Config != nil {
				reaperCfg = deps.cfg.Config.Reaper
			}
			return RunReaper(ctx, ReaperConfig{
				DB:      deps.cfg.DB,
				Logger:  deps.logger,
				Config:  reaperCfg,
				Metrics: deps.cfg.Services.Observability.MetricsSink,
			})
		},
	}
}

func buildBackgroundServices(deps *serviceStartupDeps) []backgroundService {
	if deps == nil {
		return nil
	}
	return []backgroundService{
		newRulesEngineBackgroundService(deps),
		newSchedulerBackgroundService(deps),
		newAlertRunnerBackgroundService(deps),
		newSecretRefreshBackgroundService(deps),
		newReaperBackgroundService(deps),
	}
}

// ServiceStartupResult holds the results of starting all services.
type ServiceStartupResult struct {
	HTTPServer *http.Server
	Background []backgroundServiceHandle
}

// startServices starts all enabled services and returns their completion channels.
func startServices(deps *serviceStartupDeps) ServiceStartupResult {
	return ServiceStartupResult{
		HTTPServer: startHTTPServerIfEnabled(deps),
		Background: startBackgroundServices(deps, buildBackgroundServices(deps)),
	}
}

// RunServicesWithShutdown starts all enabled services and manages their lifecycle.
// This function blocks until a shutdown signal is received or a service fails.
func RunServicesWithShutdown(cfg *ServiceOrchestrationConfig) error {
	if cfg == nil {
		return errors.New("service orchestration config is required")
	}
	ctx := context.Background()
	serviceCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}

	if cfg.Config == nil {
		return errors.New("service orchestration config missing AppConfig")
	}

	encryptor := CreateEncryptor(cfg.Config.SecretsEncryptionKey, logger)

	// Determine which services are enabled
	enabledServices, err := cfg.Config.GetEnabledServices()
	if err != nil {
		return fmt.Errorf("determine enabled services: %w", err)
	}
	errCh := make(chan error, errorChannelBufferSize(enabledServices))

	// Start all enabled services
	result := startServices(&serviceStartupDeps{
		ctx:             serviceCtx,
		cfg:             cfg,
		logger:          logger,
		encryptor:       encryptor,
		enabledServices: enabledServices,
		errCh:           errCh,
	})

	// Wait for shutdown signal or error
	return waitForShutdown(shutdownConfig{
		ctx:         serviceCtx,
		cancel:      cancel,
		errCh:       errCh,
		httpServer:  result.HTTPServer,
		jobService:  cfg.Services.Jobs,
		logger:      logger,
		backgrounds: result.Background,
	})
}

func errorChannelCapacity(enabled map[config.ServiceMode]bool) int {
	modes := []config.ServiceMode{
		config.ServiceModeHTTP,
		config.ServiceModeRulesEngine,
		config.ServiceModeScheduler,
		config.ServiceModeAlertRunner,
		config.ServiceModeSecretRefreshRunner,
		config.ServiceModeReaper,
	}

	count := 0
	for _, mode := range modes {
		if enabled[mode] {
			count++
		}
	}
	return count
}

func errorChannelBufferSize(enabled map[config.ServiceMode]bool) int {
	size := errorChannelCapacity(enabled) + 1
	if size < 1 {
		return 1
	}
	return size
}

// shutdownConfig contains dependencies for graceful shutdown.
type shutdownConfig struct {
	ctx         context.Context
	cancel      context.CancelFunc
	errCh       <-chan error
	httpServer  *http.Server
	jobService  *service.JobService
	logger      *slog.Logger
	backgrounds []backgroundServiceHandle
}

// waitForShutdown waits for shutdown signal or service error.
func waitForShutdown(cfg shutdownConfig) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(quit)

	select {
	case <-quit:
		cfg.logger.Info("shutting down services...")
		cfg.cancel() // Cancel service context before waiting
		return gracefulStop(cfg)
	case err := <-cfg.errCh:
		cfg.logger.Error("service error", "error", err)
		cfg.cancel() // Cancel service context before waiting
		if stopErr := gracefulStop(cfg); stopErr != nil {
			cfg.logger.Error("graceful stop failed", "error", stopErr)
		}
		return err
	}
}

// gracefulStop attempts to gracefully stop all services.
func gracefulStop(cfg shutdownConfig) error {
	// Gracefully stop HTTP server if running
	if cfg.httpServer != nil {
		// Create a timeout context for HTTP shutdown
		shutdownCtx, cancel := context.WithTimeout(cfg.ctx, shutdownWaitTimeout)
		defer cancel()

		if err := ShutdownHTTPServer(ShutdownConfig{
			Context:    shutdownCtx,
			Server:     cfg.httpServer,
			JobService: cfg.jobService,
			Logger:     cfg.logger,
		}); err != nil {
			return err
		}
	}

	// Wait for background services to finish
	for _, svc := range cfg.backgrounds {
		waitForService(svc.done, svc.name, cfg.logger)
	}

	return nil
}

// waitForService waits for a service to finish with timeout.
func waitForService(done <-chan struct{}, name string, logger *slog.Logger) {
	if done == nil {
		return
	}
	select {
	case <-done:
		logger.Info(name + " stopped")
	case <-time.After(shutdownWaitTimeout):
		logger.Warn("timeout waiting for " + name + " to stop")
	}
}

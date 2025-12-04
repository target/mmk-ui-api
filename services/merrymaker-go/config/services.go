package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// ServiceMode represents the available service modes.
type ServiceMode string

const (
	// ServiceModeHTTP runs the HTTP server.
	ServiceModeHTTP ServiceMode = "http"
	// ServiceModeRulesEngine runs the rules engine worker.
	ServiceModeRulesEngine ServiceMode = "rules-engine"
	// ServiceModeScheduler runs the job scheduler.
	ServiceModeScheduler ServiceMode = "scheduler"
	// ServiceModeReaper runs the job reaper for cleanup.
	ServiceModeReaper ServiceMode = "reaper"
	// ServiceModeAlertRunner runs the alert delivery job runner.
	ServiceModeAlertRunner ServiceMode = "alert-runner"
	// ServiceModeSecretRefreshRunner runs the secret refresh job runner.
	ServiceModeSecretRefreshRunner ServiceMode = "secret-refresh-runner"
)

// ValidServiceModes returns all valid service mode names.
func ValidServiceModes() []ServiceMode {
	return []ServiceMode{
		ServiceModeHTTP,
		ServiceModeRulesEngine,
		ServiceModeScheduler,
		ServiceModeReaper,
		ServiceModeAlertRunner,
		ServiceModeSecretRefreshRunner,
	}
}

// ParseServices parses a comma-delimited string of service names and returns the enabled services.
// It validates that all service names are valid and returns an error if any are invalid.
func ParseServices(servicesStr string) (map[ServiceMode]bool, error) {
	services := make(map[ServiceMode]bool)

	if servicesStr == "" {
		return services, errors.New("at least one service must be specified")
	}

	parts := strings.Split(servicesStr, ",")
	for _, part := range parts {
		serviceName := strings.TrimSpace(part)
		if serviceName == "" {
			continue
		}

		mode := ServiceMode(serviceName)
		switch mode {
		case ServiceModeHTTP,
			ServiceModeRulesEngine,
			ServiceModeScheduler,
			ServiceModeReaper,
			ServiceModeAlertRunner,
			ServiceModeSecretRefreshRunner:
			services[mode] = true
		default:
			return nil, fmt.Errorf(
				"invalid service name: %q (valid options: http, rules-engine, scheduler, reaper, alert-runner, secret-refresh-runner)",
				serviceName,
			)
		}
	}

	if len(services) == 0 {
		return nil, errors.New("at least one valid service must be specified")
	}

	return services, nil
}

// SchedulerConfig contains scheduler service configuration.
type SchedulerConfig struct {
	// BatchSize is the number of jobs to schedule per batch.
	BatchSize int `env:"SCHEDULER_BATCH_SIZE" envDefault:"25"`

	// DefaultJobType is the default job type for scheduled jobs.
	DefaultJobType model.JobType `env:"SCHEDULER_DEFAULT_JOB_TYPE" envDefault:"browser"`

	// DefaultPriority is the default priority for scheduled jobs.
	DefaultPriority int `env:"SCHEDULER_DEFAULT_PRIORITY" envDefault:"0"`

	// MaxRetries is the maximum number of retries for failed jobs.
	MaxRetries int `env:"SCHEDULER_MAX_RETRIES" envDefault:"3"`

	// OverrunPolicy determines how to handle jobs that overrun their schedule.
	// Valid values: skip, queue, reschedule
	OverrunPolicy domain.OverrunPolicy `env:"SCHEDULER_OVERRUN" envDefault:"skip"`

	// OverrunStates defines which job states block new enqueue attempts when OverrunPolicy=skip.
	// Comma-separated list of: running, pending, retrying.
	OverrunStates domain.OverrunStateMask `env:"SCHEDULER_OVERRUN_STATES" envDefault:"running"`

	// Interval is the scheduler tick interval.
	Interval time.Duration `env:"SCHEDULER_INTERVAL" envDefault:"1s"`
}

// Sanitize applies guardrails to scheduler configuration values.
func (s *SchedulerConfig) Sanitize() {
	if s.BatchSize < 1 {
		s.BatchSize = 1
	}
	if s.OverrunStates == 0 {
		s.OverrunStates = domain.OverrunStatesDefault
	}
}

// RulesEngineConfig contains rules engine service configuration.
type RulesEngineConfig struct {
	// Concurrency is the number of worker goroutines.
	Concurrency int `env:"RULES_ENGINE_CONCURRENCY" envDefault:"1"`

	// JobLease is the duration to lease a rules job.
	JobLease time.Duration `env:"RULES_ENGINE_JOB_LEASE" envDefault:"30s"`

	// BatchSize is the maximum number of events to process per rules job.
	BatchSize int `env:"RULES_ENGINE_BATCH_SIZE" envDefault:"100"`

	// AutoEnqueue determines whether to auto-enqueue rules jobs on event ingestion.
	AutoEnqueue bool `env:"RULES_ENGINE_AUTO_ENQUEUE" envDefault:"true"`
}

// Sanitize applies guardrails to rules engine configuration values.
func (r *RulesEngineConfig) Sanitize() {
	if r.BatchSize < 1 {
		r.BatchSize = 1
	}
}

// AlertRunnerConfig contains alert runner service configuration.
type AlertRunnerConfig struct {
	// Concurrency is the number of worker goroutines.
	Concurrency int `env:"ALERT_RUNNER_CONCURRENCY" envDefault:"2"`

	// JobLease is the duration to lease an alert job.
	JobLease time.Duration `env:"ALERT_RUNNER_JOB_LEASE" envDefault:"30s"`
}

// Sanitize applies guardrails to alert runner configuration values.
func (a *AlertRunnerConfig) Sanitize() {
	if a.Concurrency < 1 {
		a.Concurrency = 1
	}
	if a.JobLease < 5*time.Second {
		a.JobLease = 5 * time.Second
	}
}

// SecretRefreshRunnerConfig contains secret refresh runner service configuration.
type SecretRefreshRunnerConfig struct {
	// Concurrency is the number of worker goroutines.
	Concurrency int `env:"SECRET_REFRESH_RUNNER_CONCURRENCY" envDefault:"2"`

	// JobLease is the duration to lease a secret refresh job.
	JobLease time.Duration `env:"SECRET_REFRESH_RUNNER_JOB_LEASE" envDefault:"30s"`

	// DebugMode enables logging of actual secret values (DANGEROUS - only for debugging).
	// When enabled, the actual secret values resolved from provider scripts will be logged.
	// WARNING: This is a security risk and should NEVER be enabled in production.
	DebugMode bool `env:"SECRET_REFRESH_DEBUG_MODE" envDefault:"false"`
}

// Sanitize applies guardrails to secret refresh runner configuration values.
func (s *SecretRefreshRunnerConfig) Sanitize() {
	if s.Concurrency < 1 {
		s.Concurrency = 1
	}
	if s.JobLease < 5*time.Second {
		s.JobLease = 5 * time.Second
	}
}

// ReaperConfig contains job reaper service configuration.
type ReaperConfig struct {
	// Interval is the reaper tick interval.
	Interval time.Duration `env:"REAPER_INTERVAL" envDefault:"5m"`

	// PendingMaxAge is the maximum age for pending jobs before they are marked as failed.
	// Jobs stuck in pending status longer than this will be failed.
	PendingMaxAge time.Duration `env:"REAPER_PENDING_MAX_AGE" envDefault:"1h"`

	// CompletedMaxAge is the maximum age for completed jobs before deletion.
	CompletedMaxAge time.Duration `env:"REAPER_COMPLETED_MAX_AGE" envDefault:"168h"` // 7 days

	// FailedMaxAge is the maximum age for failed jobs before deletion.
	FailedMaxAge time.Duration `env:"REAPER_FAILED_MAX_AGE" envDefault:"168h"` // 7 days

	// JobResultsMaxAge is the maximum age for persisted job_results rows before deletion.
	// These records keep delivery history after their corresponding jobs are reaped.
	JobResultsMaxAge time.Duration `env:"REAPER_JOB_RESULTS_MAX_AGE" envDefault:"2160h"` // 90 days

	// BatchSize is the maximum number of rows to process per operation.
	// Batching prevents long locks and I/O spikes on large tables.
	BatchSize int `env:"REAPER_BATCH_SIZE" envDefault:"1000"`
}

// Sanitize applies guardrails to reaper configuration values.
func (r *ReaperConfig) Sanitize() {
	// Enforce minimum intervals to prevent excessive database load
	if r.Interval < 1*time.Minute {
		r.Interval = 1 * time.Minute
	}
	if r.PendingMaxAge < 5*time.Minute {
		r.PendingMaxAge = 5 * time.Minute
	}
	if r.CompletedMaxAge < 1*time.Hour {
		r.CompletedMaxAge = 1 * time.Hour
	}
	if r.FailedMaxAge < 1*time.Hour {
		r.FailedMaxAge = 1 * time.Hour
	}
	if r.JobResultsMaxAge < 24*time.Hour {
		r.JobResultsMaxAge = 24 * time.Hour
	}

	// Enforce batch size bounds to prevent excessive locks or inefficiency
	if r.BatchSize < 1 {
		r.BatchSize = 1
	}
	if r.BatchSize > 10000 {
		r.BatchSize = 10000
	}
}

// ServicesConfig groups all service-related configuration.
type ServicesConfig struct {
	// Services is a comma-delimited list of enabled services.
	// Valid values: http, rules-engine, scheduler, reaper, alert-runner, secret-refresh-runner
	Services string `env:"SERVICES" envDefault:"http" yaml:"services"`

	// Scheduler configuration.
	Scheduler SchedulerConfig

	// RulesEngine configuration.
	RulesEngine RulesEngineConfig

	// AlertRunner configuration.
	AlertRunner AlertRunnerConfig

	// Reaper configuration.
	Reaper ReaperConfig
}

// GetEnabledServices returns the enabled services based on the Services field.
func (s *ServicesConfig) GetEnabledServices() (map[ServiceMode]bool, error) {
	return ParseServices(s.Services)
}

// IsHTTPServerEnabled returns true if the HTTP server service is enabled.
func (s *ServicesConfig) IsHTTPServerEnabled() bool {
	services, err := s.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeHTTP]
}

// IsRulesEngineEnabled returns true if the rules engine service is enabled.
func (s *ServicesConfig) IsRulesEngineEnabled() bool {
	services, err := s.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeRulesEngine]
}

// IsSchedulerEnabled returns true if the scheduler service is enabled.
func (s *ServicesConfig) IsSchedulerEnabled() bool {
	services, err := s.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeScheduler]
}

// IsReaperEnabled returns true if the reaper service is enabled.
func (s *ServicesConfig) IsReaperEnabled() bool {
	services, err := s.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeReaper]
}

// IsAlertRunnerEnabled returns true if the alert runner service is enabled.
func (s *ServicesConfig) IsAlertRunnerEnabled() bool {
	services, err := s.GetEnabledServices()
	if err != nil {
		return false
	}
	return services[ServiceModeAlertRunner]
}

// Sanitize applies guardrails to services configuration values.
func (s *ServicesConfig) Sanitize() {
	s.Scheduler.Sanitize()
	s.RulesEngine.Sanitize()
	s.AlertRunner.Sanitize()
	s.Reaper.Sanitize()
}

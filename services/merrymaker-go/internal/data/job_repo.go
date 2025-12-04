package data

import (
	"database/sql"
	"errors"
	"log/slog"
)

var (
	// ErrJobNotFound is returned when a job is not found.
	ErrJobNotFound = errors.New("job not found")
	// ErrJobNotDeletable is returned when attempting to delete a job that is not in a deletable state.
	ErrJobNotDeletable = errors.New("job cannot be deleted (must be in pending, completed, or failed status)")
	// ErrJobReserved is returned when attempting to delete a job that has an active lease.
	ErrJobReserved = errors.New("job is reserved and cannot be deleted")
)

// RepoConfig holds configuration options for the job repository.
type RepoConfig struct {
	RetryDelaySeconds int
	Logger            *slog.Logger
	TimeProvider      TimeProvider
}

// JobRepo provides database operations for job management.
type JobRepo struct {
	DB           *sql.DB
	cfg          RepoConfig
	timeProvider TimeProvider
	logger       *slog.Logger
}

// NewJobRepo creates a new JobRepo instance with the given database connection and configuration.
func NewJobRepo(db *sql.DB, cfg RepoConfig) *JobRepo {
	tp := cfg.TimeProvider
	if tp == nil {
		tp = &RealTimeProvider{}
	}

	return &JobRepo{
		DB:           db,
		cfg:          cfg,
		timeProvider: tp,
		logger:       cfg.Logger,
	}
}

const jobColumns = `
  id,
  type,
  status,
  priority,
  payload,
  metadata,
  session_id,
  site_id,
  source_id,
  is_test,
  scheduled_at,
  started_at,
  completed_at,
  retry_count,
  max_retries,
  last_error,
  lease_expires_at,
  created_at,
  updated_at
`

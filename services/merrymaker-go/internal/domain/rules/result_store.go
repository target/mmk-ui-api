package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// ResultStoreOptions configure the behavior of ResultStore.
type ResultStoreOptions struct {
	Cache      core.CacheRepository
	CacheTTL   time.Duration
	Repository core.JobResultRepository
	Logger     *slog.Logger
	JobType    model.JobType
	IsNotFound func(error) bool
}

// JobResultStore persists and retrieves rules processing results across cache and repository layers.
type JobResultStore struct {
	cache      core.CacheRepository
	cacheTTL   time.Duration
	repository core.JobResultRepository
	logger     *slog.Logger
	jobType    model.JobType
	isNotFound func(error) bool
}

// NewResultStore constructs a ResultStore with the provided dependencies.
func NewResultStore(opts ResultStoreOptions) *JobResultStore {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	ttl := opts.CacheTTL
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}

	jobType := opts.JobType
	if jobType == "" {
		jobType = model.JobTypeRules
	}

	return &JobResultStore{
		cache:      opts.Cache,
		cacheTTL:   ttl,
		repository: opts.Repository,
		logger:     logger,
		jobType:    jobType,
		isNotFound: opts.IsNotFound,
	}
}

// Cache stores the results in the configured cache, when available.
func (s *JobResultStore) Cache(
	ctx context.Context,
	jobID string,
	results *ProcessingResults,
) error {
	if s == nil || s.cache == nil || results == nil || jobID == "" {
		return nil
	}
	payload, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("marshal results: %w", err)
	}
	if setErr := s.cache.Set(ctx, s.cacheKey(jobID), payload, s.cacheTTL); setErr != nil {
		return fmt.Errorf("cache results: %w", setErr)
	}
	return nil
}

// Persist upserts the results into the backing repository, when configured.
func (s *JobResultStore) Persist(
	ctx context.Context,
	job *model.Job,
	results *ProcessingResults,
) error {
	if s == nil || s.repository == nil || job == nil || results == nil {
		return nil
	}
	payload, err := json.Marshal(results)
	if err != nil {
		return fmt.Errorf("marshal results: %w", err)
	}
	params := core.UpsertJobResultParams{
		JobID:   job.ID,
		JobType: s.jobType,
		Result:  payload,
	}
	if upsertErr := s.repository.Upsert(ctx, params); upsertErr != nil {
		return fmt.Errorf("persist results: %w", upsertErr)
	}
	return nil
}

// Get retrieves results for the provided job ID, checking cache then repository.
func (s *JobResultStore) Get(ctx context.Context, jobID string) (*ProcessingResults, error) {
	if jobID == "" {
		return nil, ErrResultsNotFound
	}
	if res := s.getFromCache(ctx, jobID); res != nil {
		return res, nil
	}
	return s.getFromRepository(ctx, jobID)
}

func (s *JobResultStore) cacheKey(jobID string) string {
	return "rules:results:" + jobID
}

func (s *JobResultStore) getFromCache(ctx context.Context, jobID string) *ProcessingResults {
	if s.cache == nil {
		return nil
	}
	payload, err := s.cache.Get(ctx, s.cacheKey(jobID))
	if err != nil || payload == nil {
		return nil
	}
	var results ProcessingResults
	if unmarshalErr := json.Unmarshal(payload, &results); unmarshalErr != nil {
		if s.logger != nil {
			s.logger.WarnContext(ctx, "failed to unmarshal cached rules results",
				"job_id", jobID,
				"error", unmarshalErr)
		}
		return nil
	}
	return &results
}

func (s *JobResultStore) getFromRepository(
	ctx context.Context,
	jobID string,
) (*ProcessingResults, error) {
	if s.repository == nil {
		return nil, ErrResultsNotFound
	}
	record, err := s.repository.GetByJobID(ctx, jobID)
	if err != nil {
		if s.isNotFoundErr(err) {
			return nil, ErrResultsNotFound
		}
		return nil, err
	}
	if record == nil || record.JobType != s.jobType {
		return nil, ErrResultsNotFound
	}
	var results ProcessingResults
	if unmarshalErr := json.Unmarshal(record.Result, &results); unmarshalErr != nil {
		return nil, fmt.Errorf("unmarshal persisted results: %w", unmarshalErr)
	}
	return &results, nil
}

func (s *JobResultStore) isNotFoundErr(err error) bool {
	if err == nil {
		return false
	}
	if s.isNotFound != nil && s.isNotFound(err) {
		return true
	}
	return errors.Is(err, ErrResultsNotFound)
}

var _ ResultStore = (*JobResultStore)(nil)

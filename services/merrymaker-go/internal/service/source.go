package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// DebugLogger is a minimal logger interface for optional debug logging.
type DebugLogger interface {
	Debug(msg string, keyvals ...any)
}

// sourceCache defines the minimal behavior required from a cache service.
type sourceCache interface {
	CacheSourceContent(ctx context.Context, sourceID string) error
	InvalidateSourceContent(ctx context.Context, sourceID string) error
}

type sourceResolvedCache interface {
	CacheResolvedSourceContent(ctx context.Context, source *model.Source) error
}

// SourceServiceOptions groups dependencies for SourceService.
type SourceServiceOptions struct {
	SourceRepo core.SourceRepository
	Jobs       core.JobRepository
	Cache      sourceCache // optional
	Logger     DebugLogger // optional
	SecretRepo core.SecretRepository
}

// SourceService orchestrates source CRUD with auto-enqueue for test sources.
type SourceService struct {
	src   core.SourceRepository
	jobs  core.JobRepository
	secs  core.SecretRepository
	cache sourceCache
	log   DebugLogger
}

// NewSourceService constructs a new SourceService.
func NewSourceService(opts SourceServiceOptions) *SourceService {
	return &SourceService{
		src:   opts.SourceRepo,
		jobs:  opts.Jobs,
		secs:  opts.SecretRepo,
		cache: opts.Cache,
		log:   opts.Logger,
	}
}

// ResolveScript returns the source script with any secret placeholders replaced using the configured secret repository.
func (s *SourceService) ResolveScript(ctx context.Context, source *model.Source) (string, error) {
	if source == nil {
		return "", errors.New("source is nil")
	}
	if len(source.Secrets) == 0 {
		return source.Value, nil
	}
	if s.secs == nil {
		return "", errors.New("secret repository not configured")
	}
	return core.ResolveSecretPlaceholders(ctx, s.secs, source.Secrets, source.Value)
}

// Create creates a source and auto-enqueues a test job when the source is a test source.
func (s *SourceService) Create(ctx context.Context, req *model.CreateSourceRequest) (*model.Source, error) {
	source, err := s.src.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create source: %w", err)
	}
	s.cacheResolvedSourceContent(ctx, source)
	if source.Test {
		if enqueueErr := s.enqueueTestJob(ctx, source, req.ClientToken); enqueueErr != nil {
			return nil, fmt.Errorf("enqueue test job: %w", enqueueErr)
		}
	}
	return source, nil
}

// Update updates a source and auto-enqueues a test job when the updated source is marked as test.
func (s *SourceService) Update(ctx context.Context, id string, req model.UpdateSourceRequest) (*model.Source, error) {
	source, err := s.src.Update(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("update source: %w", err)
	}
	s.refreshSourceContent(ctx, source)
	if source.Test {
		if enqueueErr := s.enqueueTestJob(ctx, source, ""); enqueueErr != nil {
			return nil, fmt.Errorf("enqueue test job: %w", enqueueErr)
		}
	}
	return source, nil
}

// List returns sources with pagination.
func (s *SourceService) List(ctx context.Context, limit, offset int) ([]*model.Source, error) {
	return s.src.List(ctx, limit, offset)
}

// Optional name-filtered listing if the repository supports it.
type sourceListByName interface {
	ListByNameContains(ctx context.Context, q string, limit, offset int) ([]*model.Source, error)
}

func (s *SourceService) ListByNameContains(ctx context.Context, q string, limit, offset int) ([]*model.Source, error) {
	if repo, ok := any(s.src).(sourceListByName); ok {
		return repo.ListByNameContains(ctx, q, limit, offset)
	}
	// Fallback to unfiltered list if unsupported
	return s.src.List(ctx, limit, offset)
}

// GetByID returns a source by id.
func (s *SourceService) GetByID(ctx context.Context, id string) (*model.Source, error) {
	return s.src.GetByID(ctx, id)
}

// GetByName returns a source by name.
func (s *SourceService) GetByName(ctx context.Context, name string) (*model.Source, error) {
	source, err := s.src.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get source by name: %w", err)
	}
	return source, nil
}

// Delete deletes a source by id.
func (s *SourceService) Delete(ctx context.Context, id string) (bool, error) {
	ok, err := s.src.Delete(ctx, id)
	if err != nil {
		return ok, err
	}
	if ok {
		s.invalidateSourceContent(ctx, id)
	}
	return ok, nil
}

// jobCounter is an optional repository extension for counting jobs per source.
type jobCounter interface {
	CountBySource(ctx context.Context, sourceID string, includeTests bool) (int, error)
	CountBrowserBySource(ctx context.Context, sourceID string, includeTests bool) (int, error)
}

// jobCountsBatcher is an optional repository extension for batched counts.
type jobCountsBatcher interface {
	CountAggregatesBySources(
		ctx context.Context,
		ids []string,
		includeTests bool,
	) (map[string]model.SourceJobCounts, error)
}

// CountJobsBySource returns total job count for a source. Returns 0 if unsupported.
func (s *SourceService) CountJobsBySource(ctx context.Context, sourceID string, includeTests bool) (int, error) {
	if jc, ok := any(s.jobs).(jobCounter); ok {
		return jc.CountBySource(ctx, sourceID, includeTests)
	}
	return 0, nil
}

// CountBrowserJobsBySource returns browser job count for a source. Returns 0 if unsupported.
func (s *SourceService) CountBrowserJobsBySource(ctx context.Context, sourceID string, includeTests bool) (int, error) {
	if jc, ok := any(s.jobs).(jobCounter); ok {
		return jc.CountBrowserBySource(ctx, sourceID, includeTests)
	}
	return 0, nil
}

// CountAggregatesBySources returns aggregated counts per source ID when supported; falls back otherwise.
func (s *SourceService) CountAggregatesBySources(
	ctx context.Context,
	ids []string,
	includeTests bool,
) (map[string]map[string]int, error) {
	if jc, ok := any(s.jobs).(jobCountsBatcher); ok {
		m, err := jc.CountAggregatesBySources(ctx, ids, includeTests)
		if err != nil {
			return nil, err
		}
		out := make(map[string]map[string]int, len(m))
		for id, c := range m {
			out[id] = map[string]int{"total": c.Total, "browser": c.Browser}
		}
		return out, nil
	}
	// Fallback: loop using single-source counters
	out := make(map[string]map[string]int, len(ids))
	if jc, ok := any(s.jobs).(jobCounter); ok {
		for _, id := range ids {
			t, err1 := jc.CountBySource(ctx, id, includeTests)
			b, err2 := jc.CountBrowserBySource(ctx, id, includeTests)
			if err1 != nil || err2 != nil {
				continue
			}
			out[id] = map[string]int{"total": t, "browser": b}
		}
	}
	return out, nil
}

func (s *SourceService) enqueueTestJob(ctx context.Context, source *model.Source, clientToken string) error {
	if s.jobs == nil || source == nil {
		return nil
	}
	script, err := s.ResolveScript(ctx, source)
	if err != nil {
		return fmt.Errorf("resolve source script: %w", err)
	}
	// Define a browser job payload contract that the worker understands.
	// Prefer explicit object form and include source_id for correlation
	payload := struct {
		SourceID string `json:"source_id"`
		Script   string `json:"script"`
	}{SourceID: source.ID, Script: script}
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal test job payload: %w", err)
	}
	id := source.ID // capture for pointer
	req := &model.CreateJobRequest{
		Type:       model.JobTypeBrowser,
		Payload:    json.RawMessage(b),
		Metadata:   buildTestJobMetadata(clientToken),
		SourceID:   &id,
		IsTest:     true,
		MaxRetries: 0, // Test jobs should fail immediately without retrying
	}
	if _, err = s.jobs.Create(ctx, req); err != nil {
		return fmt.Errorf("create test job: %w", err)
	}
	return nil
}

func buildTestJobMetadata(clientToken string) json.RawMessage {
	if clientToken == "" {
		return nil
	}
	mb, err := json.Marshal(map[string]string{"client_token": clientToken})
	if err != nil {
		return nil
	}
	return json.RawMessage(mb)
}

func (s *SourceService) cacheSourceContent(ctx context.Context, sourceID string) {
	if s.cache == nil {
		return
	}
	if sourceID == "" {
		return
	}
	// Best-effort cache population; failures are logged when a debug logger is configured.
	if err := s.cache.CacheSourceContent(ctx, sourceID); err != nil && s.log != nil {
		s.log.Debug("cache source content failed", "sourceID", sourceID, "err", err)
	}
}

func (s *SourceService) invalidateSourceContent(ctx context.Context, sourceID string) {
	if s.cache == nil || sourceID == "" {
		return
	}
	if err := s.cache.InvalidateSourceContent(ctx, sourceID); err != nil && s.log != nil {
		s.log.Debug("invalidate source content failed", "sourceID", sourceID, "err", err)
	}
}

func (s *SourceService) cacheResolvedSourceContent(ctx context.Context, source *model.Source) {
	if s.cache == nil || source == nil || source.ID == "" {
		return
	}
	if cacheSvc, ok := any(s.cache).(sourceResolvedCache); ok {
		if err := cacheSvc.CacheResolvedSourceContent(ctx, source); err != nil && s.log != nil {
			s.log.Debug("cache resolved source content failed", "sourceID", source.ID, "err", err)
		}
		return
	}
	s.cacheSourceContent(ctx, source.ID)
}

func (s *SourceService) refreshSourceContent(ctx context.Context, source *model.Source) {
	if source == nil {
		return
	}
	s.invalidateSourceContent(ctx, source.ID)
	s.cacheResolvedSourceContent(ctx, source)
}

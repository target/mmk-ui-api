package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// EventServiceConfig groups configuration parameters for EventService.
type EventServiceConfig struct {
	MaxBatch                 int     // Maximum batch size for bulk operations
	ThreatScoreProcessCutoff float64 // Cutoff for threat score processing
}

// DefaultEventServiceConfig returns sensible defaults for EventService configuration.
func DefaultEventServiceConfig() EventServiceConfig {
	return EventServiceConfig{
		MaxBatch:                 1000,
		ThreatScoreProcessCutoff: 0.7,
	}
}

// EventServiceOptions groups dependencies for EventService.
type EventServiceOptions struct {
	Repo   core.EventRepository // Required: event repository
	Config EventServiceConfig   // Required: service configuration
	Logger *slog.Logger         // Optional: structured logger
}

// EventService provides business logic for event operations.
//
// This service manages:
// - Bulk event insertion with processing flags
// - Event listing with pagination and filtering
// - Pagination normalization to prevent drift across layers
// - Optional repository extensions for advanced filtering.
type EventService struct {
	repo   core.EventRepository
	config EventServiceConfig
	logger *slog.Logger
}

// NewEventService constructs a new EventService.
func NewEventService(opts EventServiceOptions) (*EventService, error) {
	if opts.Repo == nil {
		return nil, errors.New("EventRepository is required")
	}
	if opts.Config.MaxBatch <= 0 {
		return nil, errors.New("MaxBatch must be positive")
	}
	if opts.Config.ThreatScoreProcessCutoff < 0 || opts.Config.ThreatScoreProcessCutoff > 1 {
		return nil, errors.New("ThreatScoreProcessCutoff must be between 0 and 1")
	}

	var logger *slog.Logger
	if opts.Logger != nil {
		logger = opts.Logger.With("component", "event_service")
		logger.Debug("EventService initialized",
			"max_batch", opts.Config.MaxBatch,
			"threat_score_cutoff", opts.Config.ThreatScoreProcessCutoff)
	}

	return &EventService{
		repo:   opts.Repo,
		config: opts.Config,
		logger: logger,
	}, nil
}

// MustNewEventService constructs a new EventService and panics on error.
// Use this when you're certain the options are valid (e.g., in main.go).
func MustNewEventService(opts EventServiceOptions) *EventService {
	svc, err := NewEventService(opts)
	if err != nil {
		//nolint:forbidigo // Must constructor fails fast during startup wiring when configuration is invalid
		panic(fmt.Sprintf("failed to create EventService: %v", err))
	}
	return svc
}

// BulkInsert inserts multiple events in bulk.
func (s *EventService) BulkInsert(
	ctx context.Context,
	req model.BulkEventRequest,
	process bool,
) (int, error) {
	count, err := s.repo.BulkInsert(ctx, req, process)
	if err != nil {
		return 0, fmt.Errorf("bulk insert events: %w", err)
	}

	if s.logger != nil {
		s.logger.DebugContext(ctx, "bulk inserted events", "count", count, "process", process)
	}

	return count, nil
}

// BulkInsertWithProcessingFlags inserts multiple events with individual processing flags.
func (s *EventService) BulkInsertWithProcessingFlags(
	ctx context.Context,
	req model.BulkEventRequest,
	shouldProcessMap map[int]bool,
) (int, error) {
	count, err := s.repo.BulkInsertWithProcessingFlags(ctx, req, shouldProcessMap)
	if err != nil {
		return 0, fmt.Errorf("bulk insert events with processing flags: %w", err)
	}

	if s.logger != nil {
		processCount := 0
		for _, shouldProcess := range shouldProcessMap {
			if shouldProcess {
				processCount++
			}
		}
		s.logger.DebugContext(ctx, "bulk inserted events with processing flags",
			"total_count", count, "process_count", processCount)
	}

	return count, nil
}

// ListByJob lists events for a given job with pagination.
func (s *EventService) ListByJob(
	ctx context.Context,
	opts model.EventListByJobOptions,
) (*model.EventListPage, error) {
	// Normalize pagination defaults here to avoid drift across layers
	normalizedOpts := opts
	if normalizedOpts.Limit <= 0 {
		normalizedOpts.Limit = 50
	}
	if normalizedOpts.Limit > 1000 {
		normalizedOpts.Limit = 1000
	}
	if normalizedOpts.Offset < 0 {
		normalizedOpts.Offset = 0
	}
	if normalizedOpts.CursorAfter != nil || normalizedOpts.CursorBefore != nil {
		// When keyset pagination is requested, ignore any supplied offset to avoid mixing strategies.
		normalizedOpts.Offset = 0
	}

	page, err := s.repo.ListByJob(ctx, normalizedOpts)
	if err != nil {
		return nil, fmt.Errorf("list events by job %s: %w", opts.JobID, err)
	}

	if s.logger != nil {
		s.logger.DebugContext(ctx, "listed events by job",
			"job_id", opts.JobID,
			"limit", normalizedOpts.Limit,
			"offset", normalizedOpts.Offset,
			"cursor_after", normalizedOpts.CursorAfter != nil,
			"cursor_before", normalizedOpts.CursorBefore != nil,
			"count", len(page.Events))
	}

	return page, nil
}

// ListWithFilters is deprecated. Use ListByJob with filter fields instead.
// Kept for backward compatibility - simply delegates to ListByJob.
func (s *EventService) ListWithFilters(
	ctx context.Context,
	opts model.EventListByJobOptions,
) (*model.EventListPage, error) {
	return s.ListByJob(ctx, opts)
}

// CountByJob returns the total count of events for a specific job with optional filters.
// Filters (EventType, Category, SearchQuery) are applied when non-nil/non-empty.
// This is useful for pagination to show accurate total event count.
func (s *EventService) CountByJob(
	ctx context.Context,
	opts model.EventListByJobOptions,
) (int, error) {
	count, err := s.repo.CountByJob(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("count events by job %s: %w", opts.JobID, err)
	}

	if s.logger != nil {
		s.logger.DebugContext(ctx, "counted events by job",
			"job_id", opts.JobID,
			"count", count)
	}

	return count, nil
}

// GetByIDs retrieves events by their IDs.
func (s *EventService) GetByIDs(ctx context.Context, ids []string) ([]*model.Event, error) {
	events, err := s.repo.GetByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("get events by ids: %w", err)
	}
	return events, nil
}

// GetConfig returns the current service configuration.
// This can be useful for other services that need to know the configuration.
func (s *EventService) GetConfig() EventServiceConfig {
	return s.config
}

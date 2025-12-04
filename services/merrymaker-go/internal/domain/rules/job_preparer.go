package rules

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// JobPreparationOptions configure the behavior of the default job preparation service.
type JobPreparationOptions struct {
	Sites  core.SiteRepository
	Events core.EventRepository
	Logger *slog.Logger
}

// JobPreparationService coordinates lookup of job metadata required before pipeline execution.
//
// It satisfies both the AlertResolver and EventFetcher interfaces so the rules orchestrator
// can rely on a single collaborator for alert mode resolution and event hydration when
// custom implementations are not supplied.
type JobPreparationService struct {
	sites  core.SiteRepository
	events core.EventRepository
	logger *slog.Logger
}

// NewJobPreparationService constructs a JobPreparationService with the supplied dependencies.
func NewJobPreparationService(opts JobPreparationOptions) *JobPreparationService {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}
	return &JobPreparationService{
		sites:  opts.Sites,
		events: opts.Events,
		logger: logger,
	}
}

// Resolve determines the effective alert mode for the provided job context.
func (s *JobPreparationService) Resolve(
	ctx context.Context,
	params AlertResolutionParams,
) model.SiteAlertMode {
	if params.SiteID == "" || s == nil {
		return model.SiteAlertModeActive
	}
	if s.sites == nil {
		return model.SiteAlertModeActive
	}

	site, err := s.sites.GetByID(ctx, params.SiteID)
	if err != nil {
		s.logger.WarnContext(ctx, "failed to load site for rules job",
			"job_id", params.JobID,
			"site_id", params.SiteID,
			"error", err)
		return model.SiteAlertModeActive
	}
	if site != nil {
		return normalizeAlertMode(site.AlertMode)
	}
	return model.SiteAlertModeActive
}

// Fetch retrieves events for the provided job context.
func (s *JobPreparationService) Fetch(
	ctx context.Context,
	params EventFetchParams,
) ([]*model.Event, error) {
	if s == nil {
		return nil, errors.New("job preparation service is nil")
	}
	if len(params.EventIDs) == 0 {
		return nil, nil
	}
	if s.events == nil {
		return nil, errors.New("event repository is not configured")
	}

	events, err := s.events.GetByIDs(ctx, params.EventIDs)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to fetch events for rules job",
			"job_id", params.JobID,
			"error", err)
		return nil, fmt.Errorf("fetch events: %w", err)
	}
	return events, nil
}

var (
	_ AlertResolver = (*JobPreparationService)(nil)
	_ EventFetcher  = (*JobPreparationService)(nil)
)

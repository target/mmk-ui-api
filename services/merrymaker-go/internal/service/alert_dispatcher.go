package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// AlertSinkScheduler is an interface for scheduling alert jobs.
type AlertSinkScheduler interface {
	ScheduleAlert(
		ctx context.Context,
		sink *model.HTTPAlertSink,
		eventPayload json.RawMessage,
	) (*model.Job, error)
}

// AlertDispatchService dispatches alerts to HTTP alert sinks.
type AlertDispatchService struct {
	sinks     core.HTTPAlertSinkRepository
	sites     core.SiteRepository
	alertSink AlertSinkScheduler
	baseURL   string
	logger    *slog.Logger
}

// AlertDispatchServiceOptions configures the alert dispatch service.
type AlertDispatchServiceOptions struct {
	Sinks     core.HTTPAlertSinkRepository
	Sites     core.SiteRepository
	AlertSink AlertSinkScheduler
	BaseURL   string
	Logger    *slog.Logger
}

// NewAlertDispatchService creates a new alert dispatch service.
// If BaseURL is empty, it defaults to "http://localhost:8080" to ensure
// a consistent default with the HTTPConfig.
func NewAlertDispatchService(opts AlertDispatchServiceOptions) *AlertDispatchService {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	baseURL := opts.BaseURL
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}

	return &AlertDispatchService{
		sinks:     opts.Sinks,
		sites:     opts.Sites,
		alertSink: opts.AlertSink,
		baseURL:   baseURL,
		logger:    logger,
	}
}

var (
	errSiteRepoNotConfigured = errors.New(
		"alert dispatch: site repository not configured",
	)
	errSiteSinkNotConfigured           = errors.New("alert dispatch: site sink not configured")
	errSiteSinkMissing                 = errors.New("alert dispatch: site sink missing")
	errAlertSinkSchedulerNotConfigured = errors.New(
		"alert dispatch: alert sink scheduler not configured",
	)
)

// Dispatch sends an alert to all configured HTTP alert sinks.
func (s *AlertDispatchService) Dispatch(ctx context.Context, alert *model.Alert) error {
	params, ok, err := s.prepareDispatchParams(ctx, alert)
	if err != nil || !ok {
		return err
	}

	successCount, lastErr := s.dispatchToSinks(params)

	if successCount == 0 {
		s.logger.ErrorContext(ctx, "failed to dispatch alert to any sinks", "alert_id", alert.ID)
		return fmt.Errorf("alert dispatch: all sink schedules failed: %w", lastErr)
	}

	s.logger.InfoContext(ctx, "dispatched alert to HTTP alert sinks",
		"alert_id", alert.ID,
		"sinks_total", len(params.sinks),
		"sinks_success", successCount)

	return nil
}

func (s *AlertDispatchService) prepareDispatchParams(
	ctx context.Context,
	alert *model.Alert,
) (dispatchParams, bool, error) {
	if s.alertSink == nil {
		return dispatchParams{}, false, errAlertSinkSchedulerNotConfigured
	}

	site, targetSink, err := s.resolveSiteAndSink(ctx, alert.SiteID)
	if err != nil {
		if errors.Is(err, errSiteSinkNotConfigured) {
			s.logger.DebugContext(ctx, "no HTTP alert sink configured for site, skipping dispatch",
				"alert_id", alert.ID,
				"site_id", alert.SiteID)
			return dispatchParams{}, false, nil
		}
		if errors.Is(err, errSiteSinkMissing) {
			s.logger.WarnContext(ctx, "site references missing HTTP alert sink",
				"alert_id", alert.ID,
				"site_id", alert.SiteID,
				"error", err)
			return dispatchParams{}, false, nil
		}
		return dispatchParams{}, false, err
	}

	if site != nil && site.AlertMode == model.SiteAlertModeMuted {
		s.logger.InfoContext(ctx, "site alert mode muted; skipping alert dispatch",
			"alert_id", alert.ID,
			"site_id", alert.SiteID)
		return dispatchParams{}, false, nil
	}

	payload, err := s.buildEnrichedPayload(ctx, alert, site)
	if err != nil {
		return dispatchParams{}, false, err
	}

	return dispatchParams{
		ctx:     ctx,
		alert:   alert,
		sinks:   []*model.HTTPAlertSink{targetSink},
		payload: payload,
	}, true, nil
}

func (s *AlertDispatchService) buildEnrichedPayload(
	ctx context.Context,
	alert *model.Alert,
	site *model.Site,
) (json.RawMessage, error) {
	alertJSON, err := json.Marshal(alert)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to marshal alert data", "alert_id", alert.ID, "error", err)
		return nil, fmt.Errorf("alert dispatch: marshal alert: %w", err)
	}

	siteName := ""
	if site != nil {
		siteName = site.Name
	}

	enrichedPayload := AlertPayload{
		Alert:    alertJSON,
		SiteName: siteName,
		AlertURL: s.buildAlertURL(alert.ID),
	}

	payload, err := json.Marshal(enrichedPayload)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to marshal enriched payload", "alert_id", alert.ID, "error", err)
		return nil, fmt.Errorf("alert dispatch: marshal enriched payload: %w", err)
	}

	return payload, nil
}

// buildAlertURL constructs the full URL to view an alert in the UI.
// baseURL is guaranteed to be non-empty due to normalization in NewAlertDispatchService.
func (s *AlertDispatchService) buildAlertURL(alertID string) string {
	baseURL := strings.TrimRight(s.baseURL, "/")
	return fmt.Sprintf("%s/alerts/%s", baseURL, alertID)
}

// dispatchParams groups parameters for dispatchToSinks to maintain ≤3 param constraint.
type dispatchParams struct {
	ctx     context.Context
	alert   *model.Alert
	sinks   []*model.HTTPAlertSink
	payload json.RawMessage
}

// dispatchToSinks schedules alert jobs for each sink and returns success count and last error.
func (s *AlertDispatchService) dispatchToSinks(p dispatchParams) (int, error) {
	successCount := 0
	var lastErr error
	for _, sink := range p.sinks {
		job, err := s.alertSink.ScheduleAlert(p.ctx, sink, p.payload)
		if err != nil {
			lastErr = err
			s.logger.ErrorContext(p.ctx, "failed to schedule alert job",
				"alert_id", p.alert.ID,
				"sink_id", sink.ID,
				"sink_name", sink.Name,
				"error", err)
			continue
		}

		s.logger.InfoContext(p.ctx, "scheduled alert job",
			"alert_id", p.alert.ID,
			"sink_id", sink.ID,
			"sink_name", sink.Name,
			"job_id", job.ID)
		successCount++
	}
	return successCount, lastErr
}

func (s *AlertDispatchService) resolveSiteAndSink(
	ctx context.Context,
	siteID string,
) (*model.Site, *model.HTTPAlertSink, error) {
	if s.sites == nil {
		return nil, nil, fmt.Errorf(
			"alert dispatch: site repository not configured: %w",
			errSiteRepoNotConfigured,
		)
	}

	site, err := s.sites.GetByID(ctx, siteID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to load site for alert dispatch", "site_id", siteID, "error", err)
		return nil, nil, fmt.Errorf("alert dispatch: get site: %w", err)
	}

	if site == nil || site.HTTPAlertSinkID == nil {
		return site, nil, errSiteSinkNotConfigured
	}

	sinkID := strings.TrimSpace(*site.HTTPAlertSinkID)
	if sinkID == "" {
		return site, nil, errSiteSinkNotConfigured
	}

	sink, err := s.sinks.GetByID(ctx, sinkID)
	if err != nil {
		s.logger.ErrorContext(
			ctx,
			"failed to load HTTP alert sink",
			"sink_id",
			sinkID,
			"site_id",
			siteID,
			"error",
			err,
		)
		return site, nil, fmt.Errorf("alert dispatch: get sink: %w", err)
	}

	if sink == nil {
		return site, nil, fmt.Errorf("%w: sink_id=%s", errSiteSinkMissing, sinkID)
	}

	return site, sink, nil
}

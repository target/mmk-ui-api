package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// AlertDispatcher is an interface for dispatching alerts to HTTP alert sinks.
// This allows the AlertService to trigger async dispatch without tight coupling.
type AlertDispatcher interface {
	Dispatch(ctx context.Context, alert *model.Alert) error
}

// AlertServiceOptions groups dependencies for AlertService.
//
// All fields are required except Logger which is optional.
// Dispatcher is technically optional but recommended for production use.
type AlertServiceOptions struct {
	Repo       core.AlertRepository // Required: alert repository
	Sites      core.SiteRepository  // Optional: load site context for delivery decisions
	Dispatcher AlertDispatcher      // Optional: dispatches alerts to HTTP alert sinks
	Logger     *slog.Logger         // Optional: structured logger
}

// AlertService provides business logic for alert operations.
//
// RESPONSIBILITIES:
// - CRUD operations for alerts
// - Async dispatch to HTTP alert sinks (non-blocking)
// - Alert resolution workflow
// - Alert statistics
//
// ORCHESTRATION:
//   - When an alert is created, it's automatically dispatched to all configured
//     HTTP alert sinks asynchronously (fire-and-forget with error logging)
type AlertService struct {
	repo       core.AlertRepository
	dispatcher AlertDispatcher
	sites      core.SiteRepository
	logger     *slog.Logger
}

// NewAlertService constructs a new AlertService.
//
// Returns an error if Repo is nil. Dispatcher and Logger are optional.
func NewAlertService(opts AlertServiceOptions) (*AlertService, error) {
	if opts.Repo == nil {
		return nil, errors.New("AlertRepository is required")
	}

	if opts.Logger != nil {
		opts.Logger.Info("AlertService initialized",
			"has_dispatcher", opts.Dispatcher != nil)
	}

	return &AlertService{
		repo:       opts.Repo,
		dispatcher: opts.Dispatcher,
		sites:      opts.Sites,
		logger:     opts.Logger,
	}, nil
}

// MustNewAlertService constructs a new AlertService and panics on error.
//
// This is a convenience wrapper around NewAlertService for use in main.go
// and other initialization code where errors should be fatal.
func MustNewAlertService(opts AlertServiceOptions) *AlertService {
	svc, err := NewAlertService(opts)
	if err != nil {
		panic(err) //nolint:forbidigo // Must constructor fails fast when dependencies are invalid during startup
	}
	return svc
}

// Create creates a new alert with the given request parameters.
//
// If a dispatcher is configured, the alert is automatically dispatched to all
// configured HTTP alert sinks asynchronously (non-blocking). Dispatch errors
// are logged but do not fail the create operation.
func (s *AlertService) Create(
	ctx context.Context,
	req *model.CreateAlertRequest,
) (*model.Alert, error) {
	if req == nil {
		return nil, errors.New("create alert request is required")
	}

	siteMode := model.SiteAlertModeActive
	if s.sites != nil && strings.TrimSpace(req.SiteID) != "" {
		site, err := s.sites.GetByID(ctx, req.SiteID)
		if err != nil && s.logger != nil {
			s.logger.WarnContext(ctx, "failed to load site for alert creation",
				"site_id", req.SiteID,
				"error", err)
		} else if site != nil && site.AlertMode.Valid() {
			siteMode = site.AlertMode
		}
	}

	deliveryStatus := model.AlertDeliveryStatusPending
	if siteMode == model.SiteAlertModeMuted {
		deliveryStatus = model.AlertDeliveryStatusMuted
	}
	req.DeliveryStatus = deliveryStatus

	// Create alert in database
	alert, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create alert: %w", err)
	}
	alert.DeliveryStatus = deliveryStatus

	// Log alert creation
	if s.logger != nil {
		s.logger.InfoContext(ctx, "alert created",
			"alert_id", alert.ID,
			"site_id", alert.SiteID,
			"rule_type", alert.RuleType,
			"severity", alert.Severity,
			"alert_mode", siteMode,
			"delivery_status", alert.DeliveryStatus)
	}

	if deliveryStatus == model.AlertDeliveryStatusMuted {
		if s.logger != nil {
			s.logger.InfoContext(ctx, "alert delivery muted; skipping dispatch",
				"alert_id", alert.ID,
				"site_id", alert.SiteID)
		}
		return alert, nil
	}

	// Dispatch to HTTP alert sinks asynchronously (non-blocking)
	if s.dispatcher == nil {
		return alert, nil
	}

	// Copy alert value to avoid potential data races if caller mutates the pointer
	alertCopy := *alert
	s.dispatchAlertAsync(ctx, alertCopy)

	return alert, nil
}

// GetByID retrieves an alert by its ID.
func (s *AlertService) GetByID(ctx context.Context, id string) (*model.Alert, error) {
	alert, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get alert by id: %w", err)
	}
	return alert, nil
}

// List retrieves a list of alerts with the given options.
func (s *AlertService) List(
	ctx context.Context,
	opts *model.AlertListOptions,
) ([]*model.Alert, error) {
	alerts, err := s.repo.List(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list alerts: %w", err)
	}
	return alerts, nil
}

// ListWithSiteNames retrieves a list of alerts with site names using a JOIN query.
// This method eliminates N+1 queries by fetching site names in a single query.
func (s *AlertService) ListWithSiteNames(
	ctx context.Context,
	opts *model.AlertListOptions,
) ([]*model.AlertWithSiteName, error) {
	alerts, err := s.repo.ListWithSiteNames(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list alerts with site names: %w", err)
	}
	return alerts, nil
}

// ListWithSiteNamesAndCount retrieves alerts with site names and total count in a single query.
// This is more efficient than calling ListWithSiteNames and Count separately.
func (s *AlertService) ListWithSiteNamesAndCount(
	ctx context.Context,
	opts *model.AlertListOptions,
) (*model.AlertListResult, error) {
	result, err := s.repo.ListWithSiteNamesAndCount(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("list alerts with site names and count: %w", err)
	}
	return result, nil
}

// Count returns the total number of alerts matching the given filter options.
// This is useful for pagination to show accurate total alert count.
func (s *AlertService) Count(ctx context.Context, opts *model.AlertListOptions) (int, error) {
	count, err := s.repo.Count(ctx, opts)
	if err != nil {
		return 0, fmt.Errorf("count alerts: %w", err)
	}
	return count, nil
}

// Delete deletes an alert by its ID.
//
// Returns true if the alert was deleted, false if it didn't exist.
func (s *AlertService) Delete(ctx context.Context, id string) (bool, error) {
	deleted, err := s.repo.Delete(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete alert: %w", err)
	}

	if deleted && s.logger != nil {
		s.logger.InfoContext(ctx, "alert deleted", "alert_id", id)
	}

	return deleted, nil
}

func (s *AlertService) dispatchAlertAsync(ctx context.Context, alert model.Alert) {
	go func(a model.Alert) {
		defer s.recoverDispatchPanic(a)

		// Preserve request-scoped values (logging, tracing) while ignoring cancellation
		// This ensures the dispatch completes even if the original request is cancelled.
		dispatchCtx := context.WithoutCancel(ctx)
		if err := s.dispatcher.Dispatch(dispatchCtx, &a); err != nil {
			s.logDispatchError(a, err)
		}
	}(alert)
}

func (s *AlertService) recoverDispatchPanic(alert model.Alert) {
	if r := recover(); r != nil && s.logger != nil {
		s.logger.Error("panic in alert dispatch",
			"alert_id", alert.ID,
			"panic", r)
	}
}

func (s *AlertService) logDispatchError(alert model.Alert, err error) {
	if s.logger == nil {
		return
	}

	s.logger.Error("alert dispatch failed",
		"alert_id", alert.ID,
		"error", err)
}

// Stats retrieves alert statistics, optionally filtered by site ID.
//
// If siteID is nil, returns statistics for all alerts across all sites.
// If siteID is provided, returns statistics for that specific site only.
func (s *AlertService) Stats(ctx context.Context, siteID *string) (*model.AlertStats, error) {
	stats, err := s.repo.Stats(ctx, siteID)
	if err != nil {
		return nil, fmt.Errorf("get alert stats: %w", err)
	}
	return stats, nil
}

// Resolve marks an alert as resolved.
//
// This updates the alert's resolved_at timestamp, resolved_by user, and returns the updated alert.
func (s *AlertService) Resolve(
	ctx context.Context,
	params core.ResolveAlertParams,
) (*model.Alert, error) {
	alert, err := s.repo.Resolve(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("resolve alert: %w", err)
	}

	if s.logger != nil {
		s.logger.InfoContext(ctx, "alert resolved",
			"alert_id", params.ID,
			"resolved_by", params.ResolvedBy,
			"resolved_at", alert.ResolvedAt)
	}

	return alert, nil
}

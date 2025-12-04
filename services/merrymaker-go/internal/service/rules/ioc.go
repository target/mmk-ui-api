package rules

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// IOCEvaluator checks hosts (domains/IPs) against global IOCs and emits alerts with alert-once-per-scope semantics.
type IOCEvaluator struct {
	Caches   Caches
	Alerter  AlertCreator
	AlertTTL time.Duration // TTL for alert-once deduplication; zero disables
}

// IOCRequest groups inputs for IOC evaluation (≤3 parameters rule).
type IOCRequest struct {
	Scope      ScopeKey
	Host       string // domain or IP; normalized internally
	SiteID     string // duplicate of Scope.SiteID for convenience when building alerts
	JobID      string // job ID if alert originated from a browser job
	RequestURL string
	PageURL    string
	Referrer   string
	UserAgent  string
	EventID    string
}

// Evaluate checks if a host matches any global IOC and creates an alert if found.
// Returns true if an alert was created.
func (e *IOCEvaluator) Evaluate(ctx context.Context, req IOCRequest) (bool, error) {
	if err := req.Scope.Validate(); err != nil {
		return false, err
	}

	host := normalizeHost(req.Host)
	if host == "" {
		return false, nil
	}

	// Check if host matches any global IOC
	ioc, err := e.lookupIOC(ctx, host)
	if err != nil {
		return false, err
	}
	if ioc == nil {
		return false, nil // Not an IOC
	}

	// Check alert-once-per-scope deduplication
	dup, err := e.shouldDedupe(ctx, IOCDedupeParams{
		Scope: req.Scope,
		IOCID: ioc.ID,
	})
	if err != nil {
		return false, err
	}
	if dup {
		return false, nil // Already alerted for this IOC in this scope
	}

	// Create alert for IOC match
	if e.Alerter == nil {
		return false, nil
	}

	if alertErr := e.createAlert(ctx, IOCCreateAlertParams{
		Req: req,
		IOC: ioc,
	}); alertErr != nil {
		return false, alertErr
	}

	return true, nil
}

// normalizeHost normalizes host input to lowercase and trimmed.
func normalizeHost(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

// lookupIOC checks if a host matches any global IOC, handling cache configuration and errors.
func (e *IOCEvaluator) lookupIOC(ctx context.Context, host string) (*model.IOC, error) {
	// Guard against nil IOC cache
	if e.Caches.IOCs == nil {
		return nil, errors.New("IOC cache not configured")
	}

	ioc, err := e.Caches.IOCs.LookupHost(ctx, host)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil //nolint:nilnil // Not found is valid for cache lookups
		}
		return nil, err
	}

	return ioc, nil
}

type IOCDedupeParams struct {
	Scope ScopeKey
	IOCID string
}

// shouldDedupe checks if we should skip alerting due to alert-once-per-scope logic.
func (e *IOCEvaluator) shouldDedupe(
	ctx context.Context,
	params IOCDedupeParams,
) (bool, error) {
	if e.AlertTTL <= 0 || e.Caches.AlertOnce == nil {
		return false, nil
	}

	return e.Caches.AlertOnce.Seen(ctx, AlertSeenRequest{
		Scope:     params.Scope,
		DedupeKey: "ioc:" + params.IOCID,
		TTL:       e.AlertTTL,
	})
}

type IOCCreateAlertParams struct {
	Req IOCRequest
	IOC *model.IOC
}

// createAlert creates an alert for a global IOC match.
func (e *IOCEvaluator) createAlert(ctx context.Context, params IOCCreateAlertParams) error {
	// Default to high severity for IOC matches
	alertSeverity := string(model.AlertSeverityHigh)

	title := fmt.Sprintf("IOC detected: %s", params.IOC.Type)
	desc := buildAlertDescription(params.Req.Host, params.IOC)

	// Build event context with IOC details
	ctxObj := buildEventContext(params.Req, params.IOC)
	if params.Req.JobID != "" {
		ctxObj["job_id"] = params.Req.JobID
	}

	ctxJSON, err := json.Marshal(ctxObj)
	if err != nil {
		return fmt.Errorf("marshal alert context: %w", err)
	}

	_, err = e.Alerter.Create(ctx, &model.CreateAlertRequest{
		SiteID:       params.Req.SiteID,
		RuleID:       nil,                            // Could be enhanced to link to a specific IOC rule
		RuleType:     string(model.AlertRuleTypeIOC), // Reuse existing type for now
		Severity:     alertSeverity,
		Title:        title,
		Description:  desc,
		EventContext: ctxJSON,
	})
	if err != nil {
		return fmt.Errorf("create IOC alert: %w", err)
	}

	return nil
}

// buildAlertDescription constructs a human-readable alert description.
func buildAlertDescription(host string, ioc *model.IOC) string {
	desc := fmt.Sprintf(
		"Known IOC detected: %s (type: %s, value: %s, scope: global)",
		host,
		ioc.Type,
		ioc.Value,
	)
	if ioc.Description != nil && *ioc.Description != "" {
		desc += ", description: " + *ioc.Description
	}
	return desc
}

// buildEventContext constructs the event context object for the alert.
func buildEventContext(req IOCRequest, ioc *model.IOC) map[string]interface{} {
	ctx := map[string]interface{}{
		"host":      req.Host,
		"scope":     req.Scope.Scope,
		"site_id":   req.SiteID,
		"ioc_id":    ioc.ID,
		"ioc_type":  string(ioc.Type),
		"ioc_value": ioc.Value,
	}

	if req.EventID != "" {
		ctx["event_id"] = req.EventID
	}

	if req.RequestURL != "" {
		ctx["request_url"] = req.RequestURL
	}
	if req.PageURL != "" {
		ctx["page_url"] = req.PageURL
	}
	if req.Referrer != "" {
		ctx["referrer"] = req.Referrer
	}
	if req.UserAgent != "" {
		ctx["user_agent"] = req.UserAgent
	}

	if ioc.Description != nil && *ioc.Description != "" {
		ctx["ioc_description"] = *ioc.Description
	}

	return ctx
}

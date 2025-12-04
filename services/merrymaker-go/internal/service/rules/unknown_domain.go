package rules

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// AlertCreator is a small interface for creating alerts (adapter over core.AlertService).
// Kept local to avoid expanding public APIs; satisfies ≤3 params rule via request structs.
type AlertCreator interface {
	Create(ctx context.Context, req *model.CreateAlertRequest) (*model.Alert, error)
}

// AllowlistChecker optionally preempts alerts for allowed domains in a given scope.
type AllowlistChecker interface {
	Allowed(ctx context.Context, scope ScopeKey, domain string) bool
}

// UnknownDomainEvaluator determines if a domain is unknown for a scope and emits an alert once.
type UnknownDomainEvaluator struct {
	Caches    Caches
	Alerter   AlertCreator
	Allowlist AllowlistChecker // optional
	AlertTTL  time.Duration    // optional extra dedupe via AlertOnce; zero disables
	Logger    *slog.Logger     // optional; used for debug logging
}

// UnknownDomainDecision represents the outcome of evaluating a domain.
type UnknownDomainDecision string

const (
	// UnknownDomainDecisionAlertCreated indicates an alert would be or was created.
	UnknownDomainDecisionAlertCreated UnknownDomainDecision = "alert_created"
	// UnknownDomainDecisionAllowlisted indicates the domain was allowlisted.
	UnknownDomainDecisionAllowlisted UnknownDomainDecision = "allowlisted"
	// UnknownDomainDecisionAlreadySeen indicates the domain was already seen.
	UnknownDomainDecisionAlreadySeen UnknownDomainDecision = "already_seen"
	// UnknownDomainDecisionDeduped indicates the domain was suppressed by alert-once dedupe.
	UnknownDomainDecisionDeduped UnknownDomainDecision = "deduped"
	// UnknownDomainDecisionNormalizationFailed indicates the domain could not be normalized.
	UnknownDomainDecisionNormalizationFailed UnknownDomainDecision = "normalization_failed"
	// UnknownDomainDecisionError indicates an evaluation error occurred.
	UnknownDomainDecisionError UnknownDomainDecision = "error"
)

// UnknownDomainDecisionRecorder records evaluation decisions for observability.
type UnknownDomainDecisionRecorder interface {
	RecordUnknownDomainDecision(decision UnknownDomainDecision, domain string)
}

// UnknownDomainRequest groups inputs (≤3 parameters rule).
type UnknownDomainRequest struct {
	Scope      ScopeKey
	Domain     string // expected raw; normalized to lower-case internally
	SiteID     string // duplicate of Scope.SiteID for convenience when building alerts
	JobID      string // job ID if alert originated from a browser job
	RequestURL string
	PageURL    string
	Referrer   string
	UserAgent  string
	EventID    string

	Recorder UnknownDomainDecisionRecorder
}

func (req UnknownDomainRequest) record(decision UnknownDomainDecision, domain string) {
	if req.Recorder == nil {
		return
	}
	req.Recorder.RecordUnknownDomainDecision(decision, domain)
}

// Evaluate checks and records an unknown domain. It returns true if an alert was created.
func (e *UnknownDomainEvaluator) Evaluate(
	ctx context.Context,
	req UnknownDomainRequest,
) (bool, error) {
	if err := req.Scope.Validate(); err != nil {
		return false, err
	}
	domain := e.normalizeDomain(req.Domain)
	if domain == "" {
		req.record(UnknownDomainDecisionNormalizationFailed, strings.TrimSpace(req.Domain))
		e.logNormalizationFailure(ctx, req)
		return false, nil
	}

	if e.allowlisted(ctx, UnknownDomainAllowlistedCheckParams{
		Scope:  req.Scope,
		Domain: domain,
	}) {
		e.handleAllowlisted(ctx, UnknownDomainAllowlistedParams{
			Req:    req,
			Domain: domain,
		})
		return false, nil
	}

	seen, err := e.Caches.Seen.Exists(ctx, SeenKey{Scope: req.Scope, Domain: domain})
	if err != nil {
		e.logDecision(ctx, UnknownDomainLogParams{
			Reason: "domain_exists_check_error",
			Req:    req,
			Domain: domain,
		})
		req.record(UnknownDomainDecisionError, domain)
		return false, err
	}
	if seen {
		e.handleAlreadySeen(ctx, req, domain)
		return false, nil
	}

	dup, err := e.shouldDedupe(ctx, UnknownDomainDedupeParams{
		Scope:  req.Scope,
		Domain: domain,
	})
	if err != nil {
		e.logDecision(ctx, UnknownDomainLogParams{
			Reason: "domain_dedupe_check_error",
			Req:    req,
			Domain: domain,
		})
		req.record(UnknownDomainDecisionError, domain)
		return false, err
	}
	if dup {
		e.handleDeduped(ctx, req, domain)
		return false, nil
	}

	return e.createAndRecordAlert(ctx, req, domain)
}

// UnknownDomainLogParams groups parameters for logDecision and related functions.
type UnknownDomainLogParams struct {
	Reason string
	Req    UnknownDomainRequest
	Domain string
}

// logDecision logs the decision reason for why a domain did not trigger an alert.
func (e *UnknownDomainEvaluator) logDecision(
	ctx context.Context,
	params UnknownDomainLogParams,
) {
	if e.Logger == nil {
		return
	}
	e.Logger.DebugContext(ctx, "unknown domain evaluation decision",
		"reason", params.Reason,
		"domain", params.Domain,
		"site_id", params.Req.SiteID,
		"scope", params.Req.Scope)
}

func (e *UnknownDomainEvaluator) logNormalizationFailure(
	ctx context.Context,
	req UnknownDomainRequest,
) {
	// Note: req.record() is already called in Evaluate() before this function is invoked.
	// We only log here to avoid duplicate recording.
	e.logDecision(ctx, UnknownDomainLogParams{
		Reason: "domain_normalization_failed",
		Req:    req,
		Domain: "",
	})
	e.info(
		"alert suppressed: domain normalization failed",
		"raw_domain",
		req.Domain,
		"site_id",
		req.SiteID,
		"scope",
		req.Scope.Scope,
	)
}

type UnknownDomainAllowlistedParams struct {
	Req    UnknownDomainRequest
	Domain string
}

func (e *UnknownDomainEvaluator) handleAllowlisted(
	ctx context.Context,
	params UnknownDomainAllowlistedParams,
) {
	e.logDecision(ctx, UnknownDomainLogParams{
		Reason: "domain_allowlisted",
		Req:    params.Req,
		Domain: params.Domain,
	})
	e.info(
		"alert suppressed: domain allowlisted",
		"domain",
		params.Domain,
		"site_id",
		params.Req.SiteID,
		"scope",
		params.Req.Scope.Scope,
	)
	params.Req.record(UnknownDomainDecisionAllowlisted, params.Domain)
	if err := e.Caches.Seen.Record(ctx, SeenKey{Scope: params.Req.Scope, Domain: params.Domain}); err != nil {
		e.warn(
			"failed to record allowlisted domain as seen",
			"domain", params.Domain,
			"site_id", params.Req.SiteID,
			"scope", params.Req.Scope.Scope,
			"error", err,
		)
	}
}

func (e *UnknownDomainEvaluator) handleAlreadySeen(
	ctx context.Context,
	req UnknownDomainRequest,
	domain string,
) {
	e.logDecision(ctx, UnknownDomainLogParams{
		Reason: "domain_already_seen",
		Req:    req,
		Domain: domain,
	})
	e.info(
		"alert suppressed: domain already seen",
		"domain",
		domain,
		"site_id",
		req.SiteID,
		"scope",
		req.Scope.Scope,
	)
	req.record(UnknownDomainDecisionAlreadySeen, domain)
}

func (e *UnknownDomainEvaluator) handleDeduped(
	ctx context.Context,
	req UnknownDomainRequest,
	domain string,
) {
	e.logDecision(ctx, UnknownDomainLogParams{
		Reason: "domain_deduped",
		Req:    req,
		Domain: domain,
	})
	req.record(UnknownDomainDecisionDeduped, domain)
}

func (e *UnknownDomainEvaluator) createAndRecordAlert(
	ctx context.Context,
	req UnknownDomainRequest,
	domain string,
) (bool, error) {
	if ok := e.checkAlerterConfigured(ctx, req, domain); !ok {
		return false, nil
	}
	if err := e.tryCreateAlert(ctx, req, domain); err != nil {
		return false, err
	}
	if err := e.recordDomainSeen(ctx, req, domain); err != nil {
		return false, err
	}
	e.logDecision(ctx, UnknownDomainLogParams{
		Reason: "alert_created",
		Req:    req,
		Domain: domain,
	})
	e.info("alert created", "domain", domain, "site_id", req.SiteID, "scope", req.Scope.Scope)
	req.record(UnknownDomainDecisionAlertCreated, domain)
	return true, nil
}

func (e *UnknownDomainEvaluator) checkAlerterConfigured(
	ctx context.Context,
	req UnknownDomainRequest,
	domain string,
) bool {
	if e.Alerter == nil {
		e.logDecision(ctx, UnknownDomainLogParams{
			Reason: "no_alerter_configured",
			Req:    req,
			Domain: domain,
		})
		e.warn(
			"alert not created: no alerter configured",
			"domain",
			domain,
			"site_id",
			req.SiteID,
			"scope",
			req.Scope.Scope,
		)
		return false
	}
	return true
}

func (e *UnknownDomainEvaluator) tryCreateAlert(
	ctx context.Context,
	req UnknownDomainRequest,
	domain string,
) error {
	if err := e.createAlert(ctx, UnknownDomainCreateAlertParams{
		Scope:      req.Scope,
		Domain:     domain,
		JobID:      req.JobID,
		RequestURL: req.RequestURL,
		PageURL:    req.PageURL,
		Referrer:   req.Referrer,
		UserAgent:  req.UserAgent,
		EventID:    req.EventID,
	}); err != nil {
		e.logDecision(ctx, UnknownDomainLogParams{
			Reason: "alert_creation_failed",
			Req:    req,
			Domain: domain,
		})
		req.record(UnknownDomainDecisionError, domain)
		e.error(
			"failed to create alert",
			"domain",
			domain,
			"site_id",
			req.SiteID,
			"scope",
			req.Scope.Scope,
			"error",
			err,
		)
		return err
	}
	return nil
}

func (e *UnknownDomainEvaluator) recordDomainSeen(
	ctx context.Context,
	req UnknownDomainRequest,
	domain string,
) error {
	if err := e.Caches.Seen.Record(ctx, SeenKey{Scope: req.Scope, Domain: domain}); err != nil {
		e.logDecision(ctx, UnknownDomainLogParams{
			Reason: "domain_record_failed",
			Req:    req,
			Domain: domain,
		})
		req.record(UnknownDomainDecisionError, domain)
		e.error(
			"failed to record domain as seen",
			"domain",
			domain,
			"site_id",
			req.SiteID,
			"scope",
			req.Scope.Scope,
			"error",
			err,
		)
		return err
	}
	return nil
}

func (e *UnknownDomainEvaluator) info(msg string, args ...any) {
	if e.Logger != nil {
		e.Logger.Info(msg, args...)
	}
}

func (e *UnknownDomainEvaluator) warn(msg string, args ...any) {
	if e.Logger != nil {
		e.Logger.Warn(msg, args...)
	}
}

func (e *UnknownDomainEvaluator) error(msg string, args ...any) {
	if e.Logger != nil {
		e.Logger.Error(msg, args...)
	}
}

// Preview evaluates a domain in dry-run mode: it applies allowlist, seen-domain, and dedupe logic
// without creating alerts or mutating dedupe caches. When the domain would trigger an alert,
// it records the domain as seen to support baseline population.
func (e *UnknownDomainEvaluator) Preview(
	ctx context.Context,
	req UnknownDomainRequest,
) (bool, error) {
	if err := req.Scope.Validate(); err != nil {
		return false, err
	}
	domain := e.normalizeDomain(req.Domain)
	if domain == "" {
		req.record(UnknownDomainDecisionNormalizationFailed, strings.TrimSpace(req.Domain))
		return false, nil
	}

	if e.allowlisted(ctx, UnknownDomainAllowlistedCheckParams{
		Scope:  req.Scope,
		Domain: domain,
	}) {
		req.record(UnknownDomainDecisionAllowlisted, domain)
		if err := e.Caches.Seen.Record(ctx, SeenKey{Scope: req.Scope, Domain: domain}); err != nil {
			req.record(UnknownDomainDecisionError, domain)
			return false, fmt.Errorf("preview record allowlisted domain %q: %w", domain, err)
		}
		return false, nil
	}

	seen, err := e.Caches.Seen.Exists(ctx, SeenKey{Scope: req.Scope, Domain: domain})
	if err != nil {
		req.record(UnknownDomainDecisionError, domain)
		return false, fmt.Errorf("preview check seen domain %q: %w", domain, err)
	}
	if seen {
		req.record(UnknownDomainDecisionAlreadySeen, domain)
		return false, nil
	}

	dup, err := e.shouldDedupePreview(ctx, UnknownDomainDedupePreviewParams{
		Scope:  req.Scope,
		Domain: domain,
	})
	if err != nil {
		req.record(UnknownDomainDecisionError, domain)
		return false, fmt.Errorf("preview dedupe check domain %q: %w", domain, err)
	}
	if dup {
		req.record(UnknownDomainDecisionDeduped, domain)
		return false, nil
	}

	if recordErr := e.Caches.Seen.Record(ctx, SeenKey{Scope: req.Scope, Domain: domain}); recordErr != nil {
		req.record(UnknownDomainDecisionError, domain)
		return false, fmt.Errorf("preview record domain %q: %w", domain, recordErr)
	}
	req.record(UnknownDomainDecisionAlertCreated, domain)
	return true, nil
}

func (e *UnknownDomainEvaluator) normalizeDomain(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

type UnknownDomainAllowlistedCheckParams struct {
	Scope  ScopeKey
	Domain string
}

func (e *UnknownDomainEvaluator) allowlisted(
	ctx context.Context,
	params UnknownDomainAllowlistedCheckParams,
) bool {
	if e.Allowlist == nil {
		return false
	}
	return e.Allowlist.Allowed(ctx, params.Scope, params.Domain)
}

type UnknownDomainDedupeParams struct {
	Scope  ScopeKey
	Domain string
}

func (e *UnknownDomainEvaluator) shouldDedupe(
	ctx context.Context,
	params UnknownDomainDedupeParams,
) (bool, error) {
	if e.AlertTTL <= 0 || e.Caches.AlertOnce == nil {
		return false, nil
	}
	return e.Caches.AlertOnce.Seen(
		ctx,
		AlertSeenRequest{
			Scope:     params.Scope,
			DedupeKey: "unknown:" + params.Domain,
			TTL:       e.AlertTTL,
		},
	)
}

type UnknownDomainDedupePreviewParams struct {
	Scope  ScopeKey
	Domain string
}

func (e *UnknownDomainEvaluator) shouldDedupePreview(
	ctx context.Context,
	params UnknownDomainDedupePreviewParams,
) (bool, error) {
	if e.AlertTTL <= 0 || e.Caches.AlertOnce == nil {
		return false, nil
	}
	return e.Caches.AlertOnce.Peek(
		ctx,
		AlertSeenRequest{
			Scope:     params.Scope,
			DedupeKey: "unknown:" + params.Domain,
			TTL:       e.AlertTTL,
		},
	)
}

type UnknownDomainCreateAlertParams struct {
	Scope      ScopeKey
	Domain     string
	JobID      string
	RequestURL string
	PageURL    string
	Referrer   string
	UserAgent  string
	EventID    string
}

func (e *UnknownDomainEvaluator) createAlert(
	ctx context.Context,
	params UnknownDomainCreateAlertParams,
) error {
	title := "Unknown domain observed"
	desc := "First time seen domain: " + params.Domain + " (scope: " + params.Scope.Scope + ")"
	ctxObj := map[string]interface{}{
		"domain":  params.Domain,
		"scope":   params.Scope.Scope,
		"site_id": params.Scope.SiteID,
	}
	if params.JobID != "" {
		ctxObj["job_id"] = params.JobID
	}
	if params.EventID != "" {
		ctxObj["event_id"] = params.EventID
	}
	if params.RequestURL != "" {
		ctxObj["request_url"] = params.RequestURL
	}
	if params.PageURL != "" {
		ctxObj["page_url"] = params.PageURL
	}
	if params.Referrer != "" {
		ctxObj["referrer"] = params.Referrer
	}
	if params.UserAgent != "" {
		ctxObj["user_agent"] = params.UserAgent
	}
	ctxJSON, err := json.Marshal(ctxObj)
	if err != nil {
		return fmt.Errorf("marshal alert context: %w", err)
	}
	_, err = e.Alerter.Create(ctx, &model.CreateAlertRequest{
		SiteID:       params.Scope.SiteID,
		RuleID:       nil,
		RuleType:     string(model.AlertRuleTypeUnknownDomain),
		Severity:     string(model.AlertSeverityMedium),
		Title:        title,
		Description:  desc,
		EventContext: ctxJSON,
	})
	return err
}

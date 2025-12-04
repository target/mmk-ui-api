package rules

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/target/mmk-ui-api/internal/domain/model"
	servicerules "github.com/target/mmk-ui-api/internal/service/rules"
)

// UnknownDomainEvaluator exposes the subset of methods required by UnknownDomainRule.
type UnknownDomainEvaluator interface {
	Evaluate(ctx context.Context, req servicerules.UnknownDomainRequest) (bool, error)
	Preview(ctx context.Context, req servicerules.UnknownDomainRequest) (bool, error)
}

// UnknownDomainRule adapts a rules.UnknownDomainEvaluator into a domain rules pipeline rule.
type UnknownDomainRule struct {
	Evaluator UnknownDomainEvaluator
}

// ID identifies the rule.
func (r *UnknownDomainRule) ID() string {
	return "unknown_domain"
}

// Evaluate runs the unknown domain evaluator and records metrics via the rule pipeline.
func (r *UnknownDomainRule) Evaluate(ctx context.Context, item RuleWorkItem) RuleEvaluation {
	result := RuleEvaluation{RuleID: r.ID()}
	if r == nil || r.Evaluator == nil {
		return result
	}

	domain := strings.TrimSpace(item.Domain)
	if domain == "" {
		return result
	}

	mode := normalizeAlertMode(item.AlertMode)

	recorder := &unknownDomainCapture{dryRun: item.DryRun, alertMode: mode}
	req := servicerules.UnknownDomainRequest{
		Scope: servicerules.ScopeKey{
			SiteID: item.SiteID,
			Scope:  item.Scope,
		},
		Domain:     domain,
		SiteID:     item.SiteID,
		JobID:      item.JobID,
		RequestURL: item.RequestURL,
		PageURL:    item.PageURL,
		Referrer:   item.Referrer,
		UserAgent:  item.UserAgent,
		EventID:    item.EventID,
		Recorder:   recorder,
	}

	var (
		applyFn func(*ProcessingResults)
		err     error
	)
	if item.DryRun {
		applyFn, err = r.evaluateDryRun(ctx, req, recorder, domain)
	} else {
		applyFn, err = r.evaluateLive(ctx, req, recorder)
	}
	if err != nil {
		result.Err = err
		return result
	}

	result.ApplyFn = applyFn
	return result
}

func (r *UnknownDomainRule) evaluateDryRun(
	ctx context.Context,
	req servicerules.UnknownDomainRequest,
	recorder *unknownDomainCapture,
	domain string,
) (func(*ProcessingResults), error) {
	wouldAlertFlag, err := r.Evaluator.Preview(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("unknown-domain preview: %w", err)
	}

	unknownDomains := 0
	var wouldAlert []string
	if wouldAlertFlag {
		unknownDomains++
		wouldAlert = append(wouldAlert, strings.ToLower(domain))
	}
	metrics := recorder.metrics

	return func(results *ProcessingResults) {
		if results == nil {
			return
		}
		results.UnknownDomains += unknownDomains
		for _, d := range wouldAlert {
			AppendUniqueLower(&results.WouldAlertUnknown, d)
		}
		MergeUnknownDomainMetrics(&results.UnknownDomain, metrics)
	}, nil
}

func (r *UnknownDomainRule) evaluateLive(
	ctx context.Context,
	req servicerules.UnknownDomainRequest,
	recorder *unknownDomainCapture,
) (func(*ProcessingResults), error) {
	alerted, err := r.Evaluator.Evaluate(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("unknown-domain evaluate: %w", err)
	}

	alertsCreated := 0
	unknownDomains := 0
	if alerted {
		alertsCreated++
		unknownDomains++
	}
	metrics := recorder.metrics

	return func(results *ProcessingResults) {
		if results == nil {
			return
		}
		results.AlertsCreated += alertsCreated
		results.UnknownDomains += unknownDomains
		MergeUnknownDomainMetrics(&results.UnknownDomain, metrics)
	}, nil
}

type unknownDomainCapture struct {
	dryRun    bool
	alertMode model.SiteAlertMode
	metrics   UnknownDomainMetrics
}

func (c *unknownDomainCapture) RecordUnknownDomainDecision(
	decision servicerules.UnknownDomainDecision,
	domain string,
) {
	d := strings.ToLower(strings.TrimSpace(domain))
	switch decision {
	case servicerules.UnknownDomainDecisionAlertCreated:
		c.recordAlertCreated(d)
	case servicerules.UnknownDomainDecisionAllowlisted:
		c.metrics.SuppressedAllowlist.Record(d)
	case servicerules.UnknownDomainDecisionAlreadySeen:
		c.metrics.SuppressedSeen.Record(d)
	case servicerules.UnknownDomainDecisionDeduped:
		c.metrics.SuppressedDedupe.Record(d)
	case servicerules.UnknownDomainDecisionNormalizationFailed:
		c.metrics.NormalizationFailed.Record(d)
	case servicerules.UnknownDomainDecisionError:
		c.metrics.Errors.Record(d)
	}
}

func (c *unknownDomainCapture) recordAlertCreated(domain string) {
	if c.dryRun {
		c.metrics.AlertedDryRun.Record(domain)
		return
	}

	if c.alertMode == model.SiteAlertModeMuted {
		c.metrics.AlertedMuted.Record(domain)
		return
	}

	c.metrics.Alerted.Record(domain)
}

// IOCEvaluator exposes the subset of methods required by IOCRule.
type IOCEvaluator interface {
	Evaluate(ctx context.Context, req servicerules.IOCRequest) (bool, error)
}

// IOCRule adapts a rules.IOCEvaluator into a domain rules pipeline rule.
type IOCRule struct {
	Evaluator IOCEvaluator
	Cache     servicerules.IOCCache
}

// ID identifies the rule.
func (r *IOCRule) ID() string {
	return string(model.RuleTypeIOC)
}

// Evaluate runs the IOC evaluator and records metrics via the rule pipeline.
func (r *IOCRule) Evaluate(ctx context.Context, item RuleWorkItem) RuleEvaluation {
	result := RuleEvaluation{RuleID: r.ID()}
	if r == nil || r.Evaluator == nil {
		return result
	}

	host := strings.TrimSpace(item.Domain)
	if host == "" {
		return result
	}
	normalized := strings.ToLower(host)

	var (
		applyFn func(*ProcessingResults)
		err     error
	)
	if item.DryRun {
		applyFn, err = r.evaluateDryRun(ctx, normalized)
	} else {
		applyFn, err = r.evaluateLive(ctx, host, normalized, item)
	}
	if err != nil {
		result.Err = err
		return result
	}
	result.ApplyFn = applyFn
	return result
}

func (r *IOCRule) evaluateLive(
	ctx context.Context,
	host string,
	normalized string,
	item RuleWorkItem,
) (func(*ProcessingResults), error) {
	alerted, err := r.Evaluator.Evaluate(ctx, servicerules.IOCRequest{
		Scope: servicerules.ScopeKey{
			SiteID: item.SiteID,
			Scope:  item.Scope,
		},
		Host:       host,
		SiteID:     item.SiteID,
		JobID:      item.JobID,
		RequestURL: item.RequestURL,
		PageURL:    item.PageURL,
		Referrer:   item.Referrer,
		UserAgent:  item.UserAgent,
		EventID:    item.EventID,
	})
	if err != nil {
		return nil, fmt.Errorf("ioc evaluate: %w", err)
	}

	metrics := IOCMetrics{}
	matches := 0
	alerts := 0
	if alerted {
		matches = 1
		alerts = 1
		recordIOCAlertMetrics(&metrics, normalized, normalizeAlertMode(item.AlertMode))
	}

	return func(results *ProcessingResults) {
		if results == nil {
			return
		}
		results.AlertsCreated += alerts
		results.IOCHostMatches += matches
		MergeIOCMetrics(&results.IOC, metrics)
	}, nil
}

func (r *IOCRule) evaluateDryRun(
	ctx context.Context,
	normalized string,
) (func(*ProcessingResults), error) {
	if r.Cache == nil {
		return nil, nil //nolint:nilnil // returning nil indicates no-op apply function needed
	}

	ioc, err := r.Cache.LookupHost(ctx, normalized)
	if err != nil {
		if errors.Is(err, servicerules.ErrNotFound) {
			return nil, nil //nolint:nilnil // cache miss: no-op apply function
		}
		return nil, fmt.Errorf("ioc cache lookup: %w", err)
	}
	if ioc == nil {
		return nil, nil //nolint:nilnil // absent IOC by cache contract
	}

	metrics := IOCMetrics{}
	metrics.MatchesDryRun.Record(normalized)

	return func(results *ProcessingResults) {
		if results == nil {
			return
		}
		results.IOCHostMatches++
		AppendUniqueLower(&results.WouldAlertIOC, normalized)
		MergeIOCMetrics(&results.IOC, metrics)
	}, nil
}

func normalizeAlertMode(mode model.SiteAlertMode) model.SiteAlertMode {
	if normalized, ok := model.ParseSiteAlertMode(string(mode)); ok {
		return normalized
	}
	return model.SiteAlertModeActive
}

func recordIOCAlertMetrics(metrics *IOCMetrics, normalized string, mode model.SiteAlertMode) {
	if metrics == nil {
		return
	}
	metrics.Matches.Record(normalized)
	if mode == model.SiteAlertModeMuted {
		metrics.AlertsMuted.Record(normalized)
		return
	}
	metrics.Alerts.Record(normalized)
}

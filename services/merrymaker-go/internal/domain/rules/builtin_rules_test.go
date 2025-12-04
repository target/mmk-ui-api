package rules

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	servicerules "github.com/target/mmk-ui-api/internal/service/rules"
)

type stubUnknownDomainEvaluator struct {
	evalFn    func(ctx context.Context, req servicerules.UnknownDomainRequest) (bool, error)
	previewFn func(ctx context.Context, req servicerules.UnknownDomainRequest) (bool, error)
}

func (s stubUnknownDomainEvaluator) Evaluate(
	ctx context.Context,
	req servicerules.UnknownDomainRequest,
) (bool, error) {
	if s.evalFn != nil {
		return s.evalFn(ctx, req)
	}
	return false, nil
}

func (s stubUnknownDomainEvaluator) Preview(
	ctx context.Context,
	req servicerules.UnknownDomainRequest,
) (bool, error) {
	if s.previewFn != nil {
		return s.previewFn(ctx, req)
	}
	return false, nil
}

func TestUnknownDomainRule_NormalAlert(t *testing.T) {
	rule := UnknownDomainRule{
		Evaluator: stubUnknownDomainEvaluator{
			evalFn: func(_ context.Context, req servicerules.UnknownDomainRequest) (bool, error) {
				if req.Recorder != nil {
					req.Recorder.RecordUnknownDomainDecision(
						servicerules.UnknownDomainDecisionAlertCreated,
						strings.ToLower(req.Domain),
					)
				}
				return true, nil
			},
		},
	}

	item := RuleWorkItem{
		SiteID:    "site-1",
		Scope:     "default",
		Domain:    "Example.com",
		AlertMode: model.SiteAlertModeActive,
	}

	eval := rule.Evaluate(context.Background(), item)
	require.Equal(t, rule.ID(), eval.RuleID)
	require.NoError(t, eval.Err)
	require.NotNil(t, eval.ApplyFn)

	results := &ProcessingResults{}
	eval.Apply(results)

	assert.Equal(t, 1, results.AlertsCreated)
	assert.Equal(t, 1, results.UnknownDomains)
	assert.Equal(t, 1, results.UnknownDomain.Alerted.Count)
	assert.Contains(t, results.UnknownDomain.Alerted.Samples, "example.com")
	assert.Empty(t, results.WouldAlertUnknown)
}

func TestUnknownDomainRule_MutedAlert(t *testing.T) {
	rule := UnknownDomainRule{
		Evaluator: stubUnknownDomainEvaluator{
			evalFn: func(_ context.Context, req servicerules.UnknownDomainRequest) (bool, error) {
				if req.Recorder != nil {
					req.Recorder.RecordUnknownDomainDecision(
						servicerules.UnknownDomainDecisionAlertCreated,
						strings.ToLower(req.Domain),
					)
				}
				return true, nil
			},
		},
	}

	item := RuleWorkItem{
		SiteID:    "site-1",
		Scope:     "default",
		Domain:    "Muted.com",
		AlertMode: model.SiteAlertModeMuted,
	}

	eval := rule.Evaluate(context.Background(), item)
	require.NoError(t, eval.Err)

	results := &ProcessingResults{}
	eval.Apply(results)

	assert.Equal(t, 1, results.AlertsCreated)
	assert.Equal(t, 1, results.UnknownDomains)
	assert.Equal(t, 0, results.UnknownDomain.Alerted.Count)
	assert.Equal(t, 1, results.UnknownDomain.AlertedMuted.Count)
	assert.Contains(t, results.UnknownDomain.AlertedMuted.Samples, "muted.com")
}

func TestUnknownDomainRule_MutedAlert_WithUppercaseMode(t *testing.T) {
	rule := UnknownDomainRule{
		Evaluator: stubUnknownDomainEvaluator{
			evalFn: func(_ context.Context, req servicerules.UnknownDomainRequest) (bool, error) {
				if req.Recorder != nil {
					req.Recorder.RecordUnknownDomainDecision(
						servicerules.UnknownDomainDecisionAlertCreated,
						strings.ToLower(req.Domain),
					)
				}
				return true, nil
			},
		},
	}

	item := RuleWorkItem{
		SiteID:    "site-1",
		Scope:     "default",
		Domain:    "Muted.com",
		AlertMode: model.SiteAlertMode("Muted"),
	}

	eval := rule.Evaluate(context.Background(), item)
	require.NoError(t, eval.Err)

	results := &ProcessingResults{}
	eval.Apply(results)

	assert.Equal(t, 1, results.UnknownDomain.AlertedMuted.Count)
}

func TestUnknownDomainRule_DryRun(t *testing.T) {
	rule := UnknownDomainRule{
		Evaluator: stubUnknownDomainEvaluator{
			previewFn: func(_ context.Context, req servicerules.UnknownDomainRequest) (bool, error) {
				if req.Recorder != nil {
					req.Recorder.RecordUnknownDomainDecision(
						servicerules.UnknownDomainDecisionAlertCreated,
						strings.ToLower(req.Domain),
					)
				}
				return true, nil
			},
		},
	}

	item := RuleWorkItem{
		SiteID: "site-1",
		Scope:  "default",
		Domain: "Unknown.test",
		DryRun: true,
	}

	eval := rule.Evaluate(context.Background(), item)
	require.NoError(t, eval.Err)

	results := &ProcessingResults{}
	eval.Apply(results)

	assert.Equal(t, 0, results.AlertsCreated)
	assert.Equal(t, 1, results.UnknownDomains)
	assert.ElementsMatch(t, []string{"unknown.test"}, results.WouldAlertUnknown)
	assert.Equal(t, 0, results.UnknownDomain.Alerted.Count)
	assert.Equal(t, 1, results.UnknownDomain.AlertedDryRun.Count)
	assert.Contains(t, results.UnknownDomain.AlertedDryRun.Samples, "unknown.test")
}

type stubIOCEvaluator struct {
	evalFn func(ctx context.Context, req servicerules.IOCRequest) (bool, error)
}

func (s stubIOCEvaluator) Evaluate(
	ctx context.Context,
	req servicerules.IOCRequest,
) (bool, error) {
	if s.evalFn != nil {
		return s.evalFn(ctx, req)
	}
	return false, nil
}

type stubIOCCache struct {
	lookupFn func(ctx context.Context, host string) (*model.IOC, error)
}

func (s stubIOCCache) LookupHost(
	ctx context.Context,
	host string,
) (*model.IOC, error) {
	if s.lookupFn != nil {
		return s.lookupFn(ctx, host)
	}
	return nil, servicerules.ErrNotFound
}

func TestIOCRule_NormalAlert(t *testing.T) {
	item := RuleWorkItem{
		SiteID:    "site-1",
		Scope:     "default",
		Domain:    "malicious.test",
		AlertMode: model.SiteAlertModeActive,
	}

	rule := IOCRule{
		Evaluator: stubIOCEvaluator{
			evalFn: func(_ context.Context, _ servicerules.IOCRequest) (bool, error) {
				return true, nil
			},
		},
	}

	eval := rule.Evaluate(context.Background(), item)
	require.NoError(t, eval.Err)

	results := &ProcessingResults{}
	eval.Apply(results)

	assert.Equal(t, 1, results.AlertsCreated)
	assert.Equal(t, 1, results.IOCHostMatches)
	assert.Equal(t, 1, results.IOC.Matches.Count)
	assert.Equal(t, 1, results.IOC.Alerts.Count)
}

func TestIOCRule_MutedAlert(t *testing.T) {
	item := RuleWorkItem{
		SiteID:    "site-1",
		Scope:     "default",
		Domain:    "muted.example",
		AlertMode: model.SiteAlertModeMuted,
	}

	rule := IOCRule{
		Evaluator: stubIOCEvaluator{
			evalFn: func(_ context.Context, _ servicerules.IOCRequest) (bool, error) {
				return true, nil
			},
		},
	}

	eval := rule.Evaluate(context.Background(), item)
	require.NoError(t, eval.Err)

	results := &ProcessingResults{}
	eval.Apply(results)

	assert.Equal(t, 1, results.AlertsCreated)
	assert.Equal(t, 1, results.IOCHostMatches)
	assert.Equal(t, 1, results.IOC.Matches.Count)
	assert.Equal(t, 0, results.IOC.Alerts.Count)
	assert.Equal(t, 1, results.IOC.AlertsMuted.Count)
	assert.Contains(t, results.IOC.AlertsMuted.Samples, "muted.example")
}

func TestIOCRule_MutedAlert_WithUppercaseMode(t *testing.T) {
	item := RuleWorkItem{
		SiteID:    "site-1",
		Scope:     "default",
		Domain:    "muted.example",
		AlertMode: model.SiteAlertMode("Muted"),
	}

	rule := IOCRule{
		Evaluator: stubIOCEvaluator{
			evalFn: func(_ context.Context, _ servicerules.IOCRequest) (bool, error) {
				return true, nil
			},
		},
	}

	eval := rule.Evaluate(context.Background(), item)
	require.NoError(t, eval.Err)

	results := &ProcessingResults{}
	eval.Apply(results)

	assert.Equal(t, 1, results.IOCHostMatches)
	assert.Equal(t, 1, results.IOC.AlertsMuted.Count)
}

func TestIOCRule_DryRun(t *testing.T) {
	item := RuleWorkItem{
		SiteID: "site-1",
		Scope:  "default",
		Domain: "ioc.test",
		DryRun: true,
	}

	rule := IOCRule{
		Evaluator: stubIOCEvaluator{},
		Cache: stubIOCCache{
			lookupFn: func(_ context.Context, host string) (*model.IOC, error) {
				return &model.IOC{ID: "1", Value: host}, nil
			},
		},
	}

	eval := rule.Evaluate(context.Background(), item)
	require.NoError(t, eval.Err)

	results := &ProcessingResults{}
	eval.Apply(results)

	assert.Equal(t, 0, results.AlertsCreated)
	assert.Equal(t, 1, results.IOCHostMatches)
	assert.ElementsMatch(t, []string{"ioc.test"}, results.WouldAlertIOC)
	assert.Equal(t, 1, results.IOC.MatchesDryRun.Count)
	assert.Contains(t, results.IOC.MatchesDryRun.Samples, "ioc.test")
}

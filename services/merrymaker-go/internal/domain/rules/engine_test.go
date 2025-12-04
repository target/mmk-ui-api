package rules

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type stubRule struct {
	id    string
	apply func(*ProcessingResults)
	err   error
}

func (s stubRule) ID() string {
	return s.id
}

func (s stubRule) Evaluate(_ context.Context, _ RuleWorkItem) RuleEvaluation {
	eval := RuleEvaluation{RuleID: s.id}
	if s.apply != nil {
		eval.ApplyFn = s.apply
	}
	eval.Err = s.err
	return eval
}

func TestDefaultRuleEngineEvaluate(t *testing.T) {
	engine := NewRuleEngine([]Rule{
		stubRule{
			id: "ok",
			apply: func(results *ProcessingResults) {
				results.AlertsCreated += 2
			},
		},
		stubRule{
			id:  "err",
			err: assert.AnError,
		},
		nil,
	})

	item := RuleWorkItem{SiteID: "site-1", Scope: "default"}

	evals := engine.Evaluate(context.Background(), item)
	require.Len(t, evals, 2)

	require.Equal(t, "ok", evals[0].RuleID)
	require.NoError(t, evals[0].Err)
	require.NotNil(t, evals[0].ApplyFn)

	require.Equal(t, "err", evals[1].RuleID)
	require.Error(t, evals[1].Err)
	require.Nil(t, evals[1].ApplyFn)

	results := &ProcessingResults{}
	for _, eval := range evals {
		if eval.Err != nil {
			results.ErrorsEncountered++
			continue
		}
		eval.Apply(results)
	}

	assert.Equal(t, 2, results.AlertsCreated)
	assert.Equal(t, 1, results.ErrorsEncountered)
}

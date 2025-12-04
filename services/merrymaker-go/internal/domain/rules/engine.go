package rules

import "context"

// Rule evaluates a work item and returns a RuleEvaluation describing the outcome.
type Rule interface {
	ID() string
	Evaluate(ctx context.Context, item RuleWorkItem) RuleEvaluation
}

// RuleFunc adapts a simple function to the Rule interface.
type RuleFunc func(ctx context.Context, item RuleWorkItem) RuleEvaluation

// Evaluate executes f(ctx, item).
func (f RuleFunc) Evaluate(ctx context.Context, item RuleWorkItem) RuleEvaluation {
	if f == nil {
		return RuleEvaluation{}
	}
	return f(ctx, item)
}

// DefaultRuleEngine executes a collection of rules for a given work item.
type DefaultRuleEngine struct {
	rules []Rule
}

// NewRuleEngine constructs a DefaultRuleEngine from the supplied rules, filtering nil entries.
func NewRuleEngine(rules []Rule) *DefaultRuleEngine {
	filtered := make([]Rule, 0, len(rules))
	for _, rule := range rules {
		if rule != nil {
			filtered = append(filtered, rule)
		}
	}
	return &DefaultRuleEngine{rules: filtered}
}

// Evaluate runs each configured rule and returns their individual evaluations.
func (e *DefaultRuleEngine) Evaluate(ctx context.Context, item RuleWorkItem) []RuleEvaluation {
	if e == nil || len(e.rules) == 0 {
		return nil
	}

	evaluations := make([]RuleEvaluation, 0, len(e.rules))
	for _, rule := range e.rules {
		if rule == nil {
			continue
		}
		eval := rule.Evaluate(ctx, item)
		if eval.RuleID == "" {
			eval.RuleID = rule.ID()
		}
		evaluations = append(evaluations, eval)
	}
	return evaluations
}

var _ RuleEngine = (*DefaultRuleEngine)(nil)

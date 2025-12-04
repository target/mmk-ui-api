package rules

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultPipeline_RunProcessesEvents(t *testing.T) {
	t.Helper()

	engine := &stubRuleEngine{
		responses: [][]RuleEvaluation{
			{
				{
					RuleID: "capture-alert",
					ApplyFn: func(res *ProcessingResults) {
						res.AlertsCreated++
					},
				},
			},
		},
	}
	pipeline := NewPipeline(PipelineOptions{
		Engine:    engine,
		Extractor: DomainExtractorFunc(func(model.RawEvent) (string, bool) { return "example.com", true }),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	payload := &JobPayload{SiteID: "site-123", Scope: "default"}
	event := &model.Event{ID: "evt-1"}

	results, err := pipeline.Run(context.Background(), PipelineParams{
		Events:    []*model.Event{event},
		Payload:   payload,
		DryRun:    false,
		AlertMode: model.SiteAlertModeActive,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, results.DomainsProcessed)
	assert.Equal(t, 0, results.EventsSkipped)
	assert.Equal(t, 1, results.AlertsCreated)
	require.Len(t, engine.items, 1)
	assert.Equal(t, "site-123", engine.items[0].SiteID)
	assert.Equal(t, "default", engine.items[0].Scope)
	assert.Equal(t, "example.com", engine.items[0].Domain)
	assert.False(t, engine.items[0].DryRun)
}

func TestDefaultPipeline_RunSkipsWhenExtractorFails(t *testing.T) {
	t.Helper()

	engine := &stubRuleEngine{}
	pipeline := NewPipeline(PipelineOptions{
		Engine:    engine,
		Extractor: DomainExtractorFunc(func(model.RawEvent) (string, bool) { return "", false }),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	results, err := pipeline.Run(context.Background(), PipelineParams{
		Events:  []*model.Event{{ID: "evt-2"}},
		Payload: &JobPayload{SiteID: "site", Scope: "scope"},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, results.EventsSkipped)
	assert.Zero(t, results.DomainsProcessed)
	assert.Empty(t, engine.items)
}

func TestDefaultPipeline_RunTracksEvaluationErrors(t *testing.T) {
	t.Helper()

	engine := &stubRuleEngine{
		responses: [][]RuleEvaluation{
			{
				{
					RuleID: "broken",
					Err:    errors.New("rule failure"),
				},
			},
		},
	}
	pipeline := NewPipeline(PipelineOptions{
		Engine:    engine,
		Extractor: DomainExtractorFunc(func(model.RawEvent) (string, bool) { return "error.test", true }),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	results, err := pipeline.Run(context.Background(), PipelineParams{
		Events:  []*model.Event{{ID: "evt-3"}},
		Payload: &JobPayload{SiteID: "site", Scope: "scope"},
		DryRun:  true,
	})
	require.NoError(t, err)

	assert.Equal(t, 1, results.DomainsProcessed)
	assert.Equal(t, 1, results.ErrorsEncountered)
	assert.Equal(t, model.SiteAlertModeActive, results.AlertMode)
	require.Len(t, engine.items, 1)
	assert.True(t, engine.items[0].DryRun)
}

func TestDefaultPipeline_RunSkipsNilEvents(t *testing.T) {
	t.Helper()

	engine := &stubRuleEngine{}
	pipeline := NewPipeline(PipelineOptions{
		Engine: engine,
		Extractor: DomainExtractorFunc(func(model.RawEvent) (string, bool) {
			return "ignored", true
		}),
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	results, err := pipeline.Run(context.Background(), PipelineParams{
		Events: []*model.Event{nil},
	})
	require.NoError(t, err)

	assert.Equal(t, 1, results.EventsSkipped)
	assert.Zero(t, results.DomainsProcessed)
	assert.Empty(t, engine.items)
}

func TestDefaultPipeline_RunRespectsContextCancellation(t *testing.T) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	pipeline := NewPipeline(PipelineOptions{
		Extractor: DomainExtractorFunc(func(model.RawEvent) (string, bool) { return "ctx.test", true }),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	results, err := pipeline.Run(ctx, PipelineParams{
		Events: []*model.Event{{ID: "evt-ctx"}},
	})
	require.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, model.SiteAlertModeActive, results.AlertMode)
	assert.Equal(t, 0, results.DomainsProcessed)
}

func TestDefaultPipeline_RunDefaultsAlertMode(t *testing.T) {
	t.Helper()

	engine := &stubRuleEngine{
		responses: [][]RuleEvaluation{
			{
				{
					RuleID: "noop",
					ApplyFn: func(res *ProcessingResults) {
						res.AlertsCreated++
					},
				},
			},
		},
	}

	pipeline := NewPipeline(PipelineOptions{
		Engine:    engine,
		Extractor: DomainExtractorFunc(func(model.RawEvent) (string, bool) { return "default-alert.test", true }),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	results, err := pipeline.Run(context.Background(), PipelineParams{
		Events: []*model.Event{{ID: "evt-alert"}},
		// Intentionally leave AlertMode empty to exercise defaulting logic.
	})
	require.NoError(t, err)

	assert.Equal(t, model.SiteAlertModeActive, results.AlertMode)
	require.Len(t, engine.items, 1)
	assert.Equal(t, model.SiteAlertModeActive, engine.items[0].AlertMode)
	assert.Equal(t, 1, results.AlertsCreated)
}

func TestDefaultPipeline_RunNormalizesAlertMode(t *testing.T) {
	t.Helper()

	engine := &stubRuleEngine{
		responses: [][]RuleEvaluation{
			{
				{
					RuleID: "noop",
				},
			},
		},
	}

	pipeline := NewPipeline(PipelineOptions{
		Engine:    engine,
		Extractor: DomainExtractorFunc(func(model.RawEvent) (string, bool) { return "muted-alert.test", true }),
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	})

	results, err := pipeline.Run(context.Background(), PipelineParams{
		Events:    []*model.Event{{ID: "evt-alert"}},
		AlertMode: model.SiteAlertMode("Muted"),
	})
	require.NoError(t, err)

	assert.Equal(t, model.SiteAlertModeMuted, results.AlertMode)
	require.Len(t, engine.items, 1)
	assert.Equal(t, model.SiteAlertModeMuted, engine.items[0].AlertMode)
}

type stubRuleEngine struct {
	responses [][]RuleEvaluation
	items     []RuleWorkItem
}

func (s *stubRuleEngine) Evaluate(_ context.Context, item RuleWorkItem) []RuleEvaluation {
	s.items = append(s.items, item)
	if len(s.responses) == 0 {
		return nil
	}
	resp := s.responses[0]
	s.responses = s.responses[1:]
	return append([]RuleEvaluation(nil), resp...)
}
